package aws_data_export

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	infra_sdk "github.com/nullstone-io/infra-sdk"
	aws_account "github.com/nullstone-io/infra-sdk/builtin/aws/aws-account"
)

// SupportedMetrics lists the FOCUS measures a Data Export reports: all four, by specification.
func SupportedMetrics() []infra_sdk.CostMetric {
	return infra_sdk.AllCostMetrics()
}

// Coster answers cost queries from an AWS Data Export (FOCUS 1.2, daily, gzip/csv) in S3.
//
// Unlike Cost Explorer it reports ListCost and ContractedCost, carries the FOCUS ChargeCategory,
// and can group by resource. A query spanning months reads each month's manifest and files;
// months without a manifest (before the export existed) contribute nothing, so callers that
// need those months must fall back to another source.
type Coster struct {
	Accessor infra_sdk.AwsAccessor
	Location Location
	// Region is the bucket's region. Empty means us-east-1.
	Region string
	// Client, when set, is used instead of one built from Accessor (tests, or callers that
	// already hold a client).
	Client S3API
}

func (c Coster) ProviderType() string { return "aws" }

func (c Coster) client() (S3API, error) {
	if c.Client != nil {
		return c.Client, nil
	}
	region := c.Region
	if region == "" {
		region = "us-east-1"
	}
	cfg, err := c.Accessor.NewConfig(region)
	if err != nil {
		return nil, fmt.Errorf("error resolving aws config: %w", err)
	}
	if cfg == nil {
		return nil, nil
	}
	return s3.NewFromConfig(*cfg), nil
}

// HasMonth reports whether the export has delivered a billing month, and the manifest ETag
// that identifies the current refresh of it.
func (c Coster) HasMonth(ctx context.Context, month time.Time) (string, bool, error) {
	api, err := c.client()
	if err != nil || api == nil {
		return "", false, err
	}
	return HasMonth(ctx, api, c.Location, month)
}

func (c Coster) GetCosts(ctx context.Context, query infra_sdk.CostQuery) (*infra_sdk.CostResult, error) {
	if query.Granularity == infra_sdk.CostGranularityHourly {
		return nil, fmt.Errorf("data exports are delivered daily; hourly granularity is not supported")
	}
	api, err := c.client()
	if err != nil {
		return nil, err
	}
	if api == nil {
		return nil, nil
	}
	agg, err := newAggregator(query)
	if err != nil {
		return nil, err
	}
	for _, month := range monthsInRange(query.Start, query.End) {
		manifest, err := ReadManifest(ctx, api, c.Location, month)
		if errors.Is(err, ErrNoManifest) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if err := ReadRows(ctx, api, c.Location.Bucket, manifest.DataFiles, agg.add); err != nil {
			return nil, err
		}
	}
	return agg.result(), nil
}

