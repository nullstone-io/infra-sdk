package aws_account

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	efstypes "github.com/aws/aws-sdk-go-v2/service/efs/types"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	infra_sdk "github.com/nullstone-io/infra-sdk"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

func ScanEfsFileSystems(ctx context.Context, config aws.Config) ([]infra_sdk.ScanResource, error) {
	client := efs.NewFromConfig(config)
	rgClient := resourcegroupstaggingapi.NewFromConfig(config)

	// List all EFS file systems
	fileSystems, err := client.DescribeFileSystems(ctx, &efs.DescribeFileSystemsInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to describe EFS file systems: %w", err)
	}

	// Get all file system ARNs for batch tagging
	var fileSystemArns []string
	for _, fs := range fileSystems.FileSystems {
		if fs.FileSystemArn != nil {
			fileSystemArns = append(fileSystemArns, *fs.FileSystemArn)
		}
	}

	// Get all tags in a single batch
	tagMap := make(map[string]map[string]string)
	if len(fileSystemArns) > 0 {
		// Process in batches of 20 (API limit)
		for i := 0; i < len(fileSystemArns); i += 20 {
			end := i + 20
			if end > len(fileSystemArns) {
				end = len(fileSystemArns)
			}
			batch := fileSystemArns[i:end]

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

	// Process each file system
	for _, fs := range fileSystems.FileSystems {
		if fs.FileSystemId == nil {
			continue
		}

		// Get mount targets for this file system
		mountTargets, err := client.DescribeMountTargets(ctx, &efs.DescribeMountTargetsInput{
			FileSystemId: fs.FileSystemId,
		})
		if err != nil {
			continue
		}

		// Get mount target details
		var mountTargetDetails []map[string]any
		for _, mt := range mountTargets.MountTargets {
			mtDetail := map[string]any{
				"mount_target_id":      aws.ToString(mt.MountTargetId),
				"subnet_id":            aws.ToString(mt.SubnetId),
				"lifecycle_state":      string(mt.LifeCycleState),
				"ip_address":           aws.ToString(mt.IpAddress),
				"availability_zone":    aws.ToString(mt.AvailabilityZoneName),
				"availability_zone_id": aws.ToString(mt.AvailabilityZoneId),
				"owner_id":             aws.ToString(mt.OwnerId),
			}

			// Get network interface ID if available
			if mt.NetworkInterfaceId != nil {
				mtDetail["network_interface_id"] = *mt.NetworkInterfaceId
			}

			mountTargetDetails = append(mountTargetDetails, mtDetail)
		}

		// Get file system policy
		var policy map[string]any
		policyOutput, err := client.DescribeFileSystemPolicy(ctx, &efs.DescribeFileSystemPolicyInput{
			FileSystemId: fs.FileSystemId,
		})
		if err == nil && policyOutput.Policy != nil {
			policy = map[string]any{
				"policy": *policyOutput.Policy,
			}
		}

		// Get file system protection
		var protection map[string]any
		if fs.FileSystemProtection != nil {
			protection = map[string]any{
				"replication_overwrite_protection": string(fs.FileSystemProtection.ReplicationOverwriteProtection),
			}
		}

		// Get throughput mode and provisioned throughput
		throughputMode := ""
		if fs.ThroughputMode != "" {
			throughputMode = string(fs.ThroughputMode)
		}

		provisionedThroughputInMibps := float64(0)
		if fs.ProvisionedThroughputInMibps != nil {
			provisionedThroughputInMibps = *fs.ProvisionedThroughputInMibps
		}

		// Create the resource
		resources = append(resources, infra_sdk.ScanResource{
			UniqueId: *fs.FileSystemArn,
			Name:     *fs.Name,
			Taxonomy: infra_sdk.ResourceTaxonomy{
				Category:    types.CategoryDatastore,
				Platform:    "nfs",
				Subplatform: "efs",
				Provider:    "aws",
			},
			ServiceName:         "EFS",
			ServiceResourceName: "File System",
			Attributes: map[string]any{
				"arn":                             fs.FileSystemArn,
				"file_system_id":                  fs.FileSystemId,
				"creation_token":                  fs.CreationToken,
				"creation_time":                   fs.CreationTime,
				"life_cycle_state":                string(fs.LifeCycleState),
				"number_of_mount_targets":         fs.NumberOfMountTargets,
				"owner_id":                        fs.OwnerId,
				"size_in_bytes":                   getSizeInBytes(fs.SizeInBytes),
				"performance_mode":                string(fs.PerformanceMode),
				"encrypted":                       fs.Encrypted,
				"kms_key_id":                      fs.KmsKeyId,
				"throughput_mode":                 throughputMode,
				"provisioned_throughput_in_mibps": provisionedThroughputInMibps,
				"availability_zone_name":          fs.AvailabilityZoneName,
				"availability_zone_id":            fs.AvailabilityZoneId,
				"mount_targets":                   mountTargetDetails,
				"file_system_policy":              policy,
				"file_system_protection":          protection,
				"tags":                            tagMap[aws.ToString(fs.FileSystemArn)],
			},
		})
	}

	return resources, nil
}

func getSizeInBytes(size *efstypes.FileSystemSize) map[string]any {
	if size == nil {
		return nil
	}

	result := map[string]any{
		"value":     size.Value,
		"timestamp": size.Timestamp,
	}

	if size.ValueInIA != nil {
		result["value_in_ia"] = *size.ValueInIA
	}
	if size.ValueInStandard != nil {
		result["value_in_standard"] = *size.ValueInStandard
	}
	if size.ValueInArchive != nil {
		result["value_in_archive"] = *size.ValueInArchive
	}

	return result
}
