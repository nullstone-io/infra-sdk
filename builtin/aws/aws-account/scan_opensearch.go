package aws_account

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	opensearchtypes "github.com/aws/aws-sdk-go-v2/service/opensearch/types"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	infra_sdk "github.com/nullstone-io/infra-sdk"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

func ScanOpenSearchDomains(ctx context.Context, config aws.Config) ([]infra_sdk.ScanResource, error) {
	client := opensearch.NewFromConfig(config)
	rgClient := resourcegroupstaggingapi.NewFromConfig(config)

	// List all OpenSearch domains
	domains, err := client.ListDomainNames(ctx, &opensearch.ListDomainNamesInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to list OpenSearch domains: %w", err)
	}

	// Get domain details in batches
	var domainNames []string
	for _, domain := range domains.DomainNames {
		if domain.DomainName != nil {
			domainNames = append(domainNames, *domain.DomainName)
		}
	}

	var resources []infra_sdk.ScanResource

	// Process domains in batches of 5 (API limit for DescribeDomains)
	for i := 0; i < len(domainNames); i += 5 {
		end := i + 5
		if end > len(domainNames) {
			end = len(domainNames)
		}
		batch := domainNames[i:end]

		descOutput, err := client.DescribeDomains(ctx, &opensearch.DescribeDomainsInput{
			DomainNames: batch,
		})
		if err != nil {
			continue
		}

		// Get tags for all domains in this batch
		var arns []string
		for _, domain := range descOutput.DomainStatusList {
			if domain.ARN != nil {
				arns = append(arns, *domain.ARN)
			}
		}

		tagMap := make(map[string]map[string]string)
		if len(arns) > 0 {
			tagOutput, err := rgClient.GetResources(ctx, &resourcegroupstaggingapi.GetResourcesInput{
				ResourceARNList: arns,
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

		for _, domain := range descOutput.DomainStatusList {
			if domain.ARN == nil {
				continue
			}

			name := aws.ToString(domain.DomainName)
			if name == "" {
				name = *domain.ARN
			}

			// Get endpoint information
			endpoint := ""
			if domain.Endpoint != nil {
				endpoint = *domain.Endpoint
			}

			// Get version information
			version := ""
			if domain.EngineVersion != nil {
				version = *domain.EngineVersion
			}

			// Get instance type and count
			instanceType := ""
			instanceCount := int32(0)
			if domain.ClusterConfig != nil {
				if domain.ClusterConfig.InstanceType != "" {
					instanceType = string(domain.ClusterConfig.InstanceType)
				}
				if domain.ClusterConfig.InstanceCount != nil {
					instanceCount = *domain.ClusterConfig.InstanceCount
				}
			}

			// Get encryption at rest status
			encryptionAtRest := false
			if domain.EncryptionAtRestOptions != nil {
				encryptionAtRest = domain.EncryptionAtRestOptions.Enabled != nil && *domain.EncryptionAtRestOptions.Enabled
			}

			// Get node-to-node encryption status
			nodeToNodeEncryption := false
			if domain.NodeToNodeEncryptionOptions != nil {
				nodeToNodeEncryption = domain.NodeToNodeEncryptionOptions.Enabled != nil && *domain.NodeToNodeEncryptionOptions.Enabled
			}

			resources = append(resources, infra_sdk.ScanResource{
				UniqueId: *domain.ARN,
				Name:     name,
				Taxonomy: infra_sdk.ResourceTaxonomy{
					Category:    types.CategoryDatastore,
					Platform:    "elasticsearch",
					Subplatform: "opensearch",
				},
				ServiceName:         "OpenSearch",
				ServiceResourceName: "Domain",
				Attributes: map[string]any{
					"arn":                       domain.ARN,
					"endpoint":                  endpoint,
					"version":                   version,
					"instance_type":             instanceType,
					"instance_count":            instanceCount,
					"encryption_at_rest":        encryptionAtRest,
					"node_to_node_encryption":   nodeToNodeEncryption,
					"created":                   domain.Created,
					"deleted":                   domain.Deleted,
					"processing":                domain.Processing,
					"upgrade_processing":        domain.UpgradeProcessing,
					"access_policies":           domain.AccessPolicies,
					"advanced_options":          domain.AdvancedOptions,
					"advanced_security_options": getAdvancedSecurityOptions(domain.AdvancedSecurityOptions),
					"tags":                      tagMap[aws.ToString(domain.ARN)],
				},
			})
		}
	}

	return resources, nil
}

func getAdvancedSecurityOptions(options *opensearchtypes.AdvancedSecurityOptions) map[string]any {
	if options == nil {
		return nil
	}
	return map[string]any{
		"enabled":                        options.Enabled != nil && *options.Enabled,
		"internal_user_database_enabled": options.InternalUserDatabaseEnabled != nil && *options.InternalUserDatabaseEnabled,
		"saml_enabled":                   options.SAMLOptions != nil && options.SAMLOptions.Enabled != nil && *options.SAMLOptions.Enabled,
	}
}
