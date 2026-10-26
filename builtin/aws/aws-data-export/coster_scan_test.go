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

	// the row is reused between calls, so a test that keeps rows must copy them
	type kept struct {
		id       string
		keys     infra_sdk.CostSeriesGroupKeys
		start    time.Time
		end      time.Time
		currency string
		values   map[infra_sdk.CostMetric]float64
	}
	rows := []kept{}
	require.NoError(t, coster.ScanCosts(ctx, query, func(row infra_sdk.CostRow) error {
		k := kept{id: row.GroupKeys.UniqueIdentifier(), keys: append(infra_sdk.CostSeriesGroupKeys{}, row.GroupKeys...), start: row.Start, end: row.End, currency: row.Currency, values: map[infra_sdk.CostMetric]float64{}}
		for metric, value := range row.Values {
			k.values[metric] = value
		}
		rows = append(rows, k)
		return nil
	}))

	// every fixture line item arrives as its own row: nothing is summed on the way out
	require.Len(t, rows, 9)
	var ec2 *kept
	for i := range rows {
		assert.Len(t, rows[i].values, 4)
		for _, key := range rows[i].keys {
			if key.Name == infra_sdk.UniversalDimensionResource && key.Value == "i-0001" {
				ec2 = &rows[i]
			}
		}
	}
	require.NotNil(t, ec2)
	assert.Equal(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), ec2.start)
	assert.Equal(t, time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC), ec2.end)
	assert.Equal(t, "USD", ec2.currency)
	assert.Equal(t, map[infra_sdk.CostMetric]float64{
		infra_sdk.CostMetricListCost: 10, infra_sdk.CostMetricContractedCost: 9,
		infra_sdk.CostMetricEffectiveCost: 8, infra_sdk.CostMetricBilledCost: 8,
	}, ec2.values)

	t.Run("rows summed by key equal the series", func(t *testing.T) {
		res, err := coster.GetCosts(ctx, query)
		require.NoError(t, err)

		type bucket struct {
			id       string
			start    time.Time
			currency string
		}
		sums := map[bucket]map[infra_sdk.CostMetric]float64{}
		keys := map[bucket]infra_sdk.CostSeriesGroupKeys{}
		for _, row := range rows {
			b := bucket{id: row.id, start: row.start, currency: row.currency}
			if sums[b] == nil {
				sums[b] = map[infra_sdk.CostMetric]float64{}
				keys[b] = row.keys
			}
			for metric, value := range row.values {
				sums[b][metric] += value
			}
		}
		require.Len(t, sums, 8, "two fixture rows share every group value and the day")

		seen := 0
		for b, values := range sums {
			for metric, value := range values {
				series, ok := res.Series[infra_sdk.CostSeriesKey(keys[b], metric)]
				require.True(t, ok, "no series for %s %s", b.id, metric)
				for _, point := range series.Points {
					if point.Start.Equal(b.start) && point.Unit == b.currency {
						want, err := strconv.ParseFloat(point.Value, 64)
						require.NoError(t, err)
						assert.InDelta(t, want, value, 1e-9)
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

	t.Run("filters and the window apply to the stream too", func(t *testing.T) {
		filtered := query
		filtered.FilterTags = []infra_sdk.CostFilterTag{{Key: infra_sdk.UniversalTagStack, Values: []string{"core"}}}
		n := 0
		require.NoError(t, coster.ScanCosts(ctx, filtered, func(row infra_sdk.CostRow) error {
			for _, key := range row.GroupKeys {
				if key.TagKey == infra_sdk.UniversalTagStack {
					assert.Equal(t, "core", key.Value)
				}
			}
			n++
			return nil
		}))
		assert.Less(t, n, 9)
		assert.Greater(t, n, 0)
	})

	t.Run("hourly is refused like GetCosts", func(t *testing.T) {
		hourly := query
		hourly.Granularity = infra_sdk.CostGranularityHourly
		assert.Error(t, coster.ScanCosts(ctx, hourly, func(infra_sdk.CostRow) error { return nil }))
	})
}
