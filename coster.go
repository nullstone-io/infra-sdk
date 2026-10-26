package infra_sdk

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"time"
)

type CostGranularity string

const (
	CostGranularityHourly  CostGranularity = "hourly"
	CostGranularityDaily   CostGranularity = "daily"
	CostGranularityMonthly CostGranularity = "monthly"
)

// CostMetric names a cost measure using the FOCUS (FinOps Open Cost and Usage Specification)
// vocabulary. Every coster reports the subset it can derive; consumers pick the measure they
// want by name instead of reading a provider-specific metric.
//
//	ListCost       -- public list price, before any negotiated or commitment discount
//	ContractedCost -- list price after negotiated (contract) discounts, before commitment discounts
//	EffectiveCost  -- amortized cost: contracted cost after commitment discounts, with upfront
//	                  commitment purchases spread across the usage they cover
//	BilledCost     -- what the invoice charges for the period (cash basis; upfront purchases land
//	                  in full, credits and refunds are applied)
//
// Savings math: ListCost - ContractedCost is the negotiated discount, ContractedCost - EffectiveCost
// is the commitment (reservation / savings plan / CUD) discount.
type CostMetric string

const (
	CostMetricListCost       CostMetric = "ListCost"
	CostMetricContractedCost CostMetric = "ContractedCost"
	CostMetricEffectiveCost  CostMetric = "EffectiveCost"
	CostMetricBilledCost     CostMetric = "BilledCost"
)

// CostMetricDefault is the measure consumers should show when they only want one number.
// Amortized cost is the fairest day-to-day view: commitments are spread across the usage they
// discount instead of spiking on the day they were purchased.
const CostMetricDefault = CostMetricEffectiveCost

func AllCostMetrics() []CostMetric {
	return []CostMetric{CostMetricListCost, CostMetricContractedCost, CostMetricEffectiveCost, CostMetricBilledCost}
}

func (m CostMetric) IsValid() bool {
	return slices.Contains(AllCostMetrics(), m)
}

// CostChargeCategory is the FOCUS ChargeCategory: the kind of charge a row represents.
// It is the value of the UniversalDimensionChargeCategory group key.
type CostChargeCategory string

const (
	CostChargeCategoryUsage      CostChargeCategory = "Usage"
	CostChargeCategoryPurchase   CostChargeCategory = "Purchase"
	CostChargeCategoryTax        CostChargeCategory = "Tax"
	CostChargeCategoryCredit     CostChargeCategory = "Credit"
	CostChargeCategoryAdjustment CostChargeCategory = "Adjustment"
)

type Coster interface {
	GetCosts(ctx context.Context, query CostQuery) (*CostResult, error)
}

// CostCapabilities describes what a cost source can answer. The same provider can expose
// different capabilities per billing month when a richer source (an AWS Data Export) only covers
// some months; consumers shape their queries from these rather than from the provider type.
type CostCapabilities struct {
	// MaxGroupBy is the most group identifiers one query accepts; 0 means no practical limit.
	MaxGroupBy int `json:"maxGroupBy"`
	// Metrics lists the FOCUS measures the source reports. Measures not listed are absent.
	Metrics []CostMetric `json:"metrics"`
	// HasChargeCategory reports whether the source can group by UniversalDimensionChargeCategory.
	// Without it the category has to be inferred by the consumer.
	HasChargeCategory bool `json:"hasChargeCategory"`
	// HasResource reports whether the source can group by UniversalDimensionResource.
	HasResource bool `json:"hasResource"`
}

// Cost source names: where a billing month's numbers are read from.
const (
	// CostSourceCostExplorer is the AWS Cost Explorer API (BilledCost + EffectiveCost only).
	CostSourceCostExplorer = "cost-explorer"
	// CostSourceFocusExport is an AWS Data Export in FOCUS 1.2 format (all four measures, resource grain).
	CostSourceFocusExport = "focus-export"
	// CostSourceBillingExport is the GCP BigQuery billing export.
	CostSourceBillingExport = "billing-export"
)

