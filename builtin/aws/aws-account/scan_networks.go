package aws_account

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	infra_sdk "github.com/nullstone-io/infra-sdk"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

func ScanNetworks(ctx context.Context, config aws.Config) ([]infra_sdk.ScanResource, error) {
	ec2Client := ec2.NewFromConfig(config)
	output, err := ec2Client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{})
	if err != nil {
		return nil, err
	}

	resources := make([]infra_sdk.ScanResource, 0)
	for _, vpc := range output.Vpcs {
		name := ""
		// Extract Name tag if it exists
		for _, tag := range vpc.Tags {
			if *tag.Key == "Name" {
				name = *tag.Value
				break
			}
		}
		if name == "" {
			name = *vpc.VpcId
		}

		resources = append(resources, infra_sdk.ScanResource{
			UniqueId: *vpc.VpcId,
			Name:     name,
			Taxonomy: infra_sdk.ResourceTaxonomy{
				Category:    types.CategoryNetwork,
				Subcategory: "",
				Platform:    "vpc",
				Provider:    "aws",
			},
			ServiceName:         "VPC",
			ServiceResourceName: "Network",
			Attributes: map[string]any{
				"cidr_block":       vpc.CidrBlock,
				"is_default":       vpc.IsDefault,
				"state":            vpc.State,
				"instance_tenancy": vpc.InstanceTenancy,
				"tags":             vpc.Tags,
			},
		})
	}
	return resources, nil
}
