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

// metricColumns maps the measure columns produced by QueryBuilder onto FOCUS metrics.
var metricColumns = []struct {
	column string
	metric infra_sdk.CostMetric
}{
	{"list_cost", infra_sdk.CostMetricListCost},
	{"contracted_cost", infra_sdk.CostMetricContractedCost},
	{"effective_cost", infra_sdk.CostMetricEffectiveCost},
	{"billed_cost", infra_sdk.CostMetricBilledCost},
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

	currency, _ := row["currency"].(string)
	groupKeys := a.parseGroupKeys(row, groupBy)

	for _, mc := range metricColumns {
		value, ok := row[mc.column].(float64)
		if !ok {
			// a NULL aggregate means the export has no rows for this measure; report nothing rather than zero
			continue
		}
		a.CostResult.AddDatapoint(mc.metric, groupKeys, infra_sdk.CostSeriesDatapoint{
			Start: periodStart,
			End:   periodEnd,
			Unit:  currency,
			Value: fmt.Sprintf("%f", value),
		})
	}

	return nil
}

func (a *CostResultAggregator) parseGroupKeys(row map[string]bigquery.Value, groupBy infra_sdk.CostGroupIdentifiers) infra_sdk.CostSeriesGroupKeys {
	result := make(infra_sdk.CostSeriesGroupKeys, 0, len(groupBy))
	for i, grp := range groupBy {
		if grp.Dimension != "" {
			alias := fmt.Sprintf("dim_%d", i)
			value, _ := row[alias].(string)
			if grp.Dimension == infra_sdk.UniversalDimensionChargeCategory {
				value = string(GcpCostType(value).ToChargeCategory())
			}
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
