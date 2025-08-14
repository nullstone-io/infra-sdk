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
	scanner := MultiScanner{Scanners: make([]infra_sdk.Scanner, 0)}
	if providerConfig.Aws != nil {
		provider, err := getProviderFn(ctx, orgName, providerConfig.Aws.ProviderName)
		if err != nil {
			return nil, err
		}
		scanner.Scanners = append(scanner.Scanners, aws_account.Scanner{
			Assumer:        s.AwsAssumer,
			Provider:       *provider,
			ProviderConfig: providerConfig,
		})
	}
	if providerConfig.Gcp != nil {
		// TODO: Implement GCP
	}
	return scanner, nil
}

var (
	_ infra_sdk.Scanner = MultiScanner{}
)

type MultiScanner struct {
	Scanners []infra_sdk.Scanner
}

func (s MultiScanner) Scan(ctx context.Context) ([]infra_sdk.ScanResource, error) {
	for _, scanner := range s.Scanners {
		resources, err := scanner.Scan(ctx)
		if err != nil {
			return nil, err
		}
		return resources, nil
	}
	return nil, nil
}
