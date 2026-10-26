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
	aws_names "github.com/nullstone-io/infra-sdk/builtin/aws/aws-names"
)

// SupportedMetrics lists the FOCUS measures a Data Export reports: all four, by specification.
func SupportedMetrics() []infra_sdk.CostMetric {
	return infra_sdk.AllCostMetrics()
}

// Capabilities describes the Data Export tier: one query answers any grouping, every measure is
// reported, and rows carry the FOCUS charge category and billed resource.
func Capabilities() infra_sdk.CostCapabilities {
	return infra_sdk.CostCapabilities{MaxGroupBy: 0, Metrics: SupportedMetrics(), HasChargeCategory: true, HasResource: true}
}

// Coster answers cost queries from an AWS Data Export (FOCUS 1.2, daily, gzip/csv) in S3.
//
// Unlike Cost Explorer it reports ListCost and ContractedCost, carries the FOCUS ChargeCategory,
// and can group by resource. A query spanning months reads each month's manifest and files;
// months without a manifest (before the export existed) contribute nothing; aws_account.Coster
// routes those months to Cost Explorer.
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

// GetCosts sums the export in memory into series. Memory scales with the number of distinct
// (group, period, currency) buckets in the window; a consumer that can sum elsewhere (a database)
// should ScanCosts instead.
func (c Coster) GetCosts(ctx context.Context, query infra_sdk.CostQuery) (*infra_sdk.CostResult, error) {
	agg, err := newAggregator(query)
	if err != nil {
		return nil, err
	}
	ok, err := c.read(ctx, query, agg.add)
	if err != nil || !ok {
		return nil, err
	}
	return agg.result(), nil
}

// ScanCosts streams the export's line items as rows without summing them: one CostRow per
// export row that passes the query's filters, projected onto the query's group keys and period,
// carrying that row's four FOCUS measures. The same (group keys, period, currency) therefore
// arrives once per charge; the consumer sums. Nothing is held beyond the row being read, so
// memory does not grow with the month. The row, its GroupKeys and its Values are reused and only
// valid until fn returns.
func (c Coster) ScanCosts(ctx context.Context, query infra_sdk.CostQuery, fn func(infra_sdk.CostRow) error) error {
	p, err := newAggregator(query)
	if err != nil {
		return err
	}
	_, err = c.read(ctx, query, func(row Row) error { return p.emit(row, fn) })
	return err
}

// read streams every delivered month in the query window through add. False when the accessor
// has no credentials (no client), which answers the query with nothing.
func (c Coster) read(ctx context.Context, query infra_sdk.CostQuery, add func(Row) error) (bool, error) {
	if query.Granularity == infra_sdk.CostGranularityHourly {
		return false, fmt.Errorf("data exports are delivered daily; hourly granularity is not supported")
	}
	api, err := c.client()
	if err != nil {
		return false, err
	}
	if api == nil {
		return false, nil
	}
	for _, month := range MonthsInRange(query.Start, query.End) {
		manifest, err := ReadManifest(ctx, api, c.Location, month)
		if errors.Is(err, ErrNoManifest) {
			continue
		}
		if err != nil {
			return false, err
		}
		if err := ReadRows(ctx, api, c.Location.Bucket, manifest.DataFiles, add); err != nil {
			return false, err
		}
	}
	return true, nil
}

// MonthsInRange lists the first day of every month touched by [start, end).
func MonthsInRange(start, end time.Time) []time.Time {
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

// aggregator folds rows into buckets: one group (the distinct group-key values) holds one bucket
// per period and currency with the four summed measures. Memory scales with the number of
// distinct buckets, never with the number of rows, and a row that lands in an existing group
// allocates nothing: its group identifier is encoded into a scratch buffer and looked up as is.
type aggregator struct {
	query   infra_sdk.CostQuery
	groupBy infra_sdk.CostGroupIdentifiers
	filters []tagFilter
	groups  map[string]*group
	// tagValues dedupes the tag values copied into group keys, so a stack name that appears on
	// every row is held once rather than once per group.
	tagValues interner
	// keyScratch and idScratch are reused per row to build the group identifier without allocating;
	// values is the reused measure map of an emitted row.
	keyScratch infra_sdk.CostSeriesGroupKeys
	idScratch  []byte
	values     map[infra_sdk.CostMetric]float64
}

type tagFilter struct {
	awsKey string
	filter infra_sdk.CostFilterTag
}

// group is one distinct set of group-key values: a series per metric in the result.
type group struct {
	// id is the encoded group keys (CostSeriesGroupKeys.UniqueIdentifier).
	id      string
	keys    infra_sdk.CostSeriesGroupKeys
	buckets map[periodKey]*bucket
}

type periodKey struct {
	start    time.Time
	currency string
}

type bucket struct {
	start    time.Time
	end      time.Time
	currency string
	sums     costSums
}

// costSums holds the four FOCUS measures of a bucket without a per-bucket map.
type costSums struct {
	list, contracted, effective, billed float64
}

func (s *costSums) add(row Row) {
	s.list += row.ListCost
	s.contracted += row.ContractedCost
	s.effective += row.EffectiveCost
	s.billed += row.BilledCost
}

func (s costSums) get(metric infra_sdk.CostMetric) float64 {
	switch metric {
	case infra_sdk.CostMetricListCost:
		return s.list
	case infra_sdk.CostMetricContractedCost:
		return s.contracted
	case infra_sdk.CostMetricEffectiveCost:
		return s.effective
	case infra_sdk.CostMetricBilledCost:
		return s.billed
	}
	return 0
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
		filters = append(filters, tagFilter{awsKey: aws_names.UniversalTag(f.Key).ToAws(), filter: f})
	}
	return &aggregator{
		query:      query,
		groupBy:    groupBy,
		filters:    filters,
		groups:     map[string]*group{},
		tagValues:  newInterner(),
		keyScratch: make(infra_sdk.CostSeriesGroupKeys, 0, len(groupBy)),
		values:     make(map[infra_sdk.CostMetric]float64, 4),
	}, nil
}

