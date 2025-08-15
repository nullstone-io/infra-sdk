package aws_account

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kafka"
	kafkatypes "github.com/aws/aws-sdk-go-v2/service/kafka/types"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	infra_sdk "github.com/nullstone-io/infra-sdk"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

func ScanMskClusters(ctx context.Context, config aws.Config) ([]infra_sdk.ScanResource, error) {
	client := kafka.NewFromConfig(config)
	rgClient := resourcegroupstaggingapi.NewFromConfig(config)

	// Get all MSK clusters
	clusters, err := client.ListClustersV2(ctx, &kafka.ListClustersV2Input{})
	if err != nil {
		return nil, fmt.Errorf("failed to list MSK clusters: %w", err)
	}

	// Get all tags for MSK resources in a single batch
	var resourceArns []string
	for _, cluster := range clusters.ClusterInfoList {
		if cluster.ClusterArn != nil {
			resourceArns = append(resourceArns, *cluster.ClusterArn)
		}
	}

	tagMap := make(map[string]map[string]string)
	if len(resourceArns) > 0 {
		// Get all tags for the clusters in batches of 20 (API limit)
		for i := 0; i < len(resourceArns); i += 20 {
			end := i + 20
			if end > len(resourceArns) {
				end = len(resourceArns)
			}
			batch := resourceArns[i:end]

			output, err := rgClient.GetResources(ctx, &resourcegroupstaggingapi.GetResourcesInput{
				ResourceARNList: batch,
			})
			if err != nil {
				continue
			}

			for _, resource := range output.ResourceTagMappingList {
				tags := make(map[string]string)
				for _, tag := range resource.Tags {
					tags[*tag.Key] = *tag.Value
				}
				tagMap[aws.ToString(resource.ResourceARN)] = tags
			}
		}
	}

	resources := make([]infra_sdk.ScanResource, 0, len(clusters.ClusterInfoList))
	for _, cluster := range clusters.ClusterInfoList {
		if cluster.ClusterArn == nil {
			continue
		}

		name := aws.ToString(cluster.ClusterName)
		if name == "" {
			name = *cluster.ClusterArn
		}

		// Get broker details
		var kafkaVersion string
		var brokerCount int32
		var zookeeperConnectString string
		if cluster.Provisioned != nil {
			brokerCount = aws.ToInt32(cluster.Provisioned.NumberOfBrokerNodes)
			zookeeperConnectString = aws.ToString(cluster.Provisioned.ZookeeperConnectString)
			if cluster.Provisioned.CurrentBrokerSoftwareInfo != nil {
				kafkaVersion = aws.ToString(cluster.Provisioned.CurrentBrokerSoftwareInfo.KafkaVersion)
			}
		}
		if cluster.ClusterType == kafkatypes.ClusterTypeServerless {
			kafkaVersion = "Serverless"
		}

		resources = append(resources, infra_sdk.ScanResource{
			UniqueId: *cluster.ClusterArn,
			Name:     name,
			Taxonomy: infra_sdk.ResourceTaxonomy{
				Category:    types.CategoryDatastore,
				Platform:    "kafka",
				Subplatform: "msk",
			},
			ServiceName:         "MSK",
			ServiceResourceName: "Cluster",
			Attributes: map[string]any{
				"arn":                    cluster.ClusterArn,
				"cluster_type":           string(cluster.ClusterType),
				"state":                  cluster.State,
				"kafka_version":          kafkaVersion,
				"number_of_broker_nodes": brokerCount,
				"zookeeper_connect":      zookeeperConnectString,
				"creation_time":          cluster.CreationTime,
				"tags":                   tagMap[aws.ToString(cluster.ClusterArn)],
			},
		})
	}

	return resources, nil
}