// CostSource identifies which source a MonthCoster reads a billing month from.
type CostSource struct {
	// Name is one of the CostSource* constants.
	Name string `json:"name"`
	// Version identifies the current refresh of the source for the month (an export's manifest
	// ETag). Empty when the source has no such identity (an API).
	Version      string           `json:"version,omitempty"`
	Capabilities CostCapabilities `json:"capabilities"`
}

// MonthCoster is a Coster that reads billing months from whichever of its sources covers them.
// A provider with one source answers the same for every month; a provider with an optional richer
// source (AWS with a Data Export) answers per month, and GetCosts routes each month accordingly.
type MonthCoster interface {
	Coster
	// MonthSource reports which source answers the billing month and what it can do.
	MonthSource(ctx context.Context, month time.Time) (CostSource, error)
	// ReferenceCoster is the coster whose totals match the provider's own bill. When a month is
	// read from a richer source, its total is reconciled against this one. A provider with a
	// single source returns itself.
	ReferenceCoster() Coster
}

// ProviderTyped is implemented by costers that can name the cloud provider they report for.
// MultiCoster stamps this onto every series it collects so consumers can attribute a series to a
// provider without spending a group-by slot on the cloud account dimension. That matters because
// AWS Cost Explorer allows at most 2 group definitions per query.
type ProviderTyped interface {
	ProviderType() string
}

type CostQuery struct {
	Start       time.Time            `json:"start"`
	End         time.Time            `json:"end"`
	Granularity CostGranularity      `json:"granularity"`
	FilterTags  []CostFilterTag      `json:"filterTags"`
	GroupBy     CostGroupIdentifiers `json:"groupBy"`
}

// CostFilterTag restricts a query to resources whose tag Key has one of Values.
// An empty string in Values matches resources that do not carry the tag at all, which is how a
// caller asks for the untagged ("unmanaged") remainder.
type CostFilterTag struct {
	Key    string   `json:"key"`
	Values []string `json:"values"`
}

// MatchesAbsent reports whether the filter asks for resources without the tag.
func (t CostFilterTag) MatchesAbsent() bool {
	return slices.Contains(t.Values, "")
}

// PresentValues returns Values without the empty-string "absent" marker.
func (t CostFilterTag) PresentValues() []string {
	result := make([]string, 0, len(t.Values))
	for _, v := range t.Values {
		if v != "" {
			result = append(result, v)
		}
	}
	return result
}

type CostGroupIdentifiers []CostGroupIdentifier

func (s CostGroupIdentifiers) Unique() CostGroupIdentifiers {
	result := make(CostGroupIdentifiers, 0)

	visitedTags := map[string]bool{}
	visitedDimensions := map[string]bool{}
	for _, cur := range s {
		if cur.TagKey != "" {
			if _, visited := visitedTags[cur.TagKey]; !visited {
				result = append(result, cur)
				visitedTags[cur.TagKey] = true
			}
		} else if cur.Dimension != "" {
			if _, visited := visitedDimensions[cur.Dimension]; !visited {
				result = append(result, cur)
				visitedDimensions[cur.Dimension] = true
			}
		}
	}

	return result
}

type CostGroupIdentifier struct {
	TagKey    string `json:"tagKey,omitempty"`
	Dimension string `json:"dimension,omitempty"`
}

type CostResult struct {
	Series map[string]CostSeries `json:"series"`
}

// CostSeriesKey is the key a series lives under in CostResult.Series: the encoded group keys
// and the metric name.
func CostSeriesKey(groupKeys CostSeriesGroupKeys, metric CostMetric) string {
	return groupKeys.UniqueIdentifier() + ":" + string(metric)
}

// AddSeries stores a complete series, replacing any series already present for the same group
// keys and metric. Costers that assemble a series' points themselves use it instead of calling
// AddDatapoint once per point, which re-encodes the group keys every time.
func (r *CostResult) AddSeries(series CostSeries) {
	r.Series[CostSeriesKey(series.GroupKeys, series.MetricName)] = series
}

