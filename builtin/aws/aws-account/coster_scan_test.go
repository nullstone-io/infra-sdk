package aws_account

import (
	"context"
	"strconv"
	"testing"
	"time"

	infra_sdk "github.com/nullstone-io/infra-sdk"
	aws_data_export "github.com/nullstone-io/infra-sdk/builtin/aws/aws-data-export"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// valuedSource answers one monthly BilledCost point per month at a fixed value through GetCosts
// only, so the router has to fall back to EachRow for it.
type valuedSource struct {
	value   float64
	queries []infra_sdk.CostQuery
}

func (f *valuedSource) GetCosts(ctx context.Context, query infra_sdk.CostQuery) (*infra_sdk.CostResult, error) {
	f.queries = append(f.queries, query)
	res := infra_sdk.NewCostResult()
	for _, month := range aws_data_export.MonthsInRange(query.Start, query.End) {
		res.AddDatapoint(infra_sdk.CostMetricBilledCost, nil, infra_sdk.CostSeriesDatapoint{
			Start: month, End: month.AddDate(0, 1, 0), Unit: "USD", Value: fmtFloat(f.value),
		})
	}
	return res, nil
}

// scanningSource is an export that also streams; the router must prefer the stream.
type scanningSource struct {
	valuedSource
	delivered map[string]string
	scans     []infra_sdk.CostQuery
}

func (f *scanningSource) HasMonth(ctx context.Context, month time.Time) (string, bool, error) {
	etag, ok := f.delivered[month.Format("2006-01")]
	return etag, ok, nil
}

func (f *scanningSource) ScanCosts(ctx context.Context, query infra_sdk.CostQuery, fn func(infra_sdk.CostRow) error) error {
	f.scans = append(f.scans, query)
	for _, month := range aws_data_export.MonthsInRange(query.Start, query.End) {
		row := infra_sdk.CostRow{Start: month, End: month.AddDate(0, 1, 0), Currency: "USD", Values: map[infra_sdk.CostMetric]float64{infra_sdk.CostMetricBilledCost: f.value}}
		if err := fn(row); err != nil {
			return err
		}
	}
	return nil
}

func fmtFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func TestCoster_ScanCosts_Routing(t *testing.T) {
	collect := func(c Coster, q infra_sdk.CostQuery) []string {
		out := []string{}
		require.NoError(t, c.ScanCosts(context.Background(), q, func(row infra_sdk.CostRow) error {
			out = append(out, row.Start.Format("2006-01")+"="+fmtFloat(row.Values[infra_sdk.CostMetricBilledCost]))
			return nil
		}))
		return out
	}

	t.Run("no export reads cost explorer through its result", func(t *testing.T) {
		ce := &valuedSource{value: 1}
		c := Coster{costExplorer: ce}
		q := infra_sdk.CostQuery{Start: day(2026, 7, 1), End: day(2026, 9, 1), Granularity: infra_sdk.CostGranularityMonthly}
		assert.Equal(t, []string{"2026-07=1", "2026-08=1"}, collect(c, q))
		require.Len(t, ce.queries, 1)
		assert.Equal(t, q, ce.queries[0])
	})

	t.Run("mixed window streams the exported stretch and reads the rest", func(t *testing.T) {
		ce := &valuedSource{value: 1}
		export := &scanningSource{valuedSource: valuedSource{value: 2}, delivered: map[string]string{"2026-09": "etag", "2026-10": "etag"}}
		c := Coster{costExplorer: ce, export: export}
		q := infra_sdk.CostQuery{Start: day(2026, 7, 15), End: day(2026, 10, 10), Granularity: infra_sdk.CostGranularityMonthly}

		assert.Equal(t, []string{"2026-07=1", "2026-08=1", "2026-09=2", "2026-10=2"}, collect(c, q))
		require.Len(t, ce.queries, 1)
		assert.Equal(t, day(2026, 9, 1), ce.queries[0].End)
		require.Len(t, export.scans, 1, "the export's scanner is used, not its result")
		assert.Empty(t, export.queries)
		assert.Equal(t, day(2026, 9, 1), export.scans[0].Start)
	})

	t.Run("hourly always goes to cost explorer", func(t *testing.T) {
		ce := &valuedSource{value: 1}
		export := &scanningSource{valuedSource: valuedSource{value: 2}, delivered: map[string]string{"2026-09": "etag"}}
		c := Coster{costExplorer: ce, export: export}
		q := infra_sdk.CostQuery{Start: day(2026, 9, 1), End: day(2026, 10, 1), Granularity: infra_sdk.CostGranularityHourly}
		assert.Equal(t, []string{"2026-09=1"}, collect(c, q))
		assert.Empty(t, export.scans)
	})
}
