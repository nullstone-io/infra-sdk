package infra_sdk

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type CostGranularity string

const (
	CostGranularityHourly  CostGranularity = "hourly"
	CostGranularityDaily   CostGranularity = "daily"
	CostGranularityMonthly CostGranularity = "monthly"
)

type Coster interface {
	GetCosts(ctx context.Context, query CostQuery) (*CostResult, error)
}

type CostQuery struct {
	Start       time.Time            `json:"start"`
	End         time.Time            `json:"end"`
	Granularity CostGranularity      `json:"granularity"`
	FilterTags  []CostFilterTag      `json:"filterTags"`
	GroupBy     CostGroupIdentifiers `json:"groupBy"`
}

type CostFilterTag struct {
	Key    string   `json:"key"`
	Values []string `json:"values"`
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

func (r *CostResult) AddDatapoint(metricName string, groupKeys CostSeriesGroupKeys, datapoint CostSeriesDatapoint) {
	seriesKey := fmt.Sprintf("%s:%s", groupKeys.UniqueIdentifier(), metricName)
	cur, ok := r.Series[seriesKey]
	if !ok {
		cur = CostSeries{
			MetricName: metricName,
			GroupKeys:  groupKeys,
			Points:     []CostSeriesDatapoint{},
		}
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
	MetricName string                `json:"metricName"`
	GroupKeys  CostSeriesGroupKeys   `json:"groupKeys"`
	Points     []CostSeriesDatapoint `json:"points"`
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
	Name   string `json:"name"`
	TagKey string `json:"tagKey"`
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
