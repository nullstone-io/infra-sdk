package aws_data_export

import (
	"context"
	"errors"
	"testing"
	"time"

	infra_sdk "github.com/nullstone-io/infra-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	groupByAccount        = infra_sdk.CostGroupIdentifier{Dimension: infra_sdk.UniversalDimensionAccount}
	groupByStack          = infra_sdk.CostGroupIdentifier{TagKey: infra_sdk.UniversalTagStack}
	groupByEnv            = infra_sdk.CostGroupIdentifier{TagKey: infra_sdk.UniversalTagEnv}
	groupByBlock          = infra_sdk.CostGroupIdentifier{TagKey: infra_sdk.UniversalTagBlock}
	groupByService        = infra_sdk.CostGroupIdentifier{Dimension: infra_sdk.UniversalDimensionService}
	groupByChargeCategory = infra_sdk.CostGroupIdentifier{Dimension: infra_sdk.UniversalDimensionChargeCategory}
	groupByResource       = infra_sdk.CostGroupIdentifier{Dimension: infra_sdk.UniversalDimensionResource}
)

func testCoster(t *testing.T) Coster {
	return Coster{Location: testLocation, Client: fakeExport(t)}
}

// findSeries returns the series for a metric whose group keys carry every given value.
func findSeries(t *testing.T, res *infra_sdk.CostResult, metric infra_sdk.CostMetric, values ...string) infra_sdk.CostSeries {
	t.Helper()
	for _, series := range res.Series {
		if series.MetricName != metric {
			continue
		}
		matched := 0
		for _, want := range values {
			for _, key := range series.GroupKeys {
				if key.Value == want {
					matched++
					break
				}
			}
		}
		if matched == len(values) {
			return series
		}
	}
	require.Failf(t, "series not found", "metric=%s values=%v", metric, values)
	return infra_sdk.CostSeries{}
}

