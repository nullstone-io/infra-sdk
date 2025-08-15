package aws_account

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	infra_sdk "github.com/nullstone-io/infra-sdk"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

func ScanLoadBalancers(ctx context.Context, config aws.Config) ([]infra_sdk.ScanResource, error) {
	var resources []infra_sdk.ScanResource
	var errs []error

	// Scan Classic Load Balancers
	classicLBs, err := scanClassicLoadBalancers(ctx, config)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to scan Classic Load Balancers: %w", err))
	}
	resources = append(resources, classicLBs...)

	// Scan Application and Network Load Balancers (ELBv2)
	elbv2LBs, err := scanElbv2LoadBalancers(ctx, config)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to scan ELBv2 Load Balancers: %w", err))
	}
	resources = append(resources, elbv2LBs...)

	if len(errs) > 0 {
		return resources, errors.Join(errs...)
	}
	return resources, nil
}

func scanClassicLoadBalancers(ctx context.Context, config aws.Config) ([]infra_sdk.ScanResource, error) {
	client := elasticloadbalancing.NewFromConfig(config)

	output, err := client.DescribeLoadBalancers(ctx, &elasticloadbalancing.DescribeLoadBalancersInput{})
	if err != nil {
		return nil, err
	}

	resources := make([]infra_sdk.ScanResource, 0, len(output.LoadBalancerDescriptions))
	for _, lb := range output.LoadBalancerDescriptions {
		if lb.LoadBalancerName == nil {
			continue
		}

		name := *lb.LoadBalancerName

		// Get tags for the load balancer
		tagOutput, err := client.DescribeTags(ctx, &elasticloadbalancing.DescribeTagsInput{
			LoadBalancerNames: []string{name},
		})
		tags := make(map[string]string)
		if err == nil && tagOutput != nil {
			for _, desc := range tagOutput.TagDescriptions {
				for _, tag := range desc.Tags {
					tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
				}
			}
		}

		dnsName := ""
		if lb.DNSName != nil {
			dnsName = *lb.DNSName
		}

		scheme := ""
		if lb.Scheme != nil {
			scheme = *lb.Scheme
		}

		// For classic load balancers, we'll use 'elb' as the platform
		// since they don't have a type field like v2 load balancers
		resources = append(resources, infra_sdk.ScanResource{
			UniqueId: name,
			Name:     name,
			Taxonomy: infra_sdk.ResourceTaxonomy{
				Category:    types.CategoryIngress,
				Platform:    "load-balancer",
				Subplatform: "elb",
			},
			Attributes: map[string]any{
				"dns_name":        dnsName,
				"scheme":          scheme,
				"vpc_id":          lb.VPCId,
				"created_time":    lb.CreatedTime,
				"security_groups": lb.SecurityGroups,
				"listeners":       lb.ListenerDescriptions,
				"instances":       lb.Instances,
				"tags":            tags,
			},
		})
	}

	return resources, nil
}

func scanElbv2LoadBalancers(ctx context.Context, config aws.Config) ([]infra_sdk.ScanResource, error) {
	client := elasticloadbalancingv2.NewFromConfig(config)

	var lbs []elbv2types.LoadBalancer
	var marker *string

	for {
		output, err := client.DescribeLoadBalancers(ctx, &elasticloadbalancingv2.DescribeLoadBalancersInput{
			Marker: marker,
		})
		if err != nil {
			return nil, err
		}

		lbs = append(lbs, output.LoadBalancers...)

		if output.NextMarker == nil {
			break
		}
		marker = output.NextMarker
	}

	resources := make([]infra_sdk.ScanResource, 0, len(lbs))
	for _, lb := range lbs {
		if lb.LoadBalancerArn == nil {
			continue
		}

		name := aws.ToString(lb.LoadBalancerName)

		// Get tags for the load balancer
		tagOutput, err := client.DescribeTags(ctx, &elasticloadbalancingv2.DescribeTagsInput{
			ResourceArns: []string{*lb.LoadBalancerArn},
		})
		tags := make(map[string]string)
		if err == nil && tagOutput != nil {
			for _, tagDesc := range tagOutput.TagDescriptions {
				for _, tag := range tagDesc.Tags {
					tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
				}
			}
		}

		// Get listeners for the load balancer
		listenerOutput, _ := client.DescribeListeners(ctx, &elasticloadbalancingv2.DescribeListenersInput{
			LoadBalancerArn: lb.LoadBalancerArn,
		})

		// Determine the subplatform based on the load balancer type
		subplatform := ""
		switch lb.Type {
		case elbv2types.LoadBalancerTypeEnumApplication:
			subplatform = "alb"
		case elbv2types.LoadBalancerTypeEnumNetwork:
			subplatform = "nlb"
		case elbv2types.LoadBalancerTypeEnumGateway:
			subplatform = "gwlb"
		}

		resources = append(resources, infra_sdk.ScanResource{
			UniqueId: *lb.LoadBalancerArn,
			Name:     name,
			Taxonomy: infra_sdk.ResourceTaxonomy{
				Category:    types.CategoryIngress,
				Platform:    "load-balancer",
				Subplatform: subplatform,
			},
			Attributes: map[string]any{
				"dns_name":           lb.DNSName,
				"scheme":             lb.Scheme,
				"vpc_id":             lb.VpcId,
				"created_time":       lb.CreatedTime,
				"security_groups":    lb.SecurityGroups,
				"ip_address_type":    lb.IpAddressType,
				"listeners":          listenerOutput.Listeners,
				"availability_zones": lb.AvailabilityZones,
				"type":               lb.Type,
				"tags":               tags,
			},
		})
	}

	return resources, nil
}