func (r *CostResult) AddDatapoint(metric CostMetric, groupKeys CostSeriesGroupKeys, datapoint CostSeriesDatapoint) {
	seriesKey := CostSeriesKey(groupKeys, metric)
	cur, ok := r.Series[seriesKey]
	if !ok {
		cur = CostSeries{
			MetricName: metric,
			GroupKeys:  groupKeys,
			Points:     []CostSeriesDatapoint{},
		}
	}
	cur.Points = append(cur.Points, datapoint)
	r.Series[seriesKey] = cur
}

// MergeDatapoint acts like AddDatapoint except it will not add a duplicate datapoint
// This is detected by comparing start+end times on the datapoint
func (r *CostResult) MergeDatapoint(metric CostMetric, groupKeys CostSeriesGroupKeys, datapoint CostSeriesDatapoint) {
	seriesKey := CostSeriesKey(groupKeys, metric)
	cur, ok := r.Series[seriesKey]
	if !ok {
		cur = CostSeries{
			MetricName: metric,
			GroupKeys:  groupKeys,
			Points:     []CostSeriesDatapoint{},
		}
	}

	isSameDatapoint := func(cur CostSeriesDatapoint) bool {
		return cur.Start == datapoint.Start && cur.End == datapoint.End
	}
	if slices.ContainsFunc(cur.Points, isSameDatapoint) {
		return
	}
	cur.Points = append(cur.Points, datapoint)
	r.Series[seriesKey] = cur
}

func NewCostResult() *CostResult {
	return &CostResult{
		Series: map[string]CostSeries{},
	}
}

type CostSeries struct {
	MetricName CostMetric            `json:"metricName"`
	GroupKeys  CostSeriesGroupKeys   `json:"groupKeys"`
	Points     []CostSeriesDatapoint `json:"points"`
	// Provider names the cloud provider that reported this series ("aws", "gcp", ...).
	// Populated by MultiCoster; empty when a coster runs standalone.
	Provider string `json:"provider,omitempty"`
}

// MergeSeries folds every datapoint of series into the result, recording which provider it came
// from. Datapoints already present (same start+end) are left alone.
func (r *CostResult) MergeSeries(series CostSeries, provider string) {
	for _, point := range series.Points {
		r.MergeDatapoint(series.MetricName, series.GroupKeys, point)
	}
	if provider == "" {
		return
	}
	seriesKey := CostSeriesKey(series.GroupKeys, series.MetricName)
	if cur, ok := r.Series[seriesKey]; ok && cur.Provider == "" {
		cur.Provider = provider
		r.Series[seriesKey] = cur
	}
}

type CostSeriesGroupKeys []CostSeriesGroupKey

func (s CostSeriesGroupKeys) UniqueIdentifier() string {
	size := 0
	for _, key := range s {
		size += len(key.Name) + len(key.TagKey) + len(key.Value) + 2
	}
	return string(s.AppendIdentifier(make([]byte, 0, size)))
}

// AppendIdentifier writes UniqueIdentifier() onto buf and returns it. Aggregators that look a
// row's group up by identifier use it with a reused buffer so the lookup allocates nothing.
func (s CostSeriesGroupKeys) AppendIdentifier(buf []byte) []byte {
	for i, key := range s {
		if i > 0 {
			// add delimiter before index 1+
			buf = append(buf, ';')
		}
		buf = key.AppendEncoded(buf)
	}
	return buf
}

// CostSeriesGroupKey represents a grouping dimension for a cost series.
// If the group key is a tag, TagKey and TagValue are populated.
// Otherwise, Name is populated.
type CostSeriesGroupKey struct {
	Name   string `json:"name,omitempty"`
	TagKey string `json:"tagKey,omitempty"`
	Value  string `json:"value"`
}

// Encode creates a single string that can be decoded consistently
// We use `>` between name/tag-key and value since it's an invalid character for aws tags, gcp labels, k8s labels, etc.
func (k CostSeriesGroupKey) Encode() string {
	return string(k.AppendEncoded(nil))
}