func TestCoster_GetCosts(t *testing.T) {
	ctx := context.Background()

	t.Run("full attribution grain, daily", func(t *testing.T) {
		res, err := testCoster(t).GetCosts(ctx, infra_sdk.CostQuery{
			Start: aug2026, End: sep2026, Granularity: infra_sdk.CostGranularityDaily,
			GroupBy: infra_sdk.CostGroupIdentifiers{groupByAccount, groupByStack, groupByEnv, groupByBlock, groupByService, groupByChargeCategory, groupByResource},
		})
		require.NoError(t, err)

		// 9 fixture rows, two of which (cr-0001 on Aug 3) share every group value -> 8 buckets x 4 metrics
		assert.Len(t, res.Series, 32)

		ec2 := findSeries(t, res, infra_sdk.CostMetricEffectiveCost, "i-0001")
		require.Len(t, ec2.Points, 1)
		assert.Equal(t, "8", ec2.Points[0].Value)
		assert.Equal(t, "USD", ec2.Points[0].Unit)
		assert.Equal(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), ec2.Points[0].Start)
		assert.Equal(t, time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC), ec2.Points[0].End)
		assert.Equal(t, infra_sdk.CostSeriesGroupKeys{
			{Name: infra_sdk.UniversalDimensionAccount, Value: "111111111111"},
			{TagKey: infra_sdk.UniversalTagStack, Value: "core"},
			{TagKey: infra_sdk.UniversalTagEnv, Value: "prod"},
			{TagKey: infra_sdk.UniversalTagBlock, Value: "api"},
			{Name: infra_sdk.UniversalDimensionService, Value: "Amazon Elastic Compute Cloud"},
			{Name: infra_sdk.UniversalDimensionChargeCategory, Value: "Usage"},
			{Name: infra_sdk.UniversalDimensionResource, Value: "i-0001"},
		}, ec2.GroupKeys)

		// list price is reported (Cost Explorer cannot do this)
		assert.Equal(t, "10", findSeries(t, res, infra_sdk.CostMetricListCost, "i-0001").Points[0].Value)
		assert.Equal(t, "9", findSeries(t, res, infra_sdk.CostMetricContractedCost, "i-0001").Points[0].Value)

		// charge category comes straight from the row
		tax := findSeries(t, res, infra_sdk.CostMetricBilledCost, "Tax")
		assert.Equal(t, "Tax", tax.GroupKeys[5].Value)

		// split rows for one resource sum
		cr := findSeries(t, res, infra_sdk.CostMetricBilledCost, "cr-0001")
		assert.Equal(t, "5", cr.Points[0].Value)

		// untagged rows have empty tag values, never a missing key
		s3 := findSeries(t, res, infra_sdk.CostMetricBilledCost, "arn:aws:s3:::assets")
		assert.Equal(t, infra_sdk.CostSeriesGroupKey{TagKey: infra_sdk.UniversalTagStack, Value: ""}, s3.GroupKeys[1])
	})

	t.Run("monthly totals by account, one point per currency", func(t *testing.T) {
		res, err := testCoster(t).GetCosts(ctx, infra_sdk.CostQuery{
			Start: aug2026, End: sep2026, Granularity: infra_sdk.CostGranularityMonthly,
			GroupBy: infra_sdk.CostGroupIdentifiers{groupByAccount},
		})
		require.NoError(t, err)
		assert.Len(t, res.Series, 12) // 3 accounts x 4 metrics

		payer := findSeries(t, res, infra_sdk.CostMetricBilledCost, "111111111111")
		require.Len(t, payer.Points, 1)
		assert.Equal(t, "120", payer.Points[0].Value)
		assert.Equal(t, aug2026, payer.Points[0].Start)
		assert.Equal(t, sep2026, payer.Points[0].End)
		assert.Equal(t, "20", findSeries(t, res, infra_sdk.CostMetricEffectiveCost, "111111111111").Points[0].Value)

		eur := findSeries(t, res, infra_sdk.CostMetricBilledCost, "333333333333")
		assert.Equal(t, "EUR", eur.Points[0].Unit)
		assert.Equal(t, "7", eur.Points[0].Value)
	})

	t.Run("bare and user-prefixed tag keys both resolve", func(t *testing.T) {
		res, err := testCoster(t).GetCosts(ctx, infra_sdk.CostQuery{
			Start: aug2026, End: sep2026, Granularity: infra_sdk.CostGranularityMonthly,
			GroupBy: infra_sdk.CostGroupIdentifiers{groupByStack},
		})
		require.NoError(t, err)
		// "core" rows: i-0001, i-0002 (user:Stack), Support (Stack), RI purchase (user:Stack)
		assert.Equal(t, "115", findSeries(t, res, infra_sdk.CostMetricBilledCost, "core").Points[0].Value)
	})

	t.Run("absent tag filter selects untagged rows", func(t *testing.T) {
		res, err := testCoster(t).GetCosts(ctx, infra_sdk.CostQuery{
			Start: aug2026, End: sep2026, Granularity: infra_sdk.CostGranularityMonthly,
			FilterTags: []infra_sdk.CostFilterTag{{Key: infra_sdk.UniversalTagStack, Values: []string{""}}},
			GroupBy:    infra_sdk.CostGroupIdentifiers{groupByService},
		})
		require.NoError(t, err)
		assert.Equal(t, "2", findSeries(t, res, infra_sdk.CostMetricBilledCost, "Amazon Simple Storage Service").Points[0].Value)
		assert.Equal(t, "1", findSeries(t, res, infra_sdk.CostMetricBilledCost, "Tax").Points[0].Value)
		assert.Equal(t, "5", findSeries(t, res, infra_sdk.CostMetricBilledCost, "Amazon Elastic Compute Cloud").Points[0].Value)
		assert.Equal(t, "7", findSeries(t, res, infra_sdk.CostMetricBilledCost, "Amazon CloudFront").Points[0].Value)
		assert.Len(t, res.Series, 16)
	})

	t.Run("present tag filters are AND'd", func(t *testing.T) {
		res, err := testCoster(t).GetCosts(ctx, infra_sdk.CostQuery{
			Start: aug2026, End: sep2026, Granularity: infra_sdk.CostGranularityMonthly,
			FilterTags: []infra_sdk.CostFilterTag{
				{Key: infra_sdk.UniversalTagStack, Values: []string{"core"}},
				{Key: infra_sdk.UniversalTagEnv, Values: []string{"dev"}},
			},
			GroupBy: infra_sdk.CostGroupIdentifiers{groupByService},
		})
		require.NoError(t, err)
		assert.Len(t, res.Series, 4)
		assert.Equal(t, "3", findSeries(t, res, infra_sdk.CostMetricBilledCost, "AWS Support (Business)").Points[0].Value)
	})

	t.Run("present and absent values OR within one filter", func(t *testing.T) {
		res, err := testCoster(t).GetCosts(ctx, infra_sdk.CostQuery{
			Start: aug2026, End: sep2026, Granularity: infra_sdk.CostGranularityMonthly,
			FilterTags: []infra_sdk.CostFilterTag{{Key: infra_sdk.UniversalTagEnv, Values: []string{"dev", ""}}},
			GroupBy:    infra_sdk.CostGroupIdentifiers{groupByAccount},
		})
		require.NoError(t, err)
		// dev: Support 3; absent env: S3 2 + Tax 1 + cr-0001 5 (+ CloudFront 7 EUR)
		assert.Equal(t, "8", findSeries(t, res, infra_sdk.CostMetricBilledCost, "111111111111").Points[0].Value)
		assert.Equal(t, "3", findSeries(t, res, infra_sdk.CostMetricBilledCost, "222222222222").Points[0].Value)
	})

	t.Run("query window bounds the rows", func(t *testing.T) {
		res, err := testCoster(t).GetCosts(ctx, infra_sdk.CostQuery{
			Start: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC),
			Granularity: infra_sdk.CostGranularityDaily,
			GroupBy:     infra_sdk.CostGroupIdentifiers{groupByAccount},
		})
		require.NoError(t, err)
		assert.Equal(t, "3", findSeries(t, res, infra_sdk.CostMetricBilledCost, "111111111111").Points[0].Value) // S3 2 + Tax 1
	})

	t.Run("months without a manifest are skipped", func(t *testing.T) {
		jul := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
		res, err := testCoster(t).GetCosts(ctx, infra_sdk.CostQuery{
			Start: jul, End: sep2026, Granularity: infra_sdk.CostGranularityMonthly,
			GroupBy: infra_sdk.CostGroupIdentifiers{groupByAccount},
		})
		require.NoError(t, err)
		payer := findSeries(t, res, infra_sdk.CostMetricBilledCost, "111111111111")
		require.Len(t, payer.Points, 1)
		assert.Equal(t, aug2026, payer.Points[0].Start)

		res, err = testCoster(t).GetCosts(ctx, infra_sdk.CostQuery{Start: jul, End: aug2026, Granularity: infra_sdk.CostGranularityMonthly})
		require.NoError(t, err)
		assert.Empty(t, res.Series)
	})

	t.Run("no group-by yields one total series per metric", func(t *testing.T) {
		res, err := testCoster(t).GetCosts(ctx, infra_sdk.CostQuery{Start: aug2026, End: sep2026, Granularity: infra_sdk.CostGranularityMonthly})
		require.NoError(t, err)
		assert.Len(t, res.Series, 4) // USD and EUR are two points on the same (empty-key) series
		total := findSeries(t, res, infra_sdk.CostMetricBilledCost)
		require.Len(t, total.Points, 2)
		assert.Equal(t, "EUR", total.Points[0].Unit)
		assert.Equal(t, "7", total.Points[0].Value)
		assert.Equal(t, "USD", total.Points[1].Unit)
		assert.Equal(t, "123", total.Points[1].Value)
	})

	t.Run("hourly granularity is rejected", func(t *testing.T) {
		_, err := testCoster(t).GetCosts(ctx, infra_sdk.CostQuery{Start: aug2026, End: sep2026, Granularity: infra_sdk.CostGranularityHourly})
		require.Error(t, err)
	})

	t.Run("unknown dimension is rejected", func(t *testing.T) {
		_, err := testCoster(t).GetCosts(ctx, infra_sdk.CostQuery{
			Start: aug2026, End: sep2026, Granularity: infra_sdk.CostGranularityDaily,
			GroupBy: infra_sdk.CostGroupIdentifiers{{Dimension: "REGION"}},
		})
		require.Error(t, err)
	})

	t.Run("parquet export is rejected", func(t *testing.T) {
		f := newFakeS3()
		f.put(testLocation.ManifestKey(aug2026), []byte(`{"dataFiles":["exports/nullstone-focus/data/BILLING_PERIOD=2026-08/nullstone-focus-00001.snappy.parquet"]}`))
		_, err := Coster{Location: testLocation, Client: f}.GetCosts(ctx, infra_sdk.CostQuery{Start: aug2026, End: sep2026, Granularity: infra_sdk.CostGranularityDaily})
		require.ErrorIs(t, err, ErrUnsupportedFormat)
	})
}

func TestCoster_HasMonth(t *testing.T) {
	ctx := context.Background()
	coster := testCoster(t)

	etag, ok, err := coster.HasMonth(ctx, aug2026)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.NotEmpty(t, etag)

	_, ok, err = coster.HasMonth(ctx, sep2026)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestMonthsInRange(t *testing.T) {
	jul := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, []time.Time{jul, aug2026}, MonthsInRange(time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC), sep2026))
	assert.Equal(t, []time.Time{aug2026}, MonthsInRange(aug2026, time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)))
	assert.Empty(t, MonthsInRange(aug2026, aug2026))
}

func TestFormatCost(t *testing.T) {
	assert.Equal(t, "0.3", formatCost(0.1+0.2))
	assert.Equal(t, "-12.5", formatCost(-12.5))
	assert.Equal(t, "0", formatCost(0))
}

func TestErrNoManifestIsSentinel(t *testing.T) {
	_, err := ReadManifest(context.Background(), newFakeS3(), testLocation, aug2026)
	assert.True(t, errors.Is(err, ErrNoManifest))
}
