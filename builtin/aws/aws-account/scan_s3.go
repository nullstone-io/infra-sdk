package aws_account

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	infra_sdk "github.com/nullstone-io/infra-sdk"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

func ScanS3Buckets(ctx context.Context, config aws.Config) ([]infra_sdk.ScanResource, error) {
	client := s3.NewFromConfig(config)
	rgClient := resourcegroupstaggingapi.NewFromConfig(config)

	// List all S3 buckets
	buckets, err := client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to list S3 buckets: %w", err)
	}

	// Get all bucket ARNs for batch tagging
	var bucketArns []string
	for _, bucket := range buckets.Buckets {
		arn := fmt.Sprintf("arn:aws:s3:::%s", *bucket.Name)
		bucketArns = append(bucketArns, arn)
	}

	// Get all tags in a single batch
	tagMap := make(map[string]map[string]string)
	if len(bucketArns) > 0 {
		// Process in batches of 20 (API limit)
		for i := 0; i < len(bucketArns); i += 20 {
			end := i + 20
			if end > len(bucketArns) {
				end = len(bucketArns)
			}
			batch := bucketArns[i:end]

			tagOutput, err := rgClient.GetResources(ctx, &resourcegroupstaggingapi.GetResourcesInput{
				ResourceARNList: batch,
			})
			if err == nil {
				for _, resource := range tagOutput.ResourceTagMappingList {
					tags := make(map[string]string)
					for _, tag := range resource.Tags {
						tags[*tag.Key] = *tag.Value
					}
					tagMap[aws.ToString(resource.ResourceARN)] = tags
				}
			}
		}
	}

	var resources []infra_sdk.ScanResource

	// Process each bucket to get its details
	for _, bucket := range buckets.Buckets {
		bucketName := *bucket.Name
		arn := fmt.Sprintf("arn:aws:s3:::%s", bucketName)

		// Get bucket location
		locationOutput, err := client.GetBucketLocation(ctx, &s3.GetBucketLocationInput{
			Bucket: &bucketName,
		})
		if err != nil {
			continue
		}

		// Get CORS configuration
		corsOutput, _ := client.GetBucketCors(ctx, &s3.GetBucketCorsInput{
			Bucket: &bucketName,
		})

		// Get website configuration
		websiteOutput, _ := client.GetBucketWebsite(ctx, &s3.GetBucketWebsiteInput{
			Bucket: &bucketName,
		})

		resources = append(resources, infra_sdk.ScanResource{
			UniqueId: arn,
			Name:     bucketName,
			Taxonomy: infra_sdk.ResourceTaxonomy{
				Category:    types.CategoryDatastore,
				Platform:    "s3",
				Subplatform: "",
				Provider:    "aws",
			},
			ServiceName:         "S3",
			ServiceResourceName: "Bucket",
			Attributes: map[string]any{
				"arn":           arn,
				"creation_date": bucket.CreationDate,
				"region":        getBucketRegion(locationOutput.LocationConstraint),
				"cors":          getCorsDetails(corsOutput),
				"website":       getWebsiteDetails(websiteOutput),
				"tags":          tagMap[arn],
			},
		})
	}

	return resources, nil
}

// Helper functions for S3 bucket details

func getBucketRegion(constraint s3types.BucketLocationConstraint) string {
	if constraint == "" {
		return "us-east-1" // Default region for S3
	}
	return string(constraint)
}

func getCorsDetails(output *s3.GetBucketCorsOutput) map[string]any {
	if output == nil || len(output.CORSRules) == 0 {
		return nil
	}
	rules := make([]map[string]any, len(output.CORSRules))
	for i, rule := range output.CORSRules {
		rules[i] = map[string]any{
			"allowed_headers": rule.AllowedHeaders,
			"allowed_methods": rule.AllowedMethods,
			"allowed_origins": rule.AllowedOrigins,
			"expose_headers":  rule.ExposeHeaders,
			"max_age_seconds": rule.MaxAgeSeconds,
		}
	}
	return map[string]any{
		"cors_rules": rules,
	}
}

func getWebsiteDetails(output *s3.GetBucketWebsiteOutput) map[string]any {
	if output == nil {
		return nil
	}
	return map[string]any{
		"index_document":           output.IndexDocument,
		"error_document":           output.ErrorDocument,
		"redirect_all_requests_to": output.RedirectAllRequestsTo,
		"routing_rules":            output.RoutingRules,
	}
}