// AppendEncoded writes Encode() onto buf and returns it.
func (k CostSeriesGroupKey) AppendEncoded(buf []byte) []byte {
	if k.TagKey != "" {
		buf = append(buf, k.TagKey...)
	} else {
		buf = append(buf, k.Name...)
	}
	buf = append(buf, '$')
	return append(buf, k.Value...)
}

// CostSeriesDatapoint represents a single datapoint in a cost series.
// It has a Start and End time to represent the time period covered by the datapoint.
// The Value is the cost for that period.
type CostSeriesDatapoint struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	Unit  string    `json:"unit"`
	Value string    `json:"value"`
}

// CostRow is one aggregated row of a cost query: the measures a source reports for one set of
// group-key values over one period in one currency. It is the row shape of CostScanner; the
// same information in a CostResult is spread over one series per metric.
type CostRow struct {
	GroupKeys CostSeriesGroupKeys
	Start     time.Time
	End       time.Time
	Currency  string
	// Values holds the measures the source reported for this row. A metric the source cannot
	// produce is absent, never 0.
	Values map[CostMetric]float64
}

// CostScanner streams a query's result one row at a time instead of materialising a
// CostResult. A coster offers it when its rows are cheaper to hand over than series: a Data
// Export month at resource grain is tens of thousands of rows, and a consumer landing them in
// a store never needs them all at once.
//
// Rows are not promised to be summed: a source that reads line items hands each one over as it
// is, so the same (group keys, period, currency) may arrive many times and the consumer sums
// them (a database GROUP BY). A source that already aggregates sends each once. Order is
// unspecified, and a row is only valid until fn returns: it may be reused for the next.
// Consumers type-assert for the scanner and fall back to GetCosts with CostResult.EachRow.
type CostScanner interface {
	ScanCosts(ctx context.Context, query CostQuery, fn func(CostRow) error) error
}

// EachRow regroups the result's series into rows (one per group keys, period and currency, with
// every metric reported for it) and calls fn once per row. Rows are emitted grouped by series
// group keys, periods in order within a group. It is how a consumer that wants rows reads a coster
// that only offers GetCosts.
func (r *CostResult) EachRow(fn func(CostRow) error) error {
	if r == nil {
		return nil
	}
	type rowKey struct {
		start    time.Time
		end      time.Time
		currency string
	}
	type groupRows struct {
		keys CostSeriesGroupKeys
		rows map[rowKey]*CostRow
	}
	groups := map[string]*groupRows{}
	order := make([]string, 0)
	for _, series := range r.Series {
		id := series.GroupKeys.UniqueIdentifier()
		g, ok := groups[id]
		if !ok {
			g = &groupRows{keys: series.GroupKeys, rows: map[rowKey]*CostRow{}}
			groups[id] = g
			order = append(order, id)
		}
		for _, point := range series.Points {
			val, err := strconv.ParseFloat(point.Value, 64)
			if err != nil {
				return fmt.Errorf("invalid cost value %q: %w", point.Value, err)
			}
			key := rowKey{start: point.Start, end: point.End, currency: point.Unit}
			row, ok := g.rows[key]
			if !ok {
				row = &CostRow{GroupKeys: series.GroupKeys, Start: point.Start, End: point.End, Currency: point.Unit, Values: map[CostMetric]float64{}}
				g.rows[key] = row
			}
			// the same metric can appear twice for one period when a result was merged; sum
			row.Values[series.MetricName] += val
		}
	}
	sort.Strings(order)
	for _, id := range order {
		g := groups[id]
		rows := make([]*CostRow, 0, len(g.rows))
		for _, row := range g.rows {
			rows = append(rows, row)
		}
		sort.Slice(rows, func(i, j int) bool {
			if !rows[i].Start.Equal(rows[j].Start) {
				return rows[i].Start.Before(rows[j].Start)
			}
			if !rows[i].End.Equal(rows[j].End) {
				return rows[i].End.Before(rows[j].End)
			}
			return rows[i].Currency < rows[j].Currency
		})
		for _, row := range rows {
			if err := fn(*row); err != nil {
				return err
			}
		}
	}
	return nil
}
