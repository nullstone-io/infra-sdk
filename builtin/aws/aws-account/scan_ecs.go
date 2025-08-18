package aws_account

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	infra_sdk "github.com/nullstone-io/infra-sdk"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
	"slices"
)

func ScanEcsClusters(ctx context.Context, config aws.Config) ([]infra_sdk.ScanResource, error) {
	ecsClient := ecs.NewFromConfig(config)

	// List all clusters
	listOutput, err := ecsClient.ListClusters(ctx, &ecs.ListClustersInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to list ECS clusters: %w", err)
	}
	if len(listOutput.ClusterArns) == 0 {
		return []infra_sdk.ScanResource{}, nil
	}

	// Describe all clusters to get their details
	descOutput, err := ecsClient.DescribeClusters(ctx, &ecs.DescribeClustersInput{
		Clusters: listOutput.ClusterArns,
		Include:  []ecstypes.ClusterField{ecstypes.ClusterFieldTags, ecstypes.ClusterFieldSettings, ecstypes.ClusterFieldStatistics},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe ECS clusters: %w", err)
	}

	resources := make([]infra_sdk.ScanResource, 0, len(descOutput.Clusters))
	for _, cluster := range descOutput.Clusters {
		if cluster.Status != nil && *cluster.Status == "INACTIVE" {
			continue
		}

		// Collect capacity providers
		capacityProviders := make([]string, 0)
		for _, provider := range cluster.CapacityProviders {
			capacityProviders = append(capacityProviders, provider)
		}

		// Determine cluster type (EC2 or Fargate)
		clusterType := "ec2"
		serviceName := "ECS"
		if slices.Contains(capacityProviders, "FARGATE") {
			clusterType = "fargate"
			serviceName = "Fargate"
		}

		// Collect cluster settings
		settings := make(map[string]any)
		for _, setting := range cluster.Settings {
			settings[string(setting.Name)] = setting.Value
		}

		// Get statistics if available
		stats := make(map[string]string)
		for _, stat := range cluster.Statistics {
			stats[aws.ToString(stat.Name)] = aws.ToString(stat.Value)
		}

		// Get tags as map
		tags := make(map[string]string)
		for _, tag := range cluster.Tags {
			tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
		}

		name := aws.ToString(cluster.ClusterName)
		resources = append(resources, infra_sdk.ScanResource{
			UniqueId: aws.ToString(cluster.ClusterArn),
			Name:     name,
			Taxonomy: infra_sdk.ResourceTaxonomy{
				Category:    types.CategoryCluster,
				Subcategory: "",
				Platform:    "ecs",
				Subplatform: clusterType,
				Provider:    "aws",
			},
			ServiceName:         serviceName,
			ServiceResourceName: "Cluster",
			Attributes: map[string]any{
				"status":             cluster.Status,
				"running_tasks":      cluster.RunningTasksCount,
				"pending_tasks":      cluster.PendingTasksCount,
				"active_services":    cluster.ActiveServicesCount,
				"statistics":         stats,
				"settings":           settings,
				"capacity_providers": capacityProviders,
				"tags":               tags,
			},
		})
	}

	return resources, nil
}
