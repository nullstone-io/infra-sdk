package gcp_project

import (
	"fmt"
	"strings"

	"cloud.google.com/go/bigquery"
	infra_sdk "github.com/nullstone-io/infra-sdk"
)

var granularityToInterval = map[infra_sdk.CostGranularity]string{
	infra_sdk.CostGranularityHourly:  "HOUR",
	infra_sdk.CostGranularityDaily:   "DAY",
	infra_sdk.CostGranularityMonthly: "MONTH",
}

var granularityToEndInterval = map[infra_sdk.CostGranularity]string{
	infra_sdk.CostGranularityHourly:  "INTERVAL 1 HOUR",
	infra_sdk.CostGranularityDaily:   "INTERVAL 1 DAY",
	infra_sdk.CostGranularityMonthly: "INTERVAL 1 MONTH",
}

// The standard billing export reports `cost` before any credit and lists every credit separately
// with a type. FOCUS measures are derived by adding back the credit types that belong to each:
//
//	ListCost       = cost_at_list
//	ContractedCost = cost + negotiated credits            (DISCOUNT, RESELLER_MARGIN)
//	EffectiveCost  = ContractedCost + commitment credits  (CUD, SUD, subscription, free tier)
//	BilledCost     = cost + every credit                  (adds promotions, fee offsets, ...)
//
// Credit amounts are negative in the export, so "adding" a credit reduces the measure.
//
// PROVISIONAL: GCP documents DISCOUNT as "contractual spending commitment" credits, which is a
// negotiated discount in FOCUS terms and therefore belongs in ContractedCost. Verify against a
// billing account with a negotiated agreement before relying on the ListCost-ContractedCost gap.
var (
	negotiatedCreditTypes = []string{"DISCOUNT", "RESELLER_MARGIN"}
	commitmentCreditTypes = []string{
		"COMMITTED_USAGE_DISCOUNT",
		"COMMITTED_USAGE_DISCOUNT_DOLLAR_BASE",
		"SUSTAINED_USAGE_DISCOUNT",
		"SUBSCRIPTION_BENEFIT",
		"FREE_TIER",
	}
)

func amortizedCreditTypes() []string {
	return append(append([]string{}, negotiatedCreditTypes...), commitmentCreditTypes...)
}

// SupportedMetrics lists the FOCUS measures the standard billing export can report.
func SupportedMetrics() []infra_sdk.CostMetric {
	return infra_sdk.AllCostMetrics()
}

// periodEndExpr builds the expression closing each bucket.
// BigQuery's TIMESTAMP_ADD only accepts date parts up to DAY -- adding INTERVAL 1 MONTH to a
// TIMESTAMP is a query error -- so a monthly bucket adds the month in the DATE domain and
// converts back.
func periodEndExpr(granularity infra_sdk.CostGranularity, truncInterval string) string {
	bucketStart := fmt.Sprintf("TIMESTAMP_TRUNC(usage_start_time, %s)", truncInterval)
	if granularity == infra_sdk.CostGranularityMonthly {
		return fmt.Sprintf("TIMESTAMP(DATE_ADD(DATE(%s), INTERVAL 1 MONTH))", bucketStart)
	}
	return fmt.Sprintf("TIMESTAMP_ADD(%s, %s)", bucketStart, granularityToEndInterval[granularity])
}

type QueryBuilder struct {
	Table string
}

type builtQuery struct {
	SQL    string
	Params []bigquery.QueryParameter
}

