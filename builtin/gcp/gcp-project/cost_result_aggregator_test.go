package gcp_project

import (
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	infra_sdk "github.com/nullstone-io/infra-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCostResultAggregator_AddRow_NoGroupBy(t *testing.T) {
	agg := NewCostResultAggregator()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	row := map[string]bigquery.Value{
		"period_start": start,
		"period_end":   end,
		"total_cost":   1.23,
		"currency":     "USD",
	}

	err := agg.AddRow(row, nil)
	require.NoError(t, err)
	require.Len(t, agg.CostResult.Series, 1)

	for _, series := range agg.CostResult.Series {
		assert.Equal(t, "UnblendedCost", series.MetricName)
		require.Len(t, series.Points, 1)
		assert.Equal(t, start, series.Points[0].Start)
		assert.Equal(t, end, series.Points[0].End)
		assert.Equal(t, "USD", series.Points[0].Unit)
		assert.Contains(t, series.Points[0].Value, "1.23")
	}
}

func TestCostResultAggregator_AddRow_WithDimensionGroupBy(t *testing.T) {
	agg := NewCostResultAggregator()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	row := map[string]bigquery.Value{
		"period_start": start,
		"period_end":   end,
		"total_cost":   5.67,
		"currency":     "USD",
		"dim_0":        "my-project-id",
	}

	groupBy := infra_sdk.CostGroupIdentifiers{
		{Dimension: infra_sdk.UniversalDimensionAccount},
	}

	err := agg.AddRow(row, groupBy)
	require.NoError(t, err)
	require.Len(t, agg.CostResult.Series, 1)

	for _, series := range agg.CostResult.Series {
		require.Len(t, series.GroupKeys, 1)
		assert.Equal(t, infra_sdk.UniversalDimensionAccount, series.GroupKeys[0].Name)
		assert.Equal(t, "my-project-id", series.GroupKeys[0].Value)
	}
}

func TestCostResultAggregator_AddRow_WithTagGroupBy(t *testing.T) {
	agg := NewCostResultAggregator()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	row := map[string]bigquery.Value{
		"period_start": start,
		"period_end":   end,
		"total_cost":   2.50,
		"currency":     "USD",
		"label_0":      "production",
	}

	groupBy := infra_sdk.CostGroupIdentifiers{
		{TagKey: infra_sdk.UniversalTagEnv},
	}

	err := agg.AddRow(row, groupBy)
	require.NoError(t, err)
	require.Len(t, agg.CostResult.Series, 1)

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

	rows := []map[string]bigquery.Value{
		{
			"period_start": time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			"period_end":   time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
			"total_cost":   1.00,
			"currency":     "USD",
			"label_0":      "dev",
		},
		{
			"period_start": time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			"period_end":   time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
			"total_cost":   3.00,
			"currency":     "USD",
			"label_0":      "prod",
		},
	}

	for _, row := range rows {
		err := agg.AddRow(row, groupBy)
		require.NoError(t, err)
	}

	// Two different env values should produce two series
	assert.Len(t, agg.CostResult.Series, 2)
}
