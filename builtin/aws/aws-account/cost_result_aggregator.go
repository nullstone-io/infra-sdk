package aws_account

import (
	"fmt"
	"strings"
	"time"

	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	infra_sdk "github.com/nullstone-io/infra-sdk"
	aws_names "github.com/nullstone-io/infra-sdk/builtin/aws/aws-names"
)

func NewCostResultAggregator() *CostResultAggregator {
	return &CostResultAggregator{
		CostResult: infra_sdk.NewCostResult(),
	}
}

type CostResultAggregator struct {
	CostResult *infra_sdk.CostResult
}

func (a *CostResultAggregator) AddResults(resultsByTime []cetypes.ResultByTime, inputGroups infra_sdk.CostGroupIdentifiers) error {
	for _, resultByTime := range resultsByTime {
		start, end, err := a.parseWindow(resultByTime)
		if err != nil {
			return fmt.Errorf("error parsing result: %w", err)
		}

		for _, grp := range resultByTime.Groups {
			grpKeys := a.parseResultGroupKeys(inputGroups, grp.Keys)
			for ceMetricName, metricValue := range grp.Metrics {
				metric, ok := ceMetrics[ceMetricName]
				if !ok {
					// Cost Explorer only returns what we asked for; anything else is not a FOCUS measure we report
					continue
				}
				a.CostResult.AddDatapoint(metric, grpKeys, infra_sdk.CostSeriesDatapoint{
					Start: start,
					End:   end,
					Unit:  unptr(metricValue.Unit),
					Value: unptr(metricValue.Amount),
				})
			}
		}
	}
	return nil
}

func (a *CostResultAggregator) parseWindow(resultByTime cetypes.ResultByTime) (time.Time, time.Time, error) {
	if resultByTime.TimePeriod == nil {
		return time.Time{}, time.Time{}, fmt.Errorf("missing time period in results")
	}
	rawStart, rawEnd := unptr(resultByTime.TimePeriod.Start), unptr(resultByTime.TimePeriod.End)
	start, err := time.Parse("2006-01-02", rawStart)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid start time in results %q: %w", rawStart, err)
	}
	end, err := time.Parse("2006-01-02", rawEnd)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid end time in results %q: %w", rawEnd, err)
	}
	return start, end, nil
}

// parseResultGroupKeys maps a group's keys back onto the requested group definitions.
//
// Cost Explorer returns one key per definition, in request order: a tag key as "<tag>$<value>"
// and a dimension as its bare value. Tags are recognised by their prefix; dimensions are matched
// with the requested dimensions in order. The keys must not be re-sorted for this: a dimension
// value such as "Amazon ECS" sorts before a "Block$api" tag key, and matching by sorted position
// then hands the service value to the tag's slot, where it has no dimension name and is lost.
func (a *CostResultAggregator) parseResultGroupKeys(inputGroups infra_sdk.CostGroupIdentifiers, keys []string) infra_sdk.CostSeriesGroupKeys {
	tagKeys := map[string]bool{}
	dimensions := make([]string, 0, len(inputGroups))
	for _, grp := range inputGroups {
		if grp.TagKey != "" {
			tagKeys[aws_names.UniversalTag(grp.TagKey).ToAws()] = true
		} else if grp.Dimension != "" {
			dimensions = append(dimensions, grp.Dimension)
		}
	}

	result := make(infra_sdk.CostSeriesGroupKeys, 0, len(keys))
	nextDimension := 0
	for _, key := range keys {
		tokens := strings.SplitN(key, "$", 2)
		if len(tokens) == 2 && (tagKeys[tokens[0]] || len(dimensions) == 0) {
			result = append(result, infra_sdk.CostSeriesGroupKey{
				TagKey: aws_names.AwsTag(tokens[0]).ToUniversal(),
				Value:  tokens[1],
			})
			continue
		}

		name := fmt.Sprintf("dimension-%d", nextDimension)
		if nextDimension < len(dimensions) {
			name = dimensions[nextDimension]
		}
		nextDimension++
		value := key
		if name == infra_sdk.UniversalDimensionChargeCategory {
			value = string(aws_names.AwsRecordType(key).ToChargeCategory())
		}
		result = append(result, infra_sdk.CostSeriesGroupKey{
			Name:  name,
			Value: value,
		})
	}
	return result
}
