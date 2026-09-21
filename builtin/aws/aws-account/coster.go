package aws_account

import (
	"context"
	"fmt"

	ce "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	infra_sdk "github.com/nullstone-io/infra-sdk"
)

var (
	granularityMappings = map[infra_sdk.CostGranularity]cetypes.Granularity{
		infra_sdk.CostGranularityHourly:  cetypes.GranularityHourly,
		infra_sdk.CostGranularityDaily:   cetypes.GranularityDaily,
		infra_sdk.CostGranularityMonthly: cetypes.GranularityMonthly,
	}

	// ceMetrics maps the Cost Explorer metrics we request onto the FOCUS measures they represent.
	//
	// Cost Explorer has no list price, so ListCost and ContractedCost cannot be produced from
	// this coster; callers that need the discount view have to ingest a CUR/Data Export instead.
	//   NetUnblendedCost -- invoice-basis cost after credits/refunds/discounts    -> BilledCost
	//   NetAmortizedCost -- upfront RI/SP fees spread over the usage they cover  -> EffectiveCost
	ceMetrics = map[string]infra_sdk.CostMetric{
		"NetUnblendedCost": infra_sdk.CostMetricBilledCost,
		"NetAmortizedCost": infra_sdk.CostMetricEffectiveCost,
	}
)

// SupportedMetrics lists the FOCUS measures Cost Explorer can report.
func SupportedMetrics() []infra_sdk.CostMetric {
	return []infra_sdk.CostMetric{infra_sdk.CostMetricBilledCost, infra_sdk.CostMetricEffectiveCost}
}

type Coster struct {
	Accessor infra_sdk.AwsAccessor
}

func (c Coster) ProviderType() string { return "aws" }

func (c Coster) GetCosts(ctx context.Context, query infra_sdk.CostQuery) (*infra_sdk.CostResult, error) {
	// Cost Explorer is global, use us-east-1 as the region to satisfy the aws sdk
	awsConfig, err := c.Accessor.NewConfig("us-east-1")
	if err != nil {
		return nil, fmt.Errorf("error resolving aws config: %w", err)
	}
	if awsConfig == nil {
		return nil, nil
	}
	client := ce.NewFromConfig(*awsConfig)

	period := &cetypes.DateInterval{
		Start: ptr(query.Start.Format("2006-01-02")),
		End:   ptr(query.End.Format("2006-01-02")), // end is EXCLUSIVE
	}

	granularity := granularityMappings[query.Granularity]
	if granularity == "" {
		granularity = cetypes.GranularityDaily
	}

	metrics := make([]string, 0, len(ceMetrics))
	for name := range ceMetrics {
		metrics = append(metrics, name)
	}

	groupBy := query.GroupBy.Unique()
	input := &ce.GetCostAndUsageInput{
		TimePeriod:  period,
		Granularity: granularity,
		Metrics:     metrics,
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
		return filterTagToExpression(query.FilterTags[0])
	}

	root := &cetypes.Expression{}
	for _, filterTag := range query.FilterTags {
		root.And = append(root.And, *filterTagToExpression(filterTag))
	}
	return root
}

// filterTagToExpression builds the Cost Explorer expression for one tag filter.
// An empty-string value means "resources without this tag" (see infra_sdk.CostFilterTag), which
// Cost Explorer expresses with the ABSENT match option. When both present values and the absent
// marker are requested, the two are OR'd together.
func filterTagToExpression(filterTag infra_sdk.CostFilterTag) *cetypes.Expression {
	key := ptr(UniversalTag(filterTag.Key).ToAws())
	present := filterTag.PresentValues()

	var presentExpr, absentExpr *cetypes.Expression
	if len(present) > 0 {
		presentExpr = &cetypes.Expression{
			Tags: &cetypes.TagValues{
				Key:          key,
				MatchOptions: []cetypes.MatchOption{cetypes.MatchOptionEquals},
				Values:       present,
			},
		}
	}
	if filterTag.MatchesAbsent() {
		absentExpr = &cetypes.Expression{
			Tags: &cetypes.TagValues{
				Key:          key,
				MatchOptions: []cetypes.MatchOption{cetypes.MatchOptionAbsent},
			},
		}
	}

	switch {
	case presentExpr != nil && absentExpr != nil:
		return &cetypes.Expression{Or: []cetypes.Expression{*presentExpr, *absentExpr}}
	case absentExpr != nil:
		return absentExpr
	default:
		return presentExpr
	}
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
