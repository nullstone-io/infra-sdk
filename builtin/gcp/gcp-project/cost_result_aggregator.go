package gcp_project

import (
	"fmt"
	"time"

	"cloud.google.com/go/bigquery"
	infra_sdk "github.com/nullstone-io/infra-sdk"
)

func NewCostResultAggregator() *CostResultAggregator {
	return &CostResultAggregator{
		CostResult: infra_sdk.NewCostResult(),
	}
}

type CostResultAggregator struct {
	CostResult *infra_sdk.CostResult
}

// costRow represents a single row from the BigQuery billing export query.
// Field names must match the column aliases produced by QueryBuilder.
type costRow struct {
	PeriodStart bigquery.NullTimestamp `bigquery:"period_start"`
	PeriodEnd   bigquery.NullTimestamp `bigquery:"period_end"`
	TotalCost   float64                `bigquery:"total_cost"`
	Currency    bigquery.NullString    `bigquery:"currency"`
	// Dynamic group-by columns are read separately via row.Columns
}

func (a *CostResultAggregator) AddRow(row map[string]bigquery.Value, groupBy infra_sdk.CostGroupIdentifiers) error {
	periodStart, err := toTime(row["period_start"])
	if err != nil {
		return fmt.Errorf("error parsing period_start: %w", err)
	}
	periodEnd, err := toTime(row["period_end"])
	if err != nil {
		return fmt.Errorf("error parsing period_end: %w", err)
	}

	totalCost, _ := row["total_cost"].(float64)
	currency, _ := row["currency"].(string)

	groupKeys := a.parseGroupKeys(row, groupBy)

	a.CostResult.AddDatapoint("UnblendedCost", groupKeys, infra_sdk.CostSeriesDatapoint{
		Start: periodStart,
		End:   periodEnd,
		Unit:  currency,
		Value: fmt.Sprintf("%f", totalCost),
	})

	return nil
}

func (a *CostResultAggregator) parseGroupKeys(row map[string]bigquery.Value, groupBy infra_sdk.CostGroupIdentifiers) infra_sdk.CostSeriesGroupKeys {
	result := make(infra_sdk.CostSeriesGroupKeys, 0, len(groupBy))
	for i, grp := range groupBy {
		if grp.Dimension != "" {
			alias := fmt.Sprintf("dim_%d", i)
			value, _ := row[alias].(string)
			result = append(result, infra_sdk.CostSeriesGroupKey{
				Name:  GcpDimension(UniversalDimension(grp.Dimension).ToGcpColumn()).ToUniversal(),
				Value: value,
			})
		} else if grp.TagKey != "" {
			alias := fmt.Sprintf("label_%d", i)
			value, _ := row[alias].(string)
			result = append(result, infra_sdk.CostSeriesGroupKey{
				TagKey: grp.TagKey,
				Value:  value,
			})
		}
	}
	return result
}

func toTime(v bigquery.Value) (time.Time, error) {
	if v == nil {
		return time.Time{}, fmt.Errorf("nil value")
	}
	switch t := v.(type) {
	case time.Time:
		return t, nil
	default:
		return time.Time{}, fmt.Errorf("unexpected type %T", v)
	}
}
