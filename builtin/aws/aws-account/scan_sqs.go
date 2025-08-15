package aws_account

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	infra_sdk "github.com/nullstone-io/infra-sdk"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

func ScanSqsQueues(ctx context.Context, config aws.Config) ([]infra_sdk.ScanResource, error) {
	client := sqs.NewFromConfig(config)

	// List all SQS queues
	queueUrls, err := client.ListQueues(ctx, &sqs.ListQueuesInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to list SQS queues: %w", err)
	}

	var resources []infra_sdk.ScanResource

	// Process each queue to get its attributes and tags
	for _, queueUrl := range queueUrls.QueueUrls {
		// Get queue attributes
		attrOutput, err := client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
			QueueUrl: aws.String(queueUrl),
			AttributeNames: []sqstypes.QueueAttributeName{
				sqstypes.QueueAttributeNameAll,
			},
		})
		if err != nil {
			continue
		}

		// Get queue tags
		tagOutput, err := client.ListQueueTags(ctx, &sqs.ListQueueTagsInput{
			QueueUrl: aws.String(queueUrl),
		})
		tags := make(map[string]string)
		if err == nil && tagOutput.Tags != nil {
			for k, v := range tagOutput.Tags {
				tags[k] = v
			}
		}

		// Extract queue name from URL (last part of the path)
		queueName := queueUrl
		if queueArn, ok := attrOutput.Attributes[string(sqstypes.QueueAttributeNameQueueArn)]; ok && queueArn != "" {
			queueName = queueArn
		}

		// Determine if this is a FIFO queue
		isFifo := false
		if fifoAttr, ok := attrOutput.Attributes[string(sqstypes.QueueAttributeNameFifoQueue)]; ok {
			isFifo = fifoAttr == "true"
		}

		// Get visibility timeout and message retention period
		visibilityTimeout := ""
		if v, ok := attrOutput.Attributes[string(sqstypes.QueueAttributeNameVisibilityTimeout)]; ok {
			visibilityTimeout = v
		}

		messageRetentionPeriod := ""
		if v, ok := attrOutput.Attributes[string(sqstypes.QueueAttributeNameMessageRetentionPeriod)]; ok {
			messageRetentionPeriod = v
		}

		// Get redrive policy
		redrivePolicy := ""
		if v, ok := attrOutput.Attributes[string(sqstypes.QueueAttributeNameRedrivePolicy)]; ok {
			redrivePolicy = v
		}

		// Get approximate number of messages
		approximateNumberOfMessages := ""
		if v, ok := attrOutput.Attributes[string(sqstypes.QueueAttributeNameApproximateNumberOfMessages)]; ok {
			approximateNumberOfMessages = v
		}

		// Get approximate number of messages not visible
		approximateNumberOfMessagesNotVisible := ""
		if v, ok := attrOutput.Attributes[string(sqstypes.QueueAttributeNameApproximateNumberOfMessagesNotVisible)]; ok {
			approximateNumberOfMessagesNotVisible = v
		}

		// Get approximate number of messages delayed
		approximateNumberOfMessagesDelayed := ""
		if v, ok := attrOutput.Attributes[string(sqstypes.QueueAttributeNameApproximateNumberOfMessagesDelayed)]; ok {
			approximateNumberOfMessagesDelayed = v
		}

		resources = append(resources, infra_sdk.ScanResource{
			UniqueId: queueUrl,
			Name:     queueName,
			Taxonomy: infra_sdk.ResourceTaxonomy{
				Category:    types.CategoryDatastore,
				Platform:    "sqs",
				Subplatform: "",
			},
			ServiceName:         "SQS",
			ServiceResourceName: "Queue",
			Attributes: map[string]any{
				"url":                            queueUrl,
				"arn":                            attrOutput.Attributes[string(sqstypes.QueueAttributeNameQueueArn)],
				"fifo_queue":                     isFifo,
				"visibility_timeout":             visibilityTimeout,
				"message_retention_period":       messageRetentionPeriod,
				"redrive_policy":                 redrivePolicy,
				"approximate_number_of_messages": approximateNumberOfMessages,
				"approximate_number_of_messages_not_visible": approximateNumberOfMessagesNotVisible,
				"approximate_number_of_messages_delayed":     approximateNumberOfMessagesDelayed,
				"created_timestamp":                          attrOutput.Attributes[string(sqstypes.QueueAttributeNameCreatedTimestamp)],
				"last_modified_timestamp":                    attrOutput.Attributes[string(sqstypes.QueueAttributeNameLastModifiedTimestamp)],
				"tags":                                       tags,
			},
		})
	}

	return resources, nil
}
