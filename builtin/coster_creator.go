package builtin

import (
	"context"

	infra_sdk "github.com/nullstone-io/infra-sdk"
	"github.com/nullstone-io/infra-sdk/access/aws"
	aws_account "github.com/nullstone-io/infra-sdk/builtin/aws/aws-account"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

type CosterCreator struct {
	AwsAssumer aws.Assumer
}

func (s CosterCreator) NewCoster(ctx context.Context, getProviderFn GetProviderFunc, orgName string, providerConfig types.ProviderConfig) (infra_sdk.Coster, error) {
	if providerConfig.Aws != nil && providerConfig.Aws.ProviderName != "" {
		provider, err := getProviderFn(ctx, orgName, providerConfig.Aws.ProviderName)
		if err != nil {
			return nil, err
		} else if provider != nil {
			return aws_account.Coster{
				Assumer:  s.AwsAssumer,
				Provider: *provider,
			}, nil
		}
	}
	if providerConfig.Gcp != nil {
		// TODO: Implement GCP
	}
	return nil, nil
}
