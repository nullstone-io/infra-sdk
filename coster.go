package infra_sdk

import (
	"context"
	"fmt"
	"slices"
	"strings"
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

func (r *CostResult) AddDatapoint(metric CostMetric, groupKeys CostSeriesGroupKeys, datapoint CostSeriesDatapoint) {
	seriesKey := fmt.Sprintf("%s:%s", groupKeys.UniqueIdentifier(), metric)
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
	seriesKey := fmt.Sprintf("%s:%s", groupKeys.UniqueIdentifier(), metric)
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
	seriesKey := fmt.Sprintf("%s:%s", series.GroupKeys.UniqueIdentifier(), series.MetricName)
	if cur, ok := r.Series[seriesKey]; ok && cur.Provider == "" {
		cur.Provider = provider
		r.Series[seriesKey] = cur
	}
}

type CostSeriesGroupKeys []CostSeriesGroupKey

func (s CostSeriesGroupKeys) UniqueIdentifier() string {
	sb := strings.Builder{}
	for i, key := range s {
		if i > 0 {
			// add delimiter before index 1+
			sb.WriteString(";")
		}
		sb.WriteString(key.Encode())
	}
	return sb.String()
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
	if k.TagKey != "" {
		return fmt.Sprintf("%s$%s", k.TagKey, k.Value)
	}
	return fmt.Sprintf("%s$%s", k.Name, k.Value)
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
