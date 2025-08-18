package aws_account

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	elasticachetypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	infra_sdk "github.com/nullstone-io/infra-sdk"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

func ScanElastiCacheClusters(ctx context.Context, config aws.Config) ([]infra_sdk.ScanResource, error) {
	client := elasticache.NewFromConfig(config)
	rgClient := resourcegroupstaggingapi.NewFromConfig(config)

	// List all ElastiCache clusters
	var cacheClusters []elasticachetypes.CacheCluster
	var marker *string

	for {
		output, err := client.DescribeCacheClusters(ctx, &elasticache.DescribeCacheClustersInput{
			Marker:            marker,
			ShowCacheNodeInfo: aws.Bool(true),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to describe ElastiCache clusters: %w", err)
		}

		cacheClusters = append(cacheClusters, output.CacheClusters...)

		if output.Marker == nil {
			break
		}
		marker = output.Marker
	}

	// Get all cache cluster ARNs for batch tagging
	var clusterArns []string
	for _, cluster := range cacheClusters {
		if cluster.ARN != nil {
			clusterArns = append(clusterArns, *cluster.ARN)
		}
	}

	// Get all tags in a single batch
	tagMap := make(map[string]map[string]string)
	if len(clusterArns) > 0 {
		// Process in batches of 20 (API limit)
		for i := 0; i < len(clusterArns); i += 20 {
			end := i + 20
			if end > len(clusterArns) {
				end = len(clusterArns)
			}
			batch := clusterArns[i:end]

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

	// Process each cache cluster
	for _, cluster := range cacheClusters {
		if cluster.ARN == nil {
			continue
		}

		// Get cluster details
		nodeType := ""
		if cluster.CacheNodeType != nil {
			nodeType = *cluster.CacheNodeType
		}

		engine := ""
		if cluster.Engine != nil {
			engine = *cluster.Engine
		}

		engineVersion := ""
		if cluster.EngineVersion != nil {
			engineVersion = *cluster.EngineVersion
		}

		// Get endpoints
		var endpoints []map[string]string
		for _, node := range cluster.CacheNodes {
			if node.Endpoint != nil {
				endpoints = append(endpoints, map[string]string{
					"address": aws.ToString(node.Endpoint.Address),
					"port":    fmt.Sprintf("%d", aws.ToInt32(node.Endpoint.Port)),
				})
			}
		}

		// Get security groups
		var securityGroups []map[string]string
		for _, sg := range cluster.SecurityGroups {
			securityGroups = append(securityGroups, map[string]string{
				"security_group_id": aws.ToString(sg.SecurityGroupId),
				"status":            aws.ToString(sg.Status),
			})
		}

		// Get cache parameter group
		var parameterGroup string
		if cluster.CacheParameterGroup != nil {
			parameterGroup = *cluster.CacheParameterGroup.CacheParameterGroupName
		}

		// Get cache subnet group
		subnetGroup := ""
		if cluster.CacheSubnetGroupName != nil {
			subnetGroup = *cluster.CacheSubnetGroupName
		}

		// Get maintenance window
		maintenanceWindow := ""
		if cluster.PreferredMaintenanceWindow != nil {
			maintenanceWindow = *cluster.PreferredMaintenanceWindow
		}

		// Get notification configuration
		notificationConfig := ""
		if cluster.NotificationConfiguration != nil && cluster.NotificationConfiguration.TopicArn != nil {
			notificationConfig = *cluster.NotificationConfiguration.TopicArn
		}

		resources = append(resources, infra_sdk.ScanResource{
			UniqueId: *cluster.ARN,
			Name:     *cluster.CacheClusterId,
			Taxonomy: infra_sdk.ResourceTaxonomy{
				Category:    types.CategoryDatastore,
				Platform:    engine, // redis or memcached
				Subplatform: "elasticache",
				Provider:    "aws",
			},
			ServiceName:         "ElastiCache",
			ServiceResourceName: "Cluster",
			Attributes: map[string]any{
				"arn":                          cluster.ARN,
				"engine":                       engine,
				"engine_version":               engineVersion,
				"node_type":                    nodeType,
				"num_cache_nodes":              cluster.NumCacheNodes,
				"endpoints":                    endpoints,
				"security_groups":              securityGroups,
				"parameter_group":              parameterGroup,
				"subnet_group":                 subnetGroup,
				"maintenance_window":           maintenanceWindow,
				"notification_configuration":   notificationConfig,
				"auto_minor_version_upgrade":   cluster.AutoMinorVersionUpgrade,
				"cache_cluster_status":         cluster.CacheClusterStatus,
				"cache_node_type":              cluster.CacheNodeType,
				"cache_parameter_group":        parameterGroup,
				"cache_security_groups":        securityGroups,
				"cache_subnet_group_name":      subnetGroup,
				"client_download_landing_page": cluster.ClientDownloadLandingPage,
				"configuration_endpoint":       cluster.ConfigurationEndpoint,
				"preferred_availability_zone":  cluster.PreferredAvailabilityZone,
				"preferred_maintenance_window": maintenanceWindow,
				"replication_group_id":         cluster.ReplicationGroupId,
				"snapshot_retention_limit":     cluster.SnapshotRetentionLimit,
				"snapshot_window":              cluster.SnapshotWindow,
				"tags":                         tagMap[aws.ToString(cluster.ARN)],
			},
		})
	}

	// Also scan Redis replication groups
	replicationGroups, err := listReplicationGroups(ctx, client)
	if err == nil {
		for _, rg := range replicationGroups {
			// Skip if we've already processed this as a primary cluster
			found := false
			for _, cluster := range cacheClusters {
				if cluster.ReplicationGroupId != nil && *cluster.ReplicationGroupId == *rg.ReplicationGroupId {
					found = true
					break
				}
			}
			if found {
				continue
			}

			// Get tags for this replication group
			tags := make(map[string]string)
			if rg.ARN != nil {
				tags = tagMap[aws.ToString(rg.ARN)]
			}

			resources = append(resources, infra_sdk.ScanResource{
				UniqueId: *rg.ARN,
				Name:     *rg.ReplicationGroupId,
				Taxonomy: infra_sdk.ResourceTaxonomy{
					Category:    types.CategoryDatastore,
					Platform:    "redis", // Replication groups are Redis-specific
					Subplatform: "elasticache",
					Provider:    "aws",
				},
				ServiceName:         "ElastiCache",
				ServiceResourceName: "Cluster",
				Attributes: map[string]any{
					"arn":                      rg.ARN,
					"status":                   rg.Status,
					"description":              rg.Description,
					"node_groups":              rg.NodeGroups,
					"automatic_failover":       string(rg.AutomaticFailover),
					"multi_az":                 string(rg.MultiAZ),
					"configuration_endpoint":   rg.ConfigurationEndpoint,
					"snapshot_retention_limit": rg.SnapshotRetentionLimit,
					"snapshot_window":          rg.SnapshotWindow,
					"cluster_enabled":          rg.ClusterEnabled,
					"tags":                     tags,
				},
			})
		}
	}

	return resources, nil
}

func listReplicationGroups(ctx context.Context, client *elasticache.Client) ([]elasticachetypes.ReplicationGroup, error) {
	var replicationGroups []elasticachetypes.ReplicationGroup
	var marker *string

	for {
		output, err := client.DescribeReplicationGroups(ctx, &elasticache.DescribeReplicationGroupsInput{
			Marker: marker,
		})
		if err != nil {
			return nil, err
		}

		replicationGroups = append(replicationGroups, output.ReplicationGroups...)

		if output.Marker == nil {
			break
		}
		marker = output.Marker
	}

	return replicationGroups, nil
}
