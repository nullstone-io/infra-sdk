package aws_account

import (
	"fmt"
	"sort"
	"strings"
	"time"

	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	infra_sdk "github.com/nullstone-io/infra-sdk"
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
			for metricName, metricValue := range grp.Metrics {
				a.CostResult.AddDatapoint(metricName, grpKeys, infra_sdk.CostSeriesDatapoint{
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

func (a *CostResultAggregator) parseResultGroupKeys(inputGroups infra_sdk.CostGroupIdentifiers, keys []string) infra_sdk.CostSeriesGroupKeys {
	sort.Strings(keys)

	result := make(infra_sdk.CostSeriesGroupKeys, 0)
	for i, key := range keys {
		tokens := strings.SplitN(key, "$", 2)
		if len(tokens) == 2 {
			result = append(result, infra_sdk.CostSeriesGroupKey{
				TagKey: AwsTag(tokens[0]).ToUniversal(),
				Value:  tokens[1],
			})
		} else {
			name := fmt.Sprintf("dimension-%d", i)
			if i < len(inputGroups) {
				name = inputGroups[i].Dimension
			}
			result = append(result, infra_sdk.CostSeriesGroupKey{
				Name:  name,
				Value: key,
			})
		}
	}
	return result
}
