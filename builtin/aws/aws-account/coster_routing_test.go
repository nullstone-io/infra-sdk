package aws_account

import (
	"context"
	"testing"
	"time"

	infra_sdk "github.com/nullstone-io/infra-sdk"
	aws_data_export "github.com/nullstone-io/infra-sdk/builtin/aws/aws-data-export"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSource struct {
	name    string
	queries []infra_sdk.CostQuery
	// delivered lists the months (yyyy-mm) an export has; ignored for Cost Explorer.
	delivered map[string]string
}

func (f *fakeSource) GetCosts(ctx context.Context, query infra_sdk.CostQuery) (*infra_sdk.CostResult, error) {
	f.queries = append(f.queries, query)
	res := infra_sdk.NewCostResult()
	// one monthly point per month in the window, valued by the source so a merge is visible
	for _, month := range aws_data_export.MonthsInRange(query.Start, query.End) {
		res.AddDatapoint(infra_sdk.CostMetricBilledCost, nil, infra_sdk.CostSeriesDatapoint{
			Start: month, End: month.AddDate(0, 1, 0), Unit: "USD", Value: f.name,
		})
	}
	return res, nil
}

func (f *fakeSource) HasMonth(ctx context.Context, month time.Time) (string, bool, error) {
	etag, ok := f.delivered[month.Format("2006-01")]
	return etag, ok, nil
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestCoster_MonthSource(t *testing.T) {
	ce := &fakeSource{name: "ce"}
	export := &fakeSource{name: "export", delivered: map[string]string{"2026-09": `"etag-9"`}}

	t.Run("no export configured", func(t *testing.T) {
		c := Coster{costExplorer: ce}
		src, err := c.MonthSource(context.Background(), day(2026, 9, 1))
		require.NoError(t, err)
		assert.Equal(t, infra_sdk.CostSourceCostExplorer, src.Name)
		assert.Equal(t, "", src.Version)
		assert.Equal(t, CostExplorerCapabilities(), src.Capabilities)
		assert.Equal(t, 2, src.Capabilities.MaxGroupBy)
		assert.False(t, src.Capabilities.HasResource)
	})
	t.Run("export delivered the month", func(t *testing.T) {
		c := Coster{costExplorer: ce, export: export}
		src, err := c.MonthSource(context.Background(), day(2026, 9, 1))
		require.NoError(t, err)
		assert.Equal(t, infra_sdk.CostSourceFocusExport, src.Name)
		assert.Equal(t, `"etag-9"`, src.Version)
		assert.Equal(t, aws_data_export.Capabilities(), src.Capabilities)
		assert.True(t, src.Capabilities.HasResource)
		assert.Equal(t, infra_sdk.AllCostMetrics(), src.Capabilities.Metrics)
	})
	t.Run("export exists but not for this month", func(t *testing.T) {
		c := Coster{costExplorer: ce, export: export}
		src, err := c.MonthSource(context.Background(), day(2026, 8, 1))
		require.NoError(t, err)
		assert.Equal(t, infra_sdk.CostSourceCostExplorer, src.Name)
	})
}

func TestCoster_ReferenceCosterIsCostExplorer(t *testing.T) {
	ce := &fakeSource{name: "ce"}
	export := &fakeSource{name: "export", delivered: map[string]string{"2026-09": "x"}}
	c := Coster{costExplorer: ce, export: export}
	assert.Same(t, ce, c.ReferenceCoster())
}

func TestCoster_GetCosts_Routing(t *testing.T) {
	newFakes := func(delivered ...string) (*fakeSource, *fakeSource) {
		d := map[string]string{}
		for _, m := range delivered {
			d[m] = "etag"
		}
		return &fakeSource{name: "ce"}, &fakeSource{name: "export", delivered: d}
	}
	values := func(res *infra_sdk.CostResult) []string {
		out := []string{}
		for _, s := range res.Series {
			for _, p := range s.Points {
				out = append(out, p.Start.Format("2006-01")+"="+p.Value)
			}
		}
		return out
	}

	t.Run("no export passes the query through to cost explorer", func(t *testing.T) {
		ce, _ := newFakes()
		c := Coster{costExplorer: ce}
		q := infra_sdk.CostQuery{Start: day(2026, 7, 1), End: day(2026, 10, 1), Granularity: infra_sdk.CostGranularityMonthly}
		_, err := c.GetCosts(context.Background(), q)
		require.NoError(t, err)
		require.Len(t, ce.queries, 1)
		assert.Equal(t, q, ce.queries[0])
	})

	t.Run("window fully exported goes to the export untouched", func(t *testing.T) {
		ce, export := newFakes("2026-08", "2026-09")
		c := Coster{costExplorer: ce, export: export}
		q := infra_sdk.CostQuery{Start: day(2026, 8, 3), End: day(2026, 9, 20), Granularity: infra_sdk.CostGranularityDaily}
		_, err := c.GetCosts(context.Background(), q)
		require.NoError(t, err)
		assert.Empty(t, ce.queries)
		require.Len(t, export.queries, 1)
		assert.Equal(t, q, export.queries[0], "the window is not split when one source covers it")
	})

	t.Run("mixed window is split at the month where the export starts and merged", func(t *testing.T) {
		ce, export := newFakes("2026-09", "2026-10")
		c := Coster{costExplorer: ce, export: export}
		q := infra_sdk.CostQuery{Start: day(2026, 7, 15), End: day(2026, 10, 10), Granularity: infra_sdk.CostGranularityMonthly}
		res, err := c.GetCosts(context.Background(), q)
		require.NoError(t, err)

		require.Len(t, ce.queries, 1)
		assert.Equal(t, day(2026, 7, 15), ce.queries[0].Start)
		assert.Equal(t, day(2026, 9, 1), ce.queries[0].End)
		require.Len(t, export.queries, 1)
		assert.Equal(t, day(2026, 9, 1), export.queries[0].Start)
		assert.Equal(t, day(2026, 10, 10), export.queries[0].End)

		assert.ElementsMatch(t, []string{"2026-07=ce", "2026-08=ce", "2026-09=export", "2026-10=export"}, values(res))
	})

	t.Run("a gap in the export falls back per month", func(t *testing.T) {
		ce, export := newFakes("2026-07", "2026-09")
		c := Coster{costExplorer: ce, export: export}
		q := infra_sdk.CostQuery{Start: day(2026, 7, 1), End: day(2026, 10, 1), Granularity: infra_sdk.CostGranularityMonthly}
		res, err := c.GetCosts(context.Background(), q)
		require.NoError(t, err)
		require.Len(t, export.queries, 2)
		require.Len(t, ce.queries, 1)
		assert.Equal(t, day(2026, 8, 1), ce.queries[0].Start)
		assert.Equal(t, day(2026, 9, 1), ce.queries[0].End)
		assert.ElementsMatch(t, []string{"2026-07=export", "2026-08=ce", "2026-09=export"}, values(res))
	})

	t.Run("hourly always goes to cost explorer", func(t *testing.T) {
		ce, export := newFakes("2026-09")
		c := Coster{costExplorer: ce, export: export}
		q := infra_sdk.CostQuery{Start: day(2026, 9, 1), End: day(2026, 9, 2), Granularity: infra_sdk.CostGranularityHourly}
		_, err := c.GetCosts(context.Background(), q)
		require.NoError(t, err)
		assert.Empty(t, export.queries)
		assert.Len(t, ce.queries, 1)
	})
}

func TestCoster_ImplementsMonthCoster(t *testing.T) {
	var _ infra_sdk.MonthCoster = Coster{}
	var _ infra_sdk.ProviderTyped = Coster{}
	var _ infra_sdk.ProviderTyped = CostExplorerCoster{}
}

func TestCoster_ExportCosterBuiltFromLocation(t *testing.T) {
	loc, err := aws_data_export.ParseLocation("s3://ns-billing/exports/nullstone-focus")
	require.NoError(t, err)
	c := Coster{Export: &loc, ExportRegion: "us-west-2"}
	export, ok := c.exportCoster().(aws_data_export.Coster)
	require.True(t, ok)
	assert.Equal(t, loc, export.Location)
	assert.Equal(t, "us-west-2", export.Region)
	assert.Nil(t, Coster{}.exportCoster())
}
