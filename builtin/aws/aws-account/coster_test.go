package aws_account

import (
	"testing"

	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	infra_sdk "github.com/nullstone-io/infra-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCostQueryToFilter(t *testing.T) {
	t.Run("no filters", func(t *testing.T) {
		assert.Nil(t, costQueryToFilter(infra_sdk.CostQuery{}))
	})

	t.Run("single present filter", func(t *testing.T) {
		expr := costQueryToFilter(infra_sdk.CostQuery{FilterTags: []infra_sdk.CostFilterTag{
			{Key: infra_sdk.UniversalTagStack, Values: []string{"core"}},
		}})
		require.NotNil(t, expr)
		require.NotNil(t, expr.Tags)
		assert.Equal(t, "Stack", *expr.Tags.Key)
		assert.Equal(t, []string{"core"}, expr.Tags.Values)
		assert.Equal(t, []cetypes.MatchOption{cetypes.MatchOptionEquals}, expr.Tags.MatchOptions)
	})

	t.Run("absent filter uses ABSENT match option", func(t *testing.T) {
		// An empty value asks for untagged resources; Cost Explorer has a dedicated match option
		expr := costQueryToFilter(infra_sdk.CostQuery{FilterTags: []infra_sdk.CostFilterTag{
			{Key: infra_sdk.UniversalTagStack, Values: []string{""}},
		}})
		require.NotNil(t, expr)
		require.NotNil(t, expr.Tags)
		assert.Equal(t, "Stack", *expr.Tags.Key)
		assert.Empty(t, expr.Tags.Values)
		assert.Equal(t, []cetypes.MatchOption{cetypes.MatchOptionAbsent}, expr.Tags.MatchOptions)
	})

	t.Run("present and absent are OR'd", func(t *testing.T) {
		expr := costQueryToFilter(infra_sdk.CostQuery{FilterTags: []infra_sdk.CostFilterTag{
			{Key: infra_sdk.UniversalTagEnv, Values: []string{"prod", ""}},
		}})
		require.NotNil(t, expr)
		require.Len(t, expr.Or, 2)
		assert.Equal(t, []string{"prod"}, expr.Or[0].Tags.Values)
		assert.Equal(t, []cetypes.MatchOption{cetypes.MatchOptionAbsent}, expr.Or[1].Tags.MatchOptions)
	})

	t.Run("multiple filters are AND'd", func(t *testing.T) {
		expr := costQueryToFilter(infra_sdk.CostQuery{FilterTags: []infra_sdk.CostFilterTag{
			{Key: infra_sdk.UniversalTagStack, Values: []string{"core"}},
			{Key: infra_sdk.UniversalTagEnv, Values: []string{""}},
		}})
		require.NotNil(t, expr)
		require.Len(t, expr.And, 2)
		assert.Equal(t, "Stack", *expr.And[0].Tags.Key)
		assert.Equal(t, "Env", *expr.And[1].Tags.Key)
		assert.Equal(t, []cetypes.MatchOption{cetypes.MatchOptionAbsent}, expr.And[1].Tags.MatchOptions)
	})
}

func TestCostResultAggregator_AddResults_MapsMetricsAndRecordTypes(t *testing.T) {
	agg := NewCostResultAggregator()
	groupBy := infra_sdk.CostGroupIdentifiers{{Dimension: infra_sdk.UniversalDimensionChargeCategory}}
	err := agg.AddResults([]cetypes.ResultByTime{
		{
			TimePeriod: &cetypes.DateInterval{Start: ptr("2026-01-01"), End: ptr("2026-01-02")},
			Groups: []cetypes.Group{
				{
					Keys: []string{"Tax"},
					Metrics: map[string]cetypes.MetricValue{
						"NetUnblendedCost": {Amount: ptr("1.5"), Unit: ptr("USD")},
						"NetAmortizedCost": {Amount: ptr("1.5"), Unit: ptr("USD")},
						"UsageQuantity":    {Amount: ptr("3"), Unit: ptr("N/A")},
					},
				},
			},
		},
	}, groupBy)
	require.NoError(t, err)

	byMetric := map[infra_sdk.CostMetric]infra_sdk.CostSeries{}
	for _, series := range agg.CostResult.Series {
		byMetric[series.MetricName] = series
	}
	// Only the FOCUS measures we can derive are reported; UsageQuantity is dropped
	require.Len(t, byMetric, 2)
	assert.Equal(t, "1.5", byMetric[infra_sdk.CostMetricBilledCost].Points[0].Value)
	assert.Equal(t, "1.5", byMetric[infra_sdk.CostMetricEffectiveCost].Points[0].Value)

	// RECORD_TYPE values are translated to the FOCUS ChargeCategory
	keys := byMetric[infra_sdk.CostMetricBilledCost].GroupKeys
	require.Len(t, keys, 1)
	assert.Equal(t, infra_sdk.UniversalDimensionChargeCategory, keys[0].Name)
	assert.Equal(t, string(infra_sdk.CostChargeCategoryTax), keys[0].Value)
}
