package aws_data_export

import (
	"context"
	"strconv"
	"testing"
	"time"

	infra_sdk "github.com/nullstone-io/infra-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoster_ScanCosts(t *testing.T) {
	ctx := context.Background()
	query := infra_sdk.CostQuery{
		Start: aug2026, End: sep2026, Granularity: infra_sdk.CostGranularityDaily,
		GroupBy: infra_sdk.CostGroupIdentifiers{groupByAccount, groupByStack, groupByEnv, groupByBlock, groupByService, groupByChargeCategory, groupByResource},
	}
	coster := testCoster(t)

	rows := []infra_sdk.CostRow{}
	require.NoError(t, coster.ScanCosts(ctx, query, func(row infra_sdk.CostRow) error {
		rows = append(rows, row)
		return nil
	}))

	// 9 fixture rows, two of which share every group value -> 8 buckets, each one row with all four measures
	require.Len(t, rows, 8)
	var ec2 *infra_sdk.CostRow
	for i := range rows {
		assert.Len(t, rows[i].Values, 4)
		for _, key := range rows[i].GroupKeys {
			if key.Name == infra_sdk.UniversalDimensionResource && key.Value == "i-0001" {
				ec2 = &rows[i]
			}
		}
	}
	require.NotNil(t, ec2)
	assert.Equal(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), ec2.Start)
	assert.Equal(t, time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC), ec2.End)
	assert.Equal(t, "USD", ec2.Currency)
	assert.Equal(t, map[infra_sdk.CostMetric]float64{
		infra_sdk.CostMetricListCost: 10, infra_sdk.CostMetricContractedCost: 9,
		infra_sdk.CostMetricEffectiveCost: 8, infra_sdk.CostMetricBilledCost: 8,
	}, ec2.Values)

	t.Run("rows carry the same numbers as the series", func(t *testing.T) {
		res, err := coster.GetCosts(ctx, query)
		require.NoError(t, err)
		seen := 0
		for _, row := range rows {
			for metric, value := range row.Values {
				series, ok := res.Series[infra_sdk.CostSeriesKey(row.GroupKeys, metric)]
				require.True(t, ok, "row has no series for %s", metric)
				for _, point := range series.Points {
					if point.Start.Equal(row.Start) && point.Unit == row.Currency {
						want, err := strconv.ParseFloat(point.Value, 64)
						require.NoError(t, err)
						assert.Equal(t, want, value)
						seen++
					}
				}
			}
		}
		assert.Equal(t, 8*4, seen)
	})

	t.Run("a row's error stops the scan", func(t *testing.T) {
		calls := 0
		err := coster.ScanCosts(ctx, query, func(infra_sdk.CostRow) error { calls++; return assert.AnError })
		assert.ErrorIs(t, err, assert.AnError)
		assert.Equal(t, 1, calls)
	})

	t.Run("hourly is refused like GetCosts", func(t *testing.T) {
		hourly := query
		hourly.Granularity = infra_sdk.CostGranularityHourly
		assert.Error(t, coster.ScanCosts(ctx, hourly, func(infra_sdk.CostRow) error { return nil }))
	})
}
