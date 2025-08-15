package aws_account

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cloudfronttypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	infra_sdk "github.com/nullstone-io/infra-sdk"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

func ScanCdns(ctx context.Context, config aws.Config) ([]infra_sdk.ScanResource, error) {
	client := cloudfront.NewFromConfig(config)
	var resources []infra_sdk.ScanResource

	distributions, err := getCloudFrontDistributions(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to scan CloudFront distributions: %w", err)
	}
	resources = append(resources, distributions...)

	return resources, nil
}

func getCloudFrontDistributions(ctx context.Context, client *cloudfront.Client) ([]infra_sdk.ScanResource, error) {
	var distributions []cloudfronttypes.DistributionSummary
	var marker *string

	for {
		output, err := client.ListDistributions(ctx, &cloudfront.ListDistributionsInput{
			Marker: marker,
		})
		if err != nil {
			return nil, err
		}

		if output.DistributionList != nil {
			distributions = append(distributions, output.DistributionList.Items...)

			if !aws.ToBool(output.DistributionList.IsTruncated) {
				break
			}
			marker = output.DistributionList.NextMarker
		} else {
			break
		}
	}

	resources := make([]infra_sdk.ScanResource, 0, len(distributions))
	for _, dist := range distributions {
		if dist.Id == nil {
			continue
		}

		name := aws.ToString(dist.Comment)
		if name == "" {
			name = *dist.Id
		}

		// Get tags for the distribution
		tagList, err := client.ListTagsForResource(ctx, &cloudfront.ListTagsForResourceInput{
			Resource: dist.ARN,
		})
		tags := make(map[string]string)
		if err == nil && tagList != nil && tagList.Tags != nil {
			for _, tag := range tagList.Tags.Items {
				tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
			}
		}

		domains := make([]string, 0, len(dist.Aliases.Items)+1)
		domains = append(domains, aws.ToString(dist.DomainName))
		for _, alias := range dist.Aliases.Items {
			domains = append(domains, alias)
		}

		resources = append(resources, infra_sdk.ScanResource{
			UniqueId: *dist.Id,
			Name:     name,
			Taxonomy: infra_sdk.ResourceTaxonomy{
				Category:    types.CategoryIngress,
				Platform:    "cdn",
				Subplatform: "cloudfront",
			},
			ServiceName:         "CloudFront",
			ServiceResourceName: "Distribution",
			Attributes: map[string]any{
				"status":          dist.Status,
				"enabled":         dist.Enabled,
				"domain_name":     dist.DomainName,
				"domains":         domains,
				"http_version":    dist.HttpVersion,
				"price_class":     dist.PriceClass,
				"is_ipv6_enabled": dist.IsIPV6Enabled,
				"web_acl_id":      dist.WebACLId,
				"tags":            tags,
			},
		})
	}

	return resources, nil
}