func (b *QueryBuilder) Build(query infra_sdk.CostQuery) builtQuery {
	granularity := query.Granularity
	if granularity == "" {
		granularity = infra_sdk.CostGranularityDaily
	}
	truncInterval := granularityToInterval[granularity]

	groupBy := query.GroupBy.Unique()

	var selectCols []string
	var groupByCols []string
	var params []bigquery.QueryParameter

	// Always select the time window
	selectCols = append(selectCols,
		fmt.Sprintf("TIMESTAMP_TRUNC(usage_start_time, %s) AS period_start", truncInterval),
		fmt.Sprintf("%s AS period_end", periodEndExpr(granularity, truncInterval)),
	)
	groupByCols = append(groupByCols, "period_start", "period_end")

	// Add group-by columns
	for i, grp := range groupBy {
		if grp.Dimension != "" {
			col := UniversalDimension(grp.Dimension).ToGcpColumn()
			alias := fmt.Sprintf("dim_%d", i)
			selectCols = append(selectCols, fmt.Sprintf("%s AS %s", col, alias))
			groupByCols = append(groupByCols, alias)
		} else if grp.TagKey != "" {
			labelKey := UniversalTag(grp.TagKey).ToGcp()
			alias := fmt.Sprintf("label_%d", i)
			paramName := fmt.Sprintf("grp_label_%d", i)
			selectCols = append(selectCols,
				fmt.Sprintf("(SELECT l.value FROM UNNEST(labels) l WHERE l.key = @%s) AS %s", paramName, alias),
			)
			groupByCols = append(groupByCols, alias)
			params = append(params, bigquery.QueryParameter{
				Name:  paramName,
				Value: labelKey,
			})
		}
	}

	// Always select the FOCUS measures and currency
	selectCols = append(selectCols,
		"SUM(IFNULL(cost_at_list, cost)) AS list_cost",
		"SUM(cost) + SUM(IFNULL((SELECT SUM(c.amount) FROM UNNEST(credits) c WHERE c.type IN UNNEST(@negotiated_credit_types)), 0)) AS contracted_cost",
		"SUM(cost) + SUM(IFNULL((SELECT SUM(c.amount) FROM UNNEST(credits) c WHERE c.type IN UNNEST(@amortized_credit_types)), 0)) AS effective_cost",
		"SUM(cost) + SUM(IFNULL((SELECT SUM(c.amount) FROM UNNEST(credits) c), 0)) AS billed_cost",
		"currency",
	)
	groupByCols = append(groupByCols, "currency")
	params = append(params,
		bigquery.QueryParameter{Name: "negotiated_credit_types", Value: negotiatedCreditTypes},
		bigquery.QueryParameter{Name: "amortized_credit_types", Value: amortizedCreditTypes()},
	)

	// Build WHERE clause
	whereClauses := []string{
		"usage_start_time >= @start_time",
		"usage_start_time < @end_time",
	}
	params = append(params,
		bigquery.QueryParameter{Name: "start_time", Value: query.Start},
		bigquery.QueryParameter{Name: "end_time", Value: query.End},
	)

	// Add filter tag conditions.
	// An empty-string value asks for rows without the label (see infra_sdk.CostFilterTag).
	for i, filter := range query.FilterTags {
		labelKey := UniversalTag(filter.Key).ToGcp()
		keyParam := fmt.Sprintf("filter_key_%d", i)
		params = append(params, bigquery.QueryParameter{Name: keyParam, Value: labelKey})

		var clauses []string
		if present := filter.PresentValues(); len(present) > 0 {
			valParam := fmt.Sprintf("filter_vals_%d", i)
			clauses = append(clauses,
				fmt.Sprintf("EXISTS(SELECT 1 FROM UNNEST(labels) l WHERE l.key = @%s AND l.value IN UNNEST(@%s))", keyParam, valParam),
			)
			params = append(params, bigquery.QueryParameter{Name: valParam, Value: present})
		}
		if filter.MatchesAbsent() {
			clauses = append(clauses,
				fmt.Sprintf("NOT EXISTS(SELECT 1 FROM UNNEST(labels) l WHERE l.key = @%s)", keyParam),
			)
		}
		switch len(clauses) {
		case 0:
			// a filter with no values matches nothing; keep the query valid and empty
			whereClauses = append(whereClauses, "FALSE")
		case 1:
			whereClauses = append(whereClauses, clauses[0])
		default:
			whereClauses = append(whereClauses, fmt.Sprintf("(%s)", strings.Join(clauses, " OR ")))
		}
	}

	sql := fmt.Sprintf(
		"SELECT %s FROM `%s` WHERE %s GROUP BY %s ORDER BY period_start",
		strings.Join(selectCols, ", "),
		b.Table,
		strings.Join(whereClauses, " AND "),
		strings.Join(groupByCols, ", "),
	)

	return builtQuery{SQL: sql, Params: params}
}
