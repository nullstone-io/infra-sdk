package infra_sdk

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCostResult_EachRow(t *testing.T) {
	day1 := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	day2 := day1.AddDate(0, 0, 1)
	point := func(start time.Time, value string) CostSeriesDatapoint {
		return CostSeriesDatapoint{Start: start, End: start.AddDate(0, 0, 1), Unit: "USD", Value: value}
	}
	api := CostSeriesGroupKeys{{TagKey: UniversalTagStack, Value: "core"}, {Name: UniversalDimensionService, Value: "Amazon ECS"}}
	s3 := CostSeriesGroupKeys{{TagKey: UniversalTagStack, Value: ""}, {Name: UniversalDimensionService, Value: "Amazon S3"}}

	res := NewCostResult()
	// two metrics over two days for one group, one metric for another; added out of order
	res.AddDatapoint(CostMetricBilledCost, api, point(day2, "4"))
	res.AddDatapoint(CostMetricEffectiveCost, api, point(day1, "3"))
	res.AddDatapoint(CostMetricBilledCost, api, point(day1, "3.5"))
	res.AddDatapoint(CostMetricEffectiveCost, api, point(day2, "4"))
	res.AddDatapoint(CostMetricBilledCost, s3, point(day1, "1"))

	rows := []CostRow{}
	require.NoError(t, res.EachRow(func(row CostRow) error {
		rows = append(rows, row)
		return nil
	}))

	// one row per (group, period, currency), every reported metric on it, periods in order
	require.Len(t, rows, 3)
	assert.Equal(t, s3, rows[0].GroupKeys, "groups are emitted in identifier order")
	assert.Equal(t, map[CostMetric]float64{CostMetricBilledCost: 1}, rows[0].Values)
	assert.Equal(t, api, rows[1].GroupKeys)
	assert.Equal(t, day1, rows[1].Start)
	assert.Equal(t, day2, rows[1].End)
	assert.Equal(t, "USD", rows[1].Currency)
	assert.Equal(t, map[CostMetric]float64{CostMetricBilledCost: 3.5, CostMetricEffectiveCost: 3}, rows[1].Values)
	assert.Equal(t, day2, rows[2].Start)
	assert.Equal(t, map[CostMetric]float64{CostMetricBilledCost: 4, CostMetricEffectiveCost: 4}, rows[2].Values)

	t.Run("a row's error stops the scan", func(t *testing.T) {
		calls := 0
		err := res.EachRow(func(CostRow) error { calls++; return assert.AnError })
		assert.ErrorIs(t, err, assert.AnError)
		assert.Equal(t, 1, calls)
	})

	t.Run("an unparsable value is an error", func(t *testing.T) {
		bad := NewCostResult()
		bad.AddDatapoint(CostMetricBilledCost, nil, point(day1, "n/a"))
		assert.Error(t, bad.EachRow(func(CostRow) error { return nil }))
	})

	t.Run("nil result has no rows", func(t *testing.T) {
		var none *CostResult
		assert.NoError(t, none.EachRow(func(CostRow) error { return assert.AnError }))
	})
}
