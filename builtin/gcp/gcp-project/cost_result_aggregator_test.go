package gcp_project

import (
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	infra_sdk "github.com/nullstone-io/infra-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func metricRow(start, end time.Time, extra map[string]bigquery.Value) map[string]bigquery.Value {
	row := map[string]bigquery.Value{
		"period_start":    start,
		"period_end":      end,
		"list_cost":       10.0,
		"contracted_cost": 9.0,
		"effective_cost":  7.5,
		"billed_cost":     7.0,
		"currency":        "USD",
	}
	for k, v := range extra {
		row[k] = v
	}
	return row
}

func seriesByMetric(result *infra_sdk.CostResult) map[infra_sdk.CostMetric]infra_sdk.CostSeries {
	byMetric := map[infra_sdk.CostMetric]infra_sdk.CostSeries{}
	for _, series := range result.Series {
		byMetric[series.MetricName] = series
	}
	return byMetric
}

func TestCostResultAggregator_AddRow_NoGroupBy(t *testing.T) {
	agg := NewCostResultAggregator()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	err := agg.AddRow(metricRow(start, end, nil), nil)
	require.NoError(t, err)

	// One row fans out into one series per FOCUS measure
	byMetric := seriesByMetric(agg.CostResult)
	require.Len(t, byMetric, 4)
	assert.Contains(t, byMetric[infra_sdk.CostMetricListCost].Points[0].Value, "10.0")
	assert.Contains(t, byMetric[infra_sdk.CostMetricContractedCost].Points[0].Value, "9.0")
	assert.Contains(t, byMetric[infra_sdk.CostMetricEffectiveCost].Points[0].Value, "7.5")
	assert.Contains(t, byMetric[infra_sdk.CostMetricBilledCost].Points[0].Value, "7.0")

	for _, series := range byMetric {
		require.Len(t, series.Points, 1)
		assert.Equal(t, start, series.Points[0].Start)
		assert.Equal(t, end, series.Points[0].End)
		assert.Equal(t, "USD", series.Points[0].Unit)
	}
}

func TestCostResultAggregator_AddRow_NullMeasureIsOmitted(t *testing.T) {
	agg := NewCostResultAggregator()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	// A NULL aggregate (older exports without cost_at_list) must not become a zero datapoint:
	// zero would read as a 100% discount.
	row := metricRow(start, end, map[string]bigquery.Value{"list_cost": nil})
	err := agg.AddRow(row, nil)
	require.NoError(t, err)

	byMetric := seriesByMetric(agg.CostResult)
	require.Len(t, byMetric, 3)
	_, hasList := byMetric[infra_sdk.CostMetricListCost]
	assert.False(t, hasList)
}

func TestCostResultAggregator_AddRow_WithDimensionGroupBy(t *testing.T) {
	agg := NewCostResultAggregator()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	row := metricRow(start, end, map[string]bigquery.Value{"dim_0": "my-project-id"})
	groupBy := infra_sdk.CostGroupIdentifiers{
		{Dimension: infra_sdk.UniversalDimensionAccount},
	}

	err := agg.AddRow(row, groupBy)
	require.NoError(t, err)
	require.Len(t, agg.CostResult.Series, 4)

	for _, series := range agg.CostResult.Series {
		require.Len(t, series.GroupKeys, 1)
		assert.Equal(t, infra_sdk.UniversalDimensionAccount, series.GroupKeys[0].Name)
		assert.Equal(t, "my-project-id", series.GroupKeys[0].Value)
	}
}

func TestCostResultAggregator_AddRow_ChargeCategoryIsTranslated(t *testing.T) {
	agg := NewCostResultAggregator()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	row := metricRow(start, end, map[string]bigquery.Value{"dim_0": "tax"})
	groupBy := infra_sdk.CostGroupIdentifiers{
		{Dimension: infra_sdk.UniversalDimensionChargeCategory},
	}

	err := agg.AddRow(row, groupBy)
	require.NoError(t, err)

	for _, series := range agg.CostResult.Series {
		require.Len(t, series.GroupKeys, 1)
		assert.Equal(t, infra_sdk.UniversalDimensionChargeCategory, series.GroupKeys[0].Name)
		assert.Equal(t, string(infra_sdk.CostChargeCategoryTax), series.GroupKeys[0].Value)
	}
}

func TestCostResultAggregator_AddRow_WithTagGroupBy(t *testing.T) {
	agg := NewCostResultAggregator()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	row := metricRow(start, end, map[string]bigquery.Value{"label_0": "production"})
	groupBy := infra_sdk.CostGroupIdentifiers{
		{TagKey: infra_sdk.UniversalTagEnv},
	}

	err := agg.AddRow(row, groupBy)
	require.NoError(t, err)
	require.Len(t, agg.CostResult.Series, 4)

	for _, series := range agg.CostResult.Series {
		require.Len(t, series.GroupKeys, 1)
		assert.Equal(t, infra_sdk.UniversalTagEnv, series.GroupKeys[0].TagKey)
		assert.Equal(t, "production", series.GroupKeys[0].Value)
	}
}

func TestCostResultAggregator_AddRow_MultipleRows(t *testing.T) {
	agg := NewCostResultAggregator()
	groupBy := infra_sdk.CostGroupIdentifiers{
		{TagKey: infra_sdk.UniversalTagEnv},
	}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	rows := []map[string]bigquery.Value{
		metricRow(start, end, map[string]bigquery.Value{"label_0": "dev"}),
		metricRow(start, end, map[string]bigquery.Value{"label_0": "prod"}),
	}

	for _, row := range rows {
		err := agg.AddRow(row, groupBy)
		require.NoError(t, err)
	}

	// Two env values x four measures
	assert.Len(t, agg.CostResult.Series, 8)
}
