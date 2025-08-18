package aws_account

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
	infra_sdk "github.com/nullstone-io/infra-sdk"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

func ScanSnsTopics(ctx context.Context, config aws.Config) ([]infra_sdk.ScanResource, error) {
	client := sns.NewFromConfig(config)
	rgClient := resourcegroupstaggingapi.NewFromConfig(config)

	// List all SNS topics
	var topics []snstypes.Topic
	var nextToken *string

	for {
		output, err := client.ListTopics(ctx, &sns.ListTopicsInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list SNS topics: %w", err)
		}

		topics = append(topics, output.Topics...)

		if output.NextToken == nil {
			break
		}
		nextToken = output.NextToken
	}

	// Get all topic ARNs for batch tagging
	var topicArns []string
	for _, topic := range topics {
		topicArns = append(topicArns, *topic.TopicArn)
	}

	// Get all tags in a single batch
	tagMap := make(map[string]map[string]string)
	if len(topicArns) > 0 {
		// Process in batches of 20 (API limit)
		for i := 0; i < len(topicArns); i += 20 {
			end := i + 20
			if end > len(topicArns) {
				end = len(topicArns)
			}
			batch := topicArns[i:end]

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

	// Process each topic to get its details
	for _, topic := range topics {
		if topic.TopicArn == nil {
			continue
		}

		// Get topic attributes
		attrs, err := client.GetTopicAttributes(ctx, &sns.GetTopicAttributesInput{
			TopicArn: topic.TopicArn,
		})
		if err != nil {
			continue
		}

		// Get topic subscriptions
		subscriptions, err := listSubscriptionsByTopic(ctx, client, *topic.TopicArn)
		if err != nil {
			continue
		}

		// Get topic policy
		var policy map[string]any
		if attrs.Attributes["Policy"] != "" {
			policy = map[string]any{
				"policy": attrs.Attributes["Policy"],
			}
		}

		// Get delivery status logging
		deliveryStatusLogging := make([]map[string]any, 0)
		if attrs.Attributes["ApplicationSuccessFeedbackRoleArn"] != "" || attrs.Attributes["HTTPSuccessFeedbackRoleArn"] != "" {
			logEntry := map[string]any{
				"application_success_feedback_role_arn":    attrs.Attributes["ApplicationSuccessFeedbackRoleArn"],
				"application_success_feedback_sample_rate": attrs.Attributes["ApplicationSuccessFeedbackSampleRate"],
				"http_success_feedback_role_arn":           attrs.Attributes["HTTPSuccessFeedbackRoleArn"],
				"http_success_feedback_sample_rate":        attrs.Attributes["HTTPSuccessFeedbackSampleRate"],
				"lambda_success_feedback_role_arn":         attrs.Attributes["LambdaSuccessFeedbackRoleArn"],
				"lambda_success_feedback_sample_rate":      attrs.Attributes["LambdaSuccessFeedbackSampleRate"],
			}
			deliveryStatusLogging = append(deliveryStatusLogging, logEntry)
		}

		// Get topic display name
		displayName := ""
		if name, ok := attrs.Attributes["DisplayName"]; ok && name != "" {
			displayName = name
		} else {
			// Use the last part of the ARN as a fallback
			displayName = *topic.TopicArn
		}

		resources = append(resources, infra_sdk.ScanResource{
			UniqueId: *topic.TopicArn,
			Name:     displayName,
			Taxonomy: infra_sdk.ResourceTaxonomy{
				Category:    types.CategoryDatastore,
				Platform:    "sns",
				Subplatform: "",
				Provider:    "aws",
			},
			ServiceName:         "SNS",
			ServiceResourceName: "Topic",
			Attributes: map[string]any{
				"arn":                         topic.TopicArn,
				"owner":                       attrs.Attributes["Owner"],
				"subscriptions_confirmed":     attrs.Attributes["SubscriptionsConfirmed"],
				"subscriptions_deleted":       attrs.Attributes["SubscriptionsDeleted"],
				"subscriptions_pending":       attrs.Attributes["SubscriptionsPending"],
				"kms_master_key_id":           attrs.Attributes["KmsMasterKeyId"],
				"fifo_topic":                  attrs.Attributes["FifoTopic"] == "true",
				"content_based_deduplication": attrs.Attributes["ContentBasedDeduplication"] == "true",
				"policy":                      policy,
				"delivery_status_logging":     deliveryStatusLogging,
				"subscriptions":               subscriptions,
				"tags":                        tagMap[aws.ToString(topic.TopicArn)],
			},
		})
	}

	return resources, nil
}

func listSubscriptionsByTopic(ctx context.Context, client *sns.Client, topicArn string) ([]map[string]any, error) {
	var subscriptions []map[string]any
	var nextToken *string

	for {
		output, err := client.ListSubscriptionsByTopic(ctx, &sns.ListSubscriptionsByTopicInput{
			TopicArn:  &topicArn,
			NextToken: nextToken,
		})
		if err != nil {
			return nil, err
		}

		for _, sub := range output.Subscriptions {
			subscriptions = append(subscriptions, map[string]any{
				"subscription_arn": sub.SubscriptionArn,
				"protocol":         sub.Protocol,
				"endpoint":         sub.Endpoint,
				"owner":            sub.Owner,
				"topic_arn":        sub.TopicArn,
			})
		}

		if output.NextToken == nil {
			break
		}
		nextToken = output.NextToken
	}

	return subscriptions, nil
}
