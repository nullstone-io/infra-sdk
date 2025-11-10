package builtin

import (
	"context"

	infra_sdk "github.com/nullstone-io/infra-sdk"
	"github.com/nullstone-io/infra-sdk/access/aws"
	aws_account "github.com/nullstone-io/infra-sdk/builtin/aws/aws-account"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

type GetProviderFunc func(ctx context.Context, orgName string, providerName string) (*types.Provider, error)

type ScannerCreator struct {
	AwsAssumer aws.Assumer
}

func (s ScannerCreator) NewScanner(ctx context.Context, getProviderFn GetProviderFunc, orgName string, providerConfig types.ProviderConfig) (infra_sdk.Scanner, error) {
	if providerConfig.Aws != nil && providerConfig.Aws.ProviderName != "" {
		provider, err := getProviderFn(ctx, orgName, providerConfig.Aws.ProviderName)
		if err != nil {
			return nil, err
		} else if provider != nil {
			return aws_account.Scanner{
				Assumer:        s.AwsAssumer,
				Provider:       *provider,
				ProviderConfig: providerConfig,
			}, nil
		}
	}
	if providerConfig.Gcp != nil {
		// TODO: Implement GCP
	}
	return nil, nil
}
