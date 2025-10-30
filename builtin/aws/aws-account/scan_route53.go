package aws_account

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	r53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	infra_sdk "github.com/nullstone-io/infra-sdk"
	"golang.org/x/net/publicsuffix"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

// ScanRoute53 scans Route53 hosted zones and returns them as scan resources
// A hosted zone is considered a subdomain if its name is not a top-level domain (has more than one dot)
func ScanRoute53(ctx context.Context, config aws.Config) ([]infra_sdk.ScanResource, error) {
	client := route53.NewFromConfig(config)
	var resources []infra_sdk.ScanResource

	// List all hosted zones
	zones, err := listHostedZones(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to list hosted zones: %w", err)
	}

	// Convert hosted zones to scan resources
	for _, zone := range zones {
		category := types.CategoryDomain
		if isSubdomain(aws.ToString(zone.Name)) {
			category = types.CategorySubdomain
		}

		// Get tags for the hosted zone
		tags, err := getHostedZoneTags(ctx, client, aws.ToString(zone.Id))
		if err != nil {
			return nil, fmt.Errorf("error getting tags for hosted zone %s: %w", aws.ToString(zone.Name), err)
		}

		var comment string
		var privateZone bool
		if zone.Config != nil {
			comment = aws.ToString(zone.Config.Comment)
			privateZone = zone.Config.PrivateZone
		}

		resources = append(resources, infra_sdk.ScanResource{
			UniqueId: aws.ToString(zone.Id),
			Name:     aws.ToString(zone.Name),
			Taxonomy: infra_sdk.ResourceTaxonomy{
				Category: category,
				Provider: "aws",
				Platform: "route53",
			},
			ServiceName:         "Route53",
			ServiceResourceName: "Hosted Zone",
			Attributes: map[string]interface{}{
				"name":                      strings.TrimSuffix(aws.ToString(zone.Name), "."),
				"zone_id":                   aws.ToString(zone.Id),
				"comment":                   comment,
				"private_zone":              privateZone,
				"resource_record_set_count": aws.ToInt64(zone.ResourceRecordSetCount),
				"tags":                      tags,
			},
		})
	}

	return resources, nil
}

// getHostedZoneTags retrieves tags for a specific hosted zone
func getHostedZoneTags(ctx context.Context, client *route53.Client, zoneId string) (map[string]string, error) {
	tags := make(map[string]string)
	// Extract the hosted zone ID from the full ARN if needed
	// Hosted zone ID is in the format "/hostedzone/Z1234567890ABC"
	zoneId = strings.TrimPrefix(zoneId, "/hostedzone/")

	output, err := client.ListTagsForResource(ctx, &route53.ListTagsForResourceInput{
		ResourceType: r53types.TagResourceTypeHostedzone,
		ResourceId:   &zoneId,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get tags for hosted zone %s: %w", zoneId, err)
	}

	for _, tag := range output.ResourceTagSet.Tags {
		tags[unptr(tag.Key)] = unptr(tag.Value)
	}

	return tags, nil
}

// listHostedZones retrieves all Route53 hosted zones
func listHostedZones(ctx context.Context, client *route53.Client) ([]r53types.HostedZone, error) {
	var zones []r53types.HostedZone
	var marker *string

	for {
		output, err := client.ListHostedZones(ctx, &route53.ListHostedZonesInput{
			Marker: marker,
		})
		if err != nil {
			return nil, err
		}

		zones = append(zones, output.HostedZones...)

		if !output.IsTruncated {
			break
		}
		marker = output.NextMarker
	}

	return zones, nil
}

func isSubdomain(domain string) bool {
	sld, _ := publicsuffix.EffectiveTLDPlusOne(domain)
	return sld != domain
}