// accepts applies the query window and tag filters to a row.
func (a *aggregator) accepts(row Row) bool {
	if row.ChargePeriodStart.Before(a.query.Start) || !row.ChargePeriodStart.Before(a.query.End) {
		return false
	}
	for _, f := range a.filters {
		if !f.matches(row.Tags) {
			return false
		}
	}
	return true
}

// emit hands one accepted row to fn projected onto the query's group keys and period, without
// bucketing it. The CostRow reuses the aggregator's scratch keys and values.
func (a *aggregator) emit(row Row, fn func(infra_sdk.CostRow) error) error {
	if !a.accepts(row) {
		return nil
	}
	start, end := a.period(row.ChargePeriodStart)
	a.values[infra_sdk.CostMetricListCost] = row.ListCost
	a.values[infra_sdk.CostMetricContractedCost] = row.ContractedCost
	a.values[infra_sdk.CostMetricEffectiveCost] = row.EffectiveCost
	a.values[infra_sdk.CostMetricBilledCost] = row.BilledCost
	return fn(infra_sdk.CostRow{GroupKeys: a.groupKeys(row), Start: start, End: end, Currency: row.BillingCurrency, Values: a.values})
}

func (a *aggregator) add(row Row) error {
	if !a.accepts(row) {
		return nil
	}

	keys := a.groupKeys(row)
	a.idScratch = keys.AppendIdentifier(a.idScratch[:0])
	g, ok := a.groups[string(a.idScratch)]
	if !ok {
		g = &group{keys: a.retainKeys(keys), buckets: map[periodKey]*bucket{}}
		g.id = g.keys.UniqueIdentifier()
		a.groups[g.id] = g
	}

	start, end := a.period(row.ChargePeriodStart)
	pk := periodKey{start: start, currency: row.BillingCurrency}
	b, ok := g.buckets[pk]
	if !ok {
		b = &bucket{start: start, end: end, currency: row.BillingCurrency}
		g.buckets[pk] = b
	}
	b.sums.add(row)
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

// groupKeys builds the row's group keys into the reusable scratch slice. The result is only
// valid until the next call; retainKeys copies it for a new group.
func (a *aggregator) groupKeys(row Row) infra_sdk.CostSeriesGroupKeys {
	keys := a.keyScratch[:0]
	for _, g := range a.groupBy {
		if g.TagKey != "" {
			value, _ := TagValue(row.Tags, aws_names.UniversalTag(g.TagKey).ToAws())
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
	a.keyScratch = keys
	return keys
}

// retainKeys copies scratch keys for a group that outlives the row. Dimension values are already
// interned by the reader; tag values come out of the row's Tags map and are interned here.
func (a *aggregator) retainKeys(scratch infra_sdk.CostSeriesGroupKeys) infra_sdk.CostSeriesGroupKeys {
	keys := make(infra_sdk.CostSeriesGroupKeys, len(scratch))
	for i, key := range scratch {
		if key.TagKey != "" {
			key.Value = a.tagValues.intern(key.Value)
		}
		keys[i] = key
	}
	return keys
}

// ordered returns the group's buckets in period order (then currency).
func (g *group) ordered() []*bucket {
	ordered := make([]*bucket, 0, len(g.buckets))
	for _, b := range g.buckets {
		ordered = append(ordered, b)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if !ordered[i].start.Equal(ordered[j].start) {
			return ordered[i].start.Before(ordered[j].start)
		}
		return ordered[i].currency < ordered[j].currency
	})
	return ordered
}

// result shapes the groups as a CostResult: one series per (group, metric) with its points in
// period order. Series are assembled whole (AddSeries), so a group identifier is encoded once per
// series rather than once per datapoint.
func (a *aggregator) result() *infra_sdk.CostResult {
	metrics := infra_sdk.AllCostMetrics()
	result := infra_sdk.NewCostResult()
	for _, g := range a.groups {
		ordered := g.ordered()
		for _, metric := range metrics {
			points := make([]infra_sdk.CostSeriesDatapoint, 0, len(ordered))
			for _, b := range ordered {
				points = append(points, infra_sdk.CostSeriesDatapoint{
					Start: b.start,
					End:   b.end,
					Unit:  b.currency,
					Value: formatCost(b.sums.get(metric)),
				})
			}
			result.AddSeries(infra_sdk.CostSeries{MetricName: metric, GroupKeys: g.keys, Points: points})
		}
	}
	return result
}

// roundCost trims float noise from a summed cost (FOCUS costs carry at most ten decimals), so
// "0.1 + 0.2" reads 0.3.
func roundCost(v float64) float64 {
	return math.Round(v*1e10) / 1e10
}

func formatCost(v float64) string {
	return strconv.FormatFloat(roundCost(v), 'f', -1, 64)
}
