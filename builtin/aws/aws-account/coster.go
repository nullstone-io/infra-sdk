package aws_account

import (
	"context"
	"fmt"

	ce "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	infra_sdk "github.com/nullstone-io/infra-sdk"
	"github.com/nullstone-io/infra-sdk/access/aws"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

var (
	granularityMappings = map[infra_sdk.CostGranularity]cetypes.Granularity{
		infra_sdk.CostGranularityHourly:  cetypes.GranularityHourly,
		infra_sdk.CostGranularityDaily:   cetypes.GranularityDaily,
		infra_sdk.CostGranularityMonthly: cetypes.GranularityMonthly,
	}
)

type Coster struct {
	Assumer  aws.Assumer
	Provider types.Provider
}

func (c Coster) GetCosts(ctx context.Context, query infra_sdk.CostQuery) (*infra_sdk.CostResult, error) {
	// Cost Explorer is global, use us-east-1 as the region to satisfy the aws sdk
	awsConfig, err := aws.ResolveConfig(c.Assumer.AwsConfig(), c.Provider, &types.AwsProviderConfig{Region: "us-east-1"}, "")
	if err != nil {
		return nil, fmt.Errorf("error resolving aws config: %w", err)
	}
	client := ce.NewFromConfig(awsConfig)

	period := &cetypes.DateInterval{
		Start: ptr(query.Start.Format("2006-01-02")),
		End:   ptr(query.End.Format("2006-01-02")), // end is EXCLUSIVE
	}

	granularity := granularityMappings[query.Granularity]
	if granularity == "" {
		granularity = cetypes.GranularityDaily
	}

	groupBy := query.GroupBy.Unique()
	input := &ce.GetCostAndUsageInput{
		TimePeriod:  period,
		Granularity: granularity,
		Metrics:     []string{"UnblendedCost"},
		Filter:      costQueryToFilter(query),
		GroupBy:     costQueryToGroupBy(groupBy),
	}

	aggregator := NewCostResultAggregator()
	var nextToken *string
	for {
		input.NextPageToken = nextToken
		out, err := client.GetCostAndUsage(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("error querying aws cost explorer: %w", err)
		}
		if err := aggregator.AddResults(out.ResultsByTime, groupBy); err != nil {
			return nil, fmt.Errorf("error aggregating results: %w", err)
		}
		if out.NextPageToken == nil || *out.NextPageToken == "" {
			break
		}
		nextToken = out.NextPageToken
	}

	return aggregator.CostResult, nil
}

func costQueryToFilter(query infra_sdk.CostQuery) *cetypes.Expression {
	if len(query.FilterTags) < 1 {
		return nil
	}
	if len(query.FilterTags) == 1 {
		return &cetypes.Expression{
			Tags: &cetypes.TagValues{
				Key:          ptr(UniversalTag(query.FilterTags[0].Key).ToAws()),
				MatchOptions: []cetypes.MatchOption{cetypes.MatchOptionEquals},
				Values:       query.FilterTags[0].Values,
			},
		}
	}

	root := &cetypes.Expression{}
	for _, filterTag := range query.FilterTags {
		root.And = append(root.And, cetypes.Expression{
			Tags: &cetypes.TagValues{
				Key:          ptr(UniversalTag(filterTag.Key).ToAws()),
				MatchOptions: []cetypes.MatchOption{cetypes.MatchOptionEquals},
				Values:       filterTag.Values,
			},
		})
	}
	return root
}

func costQueryToGroupBy(groupBy infra_sdk.CostGroupIdentifiers) []cetypes.GroupDefinition {
	var defs []cetypes.GroupDefinition
	for _, cur := range groupBy {
		if cur.Dimension != "" {
			defs = append(defs, cetypes.GroupDefinition{
				Key:  ptr(UniversalDimension(cur.Dimension).ToAws()),
				Type: cetypes.GroupDefinitionTypeDimension,
			})
		} else if cur.TagKey != "" {
			defs = append(defs, cetypes.GroupDefinition{
				Key:  ptr(UniversalTag(cur.TagKey).ToAws()),
				Type: cetypes.GroupDefinitionTypeTag,
			})
		}

	}
	return defs
}
