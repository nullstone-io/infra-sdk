package aws_account

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/mq"
	mqtypes "github.com/aws/aws-sdk-go-v2/service/mq/types"
	infra_sdk "github.com/nullstone-io/infra-sdk"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

func ScanMqBrokers(ctx context.Context, config aws.Config) ([]infra_sdk.ScanResource, error) {
	client := mq.NewFromConfig(config)

	// List all MQ brokers
	brokers, err := client.ListBrokers(ctx, &mq.ListBrokersInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to list MQ brokers: %w", err)
	}

	var brokerIds []string
	for _, broker := range brokers.BrokerSummaries {
		if broker.BrokerId != nil {
			brokerIds = append(brokerIds, *broker.BrokerId)
		}
	}

	var resources []infra_sdk.ScanResource

	// Process brokers in batches of 5 (API limit for DescribeBroker)
	for i := 0; i < len(brokerIds); i += 5 {
		end := i + 5
		if end > len(brokerIds) {
			end = len(brokerIds)
		}
		batch := brokerIds[i:end]

		// Get details for this batch of brokers
		for _, brokerId := range batch {
			descOutput, err := client.DescribeBroker(ctx, &mq.DescribeBrokerInput{
				BrokerId: aws.String(brokerId),
			})
			if err != nil {
				continue
			}

			// Get tags for this broker
			tags := make(map[string]string)
			if descOutput.Tags != nil {
				for k, v := range descOutput.Tags {
					tags[k] = v
				}
			}

			// Get broker engine type
			engineType := ""
			if descOutput.EngineType != "" {
				switch descOutput.EngineType {
				case mqtypes.EngineTypeRabbitmq:
					engineType = "rabbitmq"
				case mqtypes.EngineTypeActivemq:
					engineType = "activemq"
				default:
					engineType = string(descOutput.EngineType)
				}
			}

			// Get instance details
			instanceType := ""
			if descOutput.HostInstanceType != nil {
				instanceType = *descOutput.HostInstanceType
			}

			// Get endpoints
			endpoints := map[string]string{
				"console": "",
			}

			if descOutput.BrokerInstances != nil {
				for _, instance := range descOutput.BrokerInstances {
					if instance.ConsoleURL != nil {
						endpoints["console"] = *instance.ConsoleURL
						break
					}
				}
			}

			// Add endpoints based on engine type
			switch descOutput.EngineType {
			case mqtypes.EngineTypeRabbitmq:
				endpoints["amqp"] = ""
				endpoints["amqps"] = ""
				if descOutput.BrokerInstances != nil {
					for _, instance := range descOutput.BrokerInstances {
						if instance.Endpoints != nil {
							for _, endpoint := range instance.Endpoints {
								endpoints["amqp"] = endpoint
								endpoints["amqps"] = endpoint
							}
						}
					}
				}
			case mqtypes.EngineTypeActivemq:
				endpoints["ssl"] = ""
				endpoints["stomp+ssl"] = ""
				endpoints["websocket"] = ""
				endpoints["wss"] = ""
				if descOutput.BrokerInstances != nil {
					for _, instance := range descOutput.BrokerInstances {
						if instance.Endpoints != nil {
							for _, endpoint := range instance.Endpoints {
								endpoints["ssl"] = endpoint
								endpoints["stomp+ssl"] = endpoint
								endpoints["websocket"] = endpoint
								endpoints["wss"] = endpoint
							}
						}
					}
				}
			}

			// Get deployment mode
			deploymentMode := ""
			if descOutput.DeploymentMode != "" {
				deploymentMode = string(descOutput.DeploymentMode)
			}

			// Get authentication strategy
			authStrategy := ""
			if descOutput.AuthenticationStrategy != "" {
				authStrategy = string(descOutput.AuthenticationStrategy)
			}

			resources = append(resources, infra_sdk.ScanResource{
				UniqueId: *descOutput.BrokerArn,
				Name:     aws.ToString(descOutput.BrokerName),
				Taxonomy: infra_sdk.ResourceTaxonomy{
					Category:    types.CategoryDatastore,
					Platform:    engineType,
					Subplatform: "amazon-mq",
				},
				Attributes: map[string]any{
					"arn":                           descOutput.BrokerArn,
					"broker_id":                     descOutput.BrokerId,
					"broker_state":                  descOutput.BrokerState,
					"engine_version":                descOutput.EngineVersion,
					"instance_type":                 instanceType,
					"deployment_mode":               deploymentMode,
					"authentication_strategy":       authStrategy,
					"publicly_accessible":           descOutput.PubliclyAccessible,
					"auto_minor_version_upgrade":    descOutput.AutoMinorVersionUpgrade,
					"endpoints":                     endpoints,
					"created":                       descOutput.Created,
					"maintenance_window_start_time": getMaintenanceWindow(descOutput.MaintenanceWindowStartTime),
					"logs":                          getLogsConfig(descOutput.Logs),
					"tags":                          tags,
				},
			})
		}
	}

	return resources, nil
}

func getMaintenanceWindow(window *mqtypes.WeeklyStartTime) map[string]any {
	if window == nil {
		return nil
	}
	return map[string]any{
		"day_of_week": string(window.DayOfWeek),
		"time_of_day": window.TimeOfDay,
		"time_zone":   window.TimeZone,
	}
}

func getLogsConfig(logs *mqtypes.LogsSummary) map[string]any {
	if logs == nil {
		return nil
	}
	return map[string]any{
		"audit":   logs.Audit,
		"general": logs.General,
	}
}