// monthsInRange lists the first day of every month touched by [start, end).
func monthsInRange(start, end time.Time) []time.Time {
	months := make([]time.Time, 0)
	if !start.Before(end) {
		return months
	}
	first := time.Date(start.UTC().Year(), start.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	last := end.UTC().Add(-time.Nanosecond)
	lastMonth := time.Date(last.Year(), last.Month(), 1, 0, 0, 0, 0, time.UTC)
	for cur := first; !cur.After(lastMonth); cur = cur.AddDate(0, 1, 0) {
		months = append(months, cur)
	}
	return months
}

// aggregator folds rows into series buckets keyed by group values, period and currency.
// Memory scales with the number of distinct buckets, never with the number of rows.
type aggregator struct {
	query   infra_sdk.CostQuery
	groupBy infra_sdk.CostGroupIdentifiers
	filters []tagFilter
	buckets map[bucketKey]*bucket
}

type tagFilter struct {
	awsKey string
	filter infra_sdk.CostFilterTag
}

type bucketKey struct {
	groups   string
	start    time.Time
	currency string
}

type bucket struct {
	keys     infra_sdk.CostSeriesGroupKeys
	start    time.Time
	end      time.Time
	currency string
	sums     map[infra_sdk.CostMetric]float64
}

func newAggregator(query infra_sdk.CostQuery) (*aggregator, error) {
	groupBy := query.GroupBy.Unique()
	for _, g := range groupBy {
		if g.Dimension == "" {
			continue
		}
		switch g.Dimension {
		case infra_sdk.UniversalDimensionAccount, infra_sdk.UniversalDimensionService,
			infra_sdk.UniversalDimensionChargeCategory, infra_sdk.UniversalDimensionResource:
		default:
			return nil, fmt.Errorf("data exports cannot group by dimension %q", g.Dimension)
		}
	}
	filters := make([]tagFilter, 0, len(query.FilterTags))
	for _, f := range query.FilterTags {
		filters = append(filters, tagFilter{awsKey: aws_account.UniversalTag(f.Key).ToAws(), filter: f})
	}
	return &aggregator{query: query, groupBy: groupBy, filters: filters, buckets: map[bucketKey]*bucket{}}, nil
}

func (a *aggregator) add(row Row) error {
	if row.ChargePeriodStart.Before(a.query.Start) || !row.ChargePeriodStart.Before(a.query.End) {
		return nil
	}
	for _, f := range a.filters {
		if !f.matches(row.Tags) {
			return nil
		}
	}

	start, end := a.period(row.ChargePeriodStart)
	keys := a.groupKeys(row)
	key := bucketKey{groups: keys.UniqueIdentifier(), start: start, currency: row.BillingCurrency}
	b, ok := a.buckets[key]
	if !ok {
		b = &bucket{keys: keys, start: start, end: end, currency: row.BillingCurrency, sums: map[infra_sdk.CostMetric]float64{}}
		a.buckets[key] = b
	}
	b.sums[infra_sdk.CostMetricListCost] += row.ListCost
	b.sums[infra_sdk.CostMetricContractedCost] += row.ContractedCost
	b.sums[infra_sdk.CostMetricEffectiveCost] += row.EffectiveCost
	b.sums[infra_sdk.CostMetricBilledCost] += row.BilledCost
	return nil
}

// matches applies one tag filter: a present value must be listed; a missing tag matches only
// when the filter asks for absent (see infra_sdk.CostFilterTag).
func (f tagFilter) matches(tags map[string]string) bool {
	value, ok := TagValue(tags, f.awsKey)
	if !ok || value == "" {
		return f.filter.MatchesAbsent()
	}
	for _, want := range f.filter.PresentValues() {
		if want == value {
			return true
		}
	}
	return false
}

func (a *aggregator) period(t time.Time) (time.Time, time.Time) {
	if a.query.Granularity == infra_sdk.CostGranularityMonthly {
		start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 1, 0)
	}
	start := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 0, 1)
}

func (a *aggregator) groupKeys(row Row) infra_sdk.CostSeriesGroupKeys {
	keys := make(infra_sdk.CostSeriesGroupKeys, 0, len(a.groupBy))
	for _, g := range a.groupBy {
		if g.TagKey != "" {
			value, _ := TagValue(row.Tags, aws_account.UniversalTag(g.TagKey).ToAws())
			keys = append(keys, infra_sdk.CostSeriesGroupKey{TagKey: g.TagKey, Value: value})
			continue
		}
		var value string
		switch g.Dimension {
		case infra_sdk.UniversalDimensionAccount:
			value = row.SubAccountId
		case infra_sdk.UniversalDimensionService:
			value = row.ServiceName
		case infra_sdk.UniversalDimensionChargeCategory:
			value = row.ChargeCategory
		case infra_sdk.UniversalDimensionResource:
			value = row.ResourceId
		}
		keys = append(keys, infra_sdk.CostSeriesGroupKey{Name: g.Dimension, Value: value})
	}
	return keys
}

func (a *aggregator) result() *infra_sdk.CostResult {
	ordered := make([]*bucket, 0, len(a.buckets))
	for _, b := range a.buckets {
		ordered = append(ordered, b)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if !ordered[i].start.Equal(ordered[j].start) {
			return ordered[i].start.Before(ordered[j].start)
		}
		if ordered[i].currency != ordered[j].currency {
			return ordered[i].currency < ordered[j].currency
		}
		return ordered[i].keys.UniqueIdentifier() < ordered[j].keys.UniqueIdentifier()
	})

	result := infra_sdk.NewCostResult()
	for _, b := range ordered {
		for _, metric := range infra_sdk.AllCostMetrics() {
			result.AddDatapoint(metric, b.keys, infra_sdk.CostSeriesDatapoint{
				Start: b.start,
				End:   b.end,
				Unit:  b.currency,
				Value: formatCost(b.sums[metric]),
			})
		}
	}
	return result
}

// formatCost renders a summed cost with float noise trimmed (FOCUS costs carry at most ten
// decimals), so "0.1 + 0.2" reads 0.3.
func formatCost(v float64) string {
	return strconv.FormatFloat(math.Round(v*1e10)/1e10, 'f', -1, 64)
}
