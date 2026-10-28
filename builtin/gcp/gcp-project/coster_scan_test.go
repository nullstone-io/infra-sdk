package gcp_project

import (
	"errors"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	infra_sdk "github.com/nullstone-io/infra-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/iterator"
)

// fakeIterator yields rows the way bigquery.RowIterator does: one per Next, then iterator.Done.
type fakeIterator struct {
	rows []map[string]bigquery.Value
	err  error
	i    int
}

func (f *fakeIterator) Next(dst interface{}) error {
	if f.i >= len(f.rows) {
		if f.err != nil {
			return f.err
		}
		return iterator.Done
	}
	*(dst.(*map[string]bigquery.Value)) = f.rows[f.i]
	f.i++
	return nil
}

var (
	scanDay1 = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	scanDay2 = time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	scanDay3 = time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
)

var scanGroupBy = infra_sdk.CostGroupIdentifiers{
	{Dimension: infra_sdk.UniversalDimensionService},
	{Dimension: infra_sdk.UniversalDimensionChargeCategory},
	{TagKey: "nullstone.io/stack"},
}

func scanRows() []map[string]bigquery.Value {
	return []map[string]bigquery.Value{
		metricRow(scanDay1, scanDay2, map[string]bigquery.Value{"dim_0": "Compute Engine", "dim_1": "regular", "label_2": "core"}),
		metricRow(scanDay1, scanDay2, map[string]bigquery.Value{"dim_0": "Cloud SQL", "dim_1": "regular", "label_2": "core", "contracted_cost": nil}),
		metricRow(scanDay2, scanDay3, map[string]bigquery.Value{"dim_0": "Compute Engine", "dim_1": "tax", "label_2": "", "list_cost": 1.25, "billed_cost": 1.25}),
	}
}

func TestToCostRow(t *testing.T) {
	row, err := toCostRow(scanRows()[1], scanGroupBy)
	require.NoError(t, err)

	assert.Equal(t, scanDay1, row.Start)
	assert.Equal(t, scanDay2, row.End)
	assert.Equal(t, "USD", row.Currency)
	assert.Equal(t, infra_sdk.CostSeriesGroupKeys{
		{Name: infra_sdk.UniversalDimensionService, Value: "Cloud SQL"},
		{Name: infra_sdk.UniversalDimensionChargeCategory, Value: string(infra_sdk.CostChargeCategoryUsage)},
		{TagKey: "nullstone.io/stack", Value: "core"},
	}, row.GroupKeys, "group keys follow the query's order with the charge category translated")
	assert.Equal(t, map[infra_sdk.CostMetric]float64{
		infra_sdk.CostMetricListCost:      10.0,
		infra_sdk.CostMetricEffectiveCost: 7.5,
		infra_sdk.CostMetricBilledCost:    7.0,
	}, row.Values, "a NULL measure is absent, never zero")
}

func TestToCostRow_RejectsMissingPeriod(t *testing.T) {
	_, err := toCostRow(map[string]bigquery.Value{"period_end": scanDay2}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "period_start")
}

func TestDrain_EmitsEveryRowInOrder(t *testing.T) {
	var got []string
	err := drain(&fakeIterator{rows: scanRows()}, func(row map[string]bigquery.Value) error {
		got = append(got, row["dim_0"].(string)+"/"+row["dim_1"].(string))
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"Compute Engine/regular", "Cloud SQL/regular", "Compute Engine/tax"}, got)
}

func TestDrain_StopsOnConsumerError(t *testing.T) {
	boom := errors.New("boom")
	calls := 0
	err := drain(&fakeIterator{rows: scanRows()}, func(map[string]bigquery.Value) error {
		calls++
		return boom
	})
	assert.ErrorIs(t, err, boom)
	assert.Equal(t, 1, calls, "the first error stops the read")
}

func TestDrain_WrapsIteratorError(t *testing.T) {
	cause := errors.New("context canceled")
	err := drain(&fakeIterator{rows: scanRows()[:1], err: cause}, func(map[string]bigquery.Value) error { return nil })
	require.ErrorIs(t, err, cause)
	assert.Contains(t, err.Error(), "error reading bigquery result row")
}

// The scanned rows must carry exactly what GetCosts would have folded into series: a consumer
// summing rows by key reads the same numbers either way.
func TestScan_MatchesAggregatedResult(t *testing.T) {
	aggregator := NewCostResultAggregator()
	var scanned []infra_sdk.CostRow
	err := drain(&fakeIterator{rows: scanRows()}, func(row map[string]bigquery.Value) error {
		require.NoError(t, aggregator.AddRow(row, scanGroupBy))
		costRow, err := toCostRow(row, scanGroupBy)
		if err != nil {
			return err
		}
		scanned = append(scanned, costRow)
		return nil
	})
	require.NoError(t, err)

	var regrouped []infra_sdk.CostRow
	require.NoError(t, aggregator.CostResult.EachRow(func(row infra_sdk.CostRow) error {
		regrouped = append(regrouped, row)
		return nil
	}))
	require.Len(t, regrouped, len(scanned))

	byKey := func(rows []infra_sdk.CostRow) map[string]infra_sdk.CostRow {
		m := map[string]infra_sdk.CostRow{}
		for _, r := range rows {
			m[r.GroupKeys.UniqueIdentifier()+"|"+r.Start.Format(time.RFC3339)+"|"+r.Currency] = r
		}
		return m
	}
	want, got := byKey(regrouped), byKey(scanned)
	require.Len(t, got, len(want))
	for key, w := range want {
		g, ok := got[key]
		require.True(t, ok, "scanned rows lack %s", key)
		assert.Equal(t, w.End, g.End, key)
		require.Len(t, g.Values, len(w.Values), key)
		for metric, value := range w.Values {
			assert.InDelta(t, value, g.Values[metric], 1e-6, "%s %s", key, metric)
		}
	}
}
