package aws_account

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/apigateway"
	apigwtypes "github.com/aws/aws-sdk-go-v2/service/apigateway/types"
	"github.com/aws/aws-sdk-go-v2/service/apigatewayv2"
	apigwv2types "github.com/aws/aws-sdk-go-v2/service/apigatewayv2/types"
	infra_sdk "github.com/nullstone-io/infra-sdk"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

func ScanApiGateways(ctx context.Context, config aws.Config) ([]infra_sdk.ScanResource, error) {
	client := apigateway.NewFromConfig(config)
	var resources []infra_sdk.ScanResource
	var errs []error

	// Get REST APIs
	restApis, err := getRestApis(ctx, client)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to scan REST APIs: %w", err))
	}
	resources = append(resources, restApis...)

	// Get HTTP APIs (API Gateway v2)
	httpApis, err := getHttpApis(ctx, config)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to scan HTTP APIs: %w", err))
	}
	resources = append(resources, httpApis...)

	if len(errs) > 0 {
		return resources, errors.Join(errs...)
	}
	return resources, nil
}

func getRestApis(ctx context.Context, client *apigateway.Client) ([]infra_sdk.ScanResource, error) {
	var apis []apigwtypes.RestApi
	var position *string

	for {
		output, err := client.GetRestApis(ctx, &apigateway.GetRestApisInput{
			Position: position,
		})
		if err != nil {
			return nil, err
		}

		apis = append(apis, output.Items...)

		if output.Position == nil {
			break
		}
		position = output.Position
	}

	resources := make([]infra_sdk.ScanResource, 0, len(apis))
	for _, api := range apis {
		if api.Id == nil {
			continue
		}

		name := aws.ToString(api.Name)
		if name == "" {
			name = *api.Id
		}

		tags := make(map[string]string)
		if len(api.Tags) > 0 {
			for k, v := range api.Tags {
				tags[k] = v
			}
		}

		resources = append(resources, infra_sdk.ScanResource{
			UniqueId: *api.Id,
			Name:     name,
			Taxonomy: infra_sdk.ResourceTaxonomy{
				Category:    types.CategoryIngress,
				Platform:    "api-gateway",
				Subplatform: "rest-api",
			},
			ServiceName:         "API Gateway",
			ServiceResourceName: "REST API",
			Attributes: map[string]any{
				"endpoint_configuration": api.EndpointConfiguration,
				"created_date":           api.CreatedDate,
				"api_key_source":         api.ApiKeySource,
				"minimum_compression":    api.MinimumCompressionSize,
				"tags":                   tags,
			},
		})
	}

	return resources, nil
}

func getHttpApis(ctx context.Context, config aws.Config) ([]infra_sdk.ScanResource, error) {
	client := apigatewayv2.NewFromConfig(config)
	var apis []apigwv2types.Api
	var nextToken *string

	// List all HTTP APIs
	for {
		output, err := client.GetApis(ctx, &apigatewayv2.GetApisInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list HTTP APIs: %w", err)
		}

		apis = append(apis, output.Items...)

		if output.NextToken == nil {
			break
		}
		nextToken = output.NextToken
	}

	resources := make([]infra_sdk.ScanResource, 0, len(apis))
	for _, api := range apis {
		if api.ApiId == nil {
			continue
		}

		name := aws.ToString(api.Name)
		if name == "" {
			name = *api.ApiId
		}

		// Get tags for the API
		tags := make(map[string]string)
		tagOutput, err := client.GetTags(ctx, &apigatewayv2.GetTagsInput{
			ResourceArn: api.ApiId,
		})
		if err == nil && tagOutput != nil {
			tags = tagOutput.Tags
		}

		// Get the API endpoint
		apiEndpoint := ""
		if api.ApiEndpoint != nil {
			apiEndpoint = *api.ApiEndpoint
		}

		// Get the protocol type
		protocolType := ""
		if api.ProtocolType != "" {
			protocolType = string(api.ProtocolType)
		}

		resources = append(resources, infra_sdk.ScanResource{
			UniqueId: *api.ApiId,
			Name:     name,
			Taxonomy: infra_sdk.ResourceTaxonomy{
				Category:    types.CategoryIngress,
				Platform:    "api-gateway",
				Subplatform: "http-api",
			},
			ServiceName:         "API Gateway",
			ServiceResourceName: "HTTP API",
			Attributes: map[string]any{
				"api_endpoint":                 apiEndpoint,
				"protocol_type":                protocolType,
				"api_id":                       api.ApiId,
				"api_key_selection_expression": api.ApiKeySelectionExpression,
				"created_date":                 api.CreatedDate,
				"description":                  api.Description,
				"disable_execute_api_endpoint": api.DisableExecuteApiEndpoint,
				"version":                      api.Version,
				"tags":                         tags,
			},
		})
	}

	return resources, nil
}
