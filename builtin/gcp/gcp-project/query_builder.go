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
	endInterval := granularityToEndInterval[granularity]

	groupBy := query.GroupBy.Unique()

	var selectCols []string
	var groupByCols []string
	var params []bigquery.QueryParameter

	// Always select the time window
	selectCols = append(selectCols,
		fmt.Sprintf("TIMESTAMP_TRUNC(usage_start_time, %s) AS period_start", truncInterval),
		fmt.Sprintf("TIMESTAMP_ADD(TIMESTAMP_TRUNC(usage_start_time, %s), %s) AS period_end", truncInterval, endInterval),
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

	// Always select cost and currency
	selectCols = append(selectCols,
		"SUM(cost) + SUM(IFNULL((SELECT SUM(c.amount) FROM UNNEST(credits) c), 0)) AS total_cost",
		"currency",
	)
	groupByCols = append(groupByCols, "currency")

	// Build WHERE clause
	whereClauses := []string{
		"usage_start_time >= @start_time",
		"usage_start_time < @end_time",
	}
	params = append(params,
		bigquery.QueryParameter{Name: "start_time", Value: query.Start},
		bigquery.QueryParameter{Name: "end_time", Value: query.End},
	)

	// Add filter tag conditions
	for i, filter := range query.FilterTags {
		labelKey := UniversalTag(filter.Key).ToGcp()
		keyParam := fmt.Sprintf("filter_key_%d", i)
		valParam := fmt.Sprintf("filter_vals_%d", i)
		whereClauses = append(whereClauses,
			fmt.Sprintf("EXISTS(SELECT 1 FROM UNNEST(labels) l WHERE l.key = @%s AND l.value IN UNNEST(@%s))", keyParam, valParam),
		)
		params = append(params,
			bigquery.QueryParameter{Name: keyParam, Value: labelKey},
			bigquery.QueryParameter{Name: valParam, Value: filter.Values},
		)
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
