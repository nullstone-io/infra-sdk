package builtin

import (
	"context"

	infra_sdk "github.com/nullstone-io/infra-sdk"
	aws_account "github.com/nullstone-io/infra-sdk/builtin/aws/aws-account"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

type GetProviderFunc func(ctx context.Context, orgName string, providerName string) (*types.Provider, error)

type ScannerCreator struct {
	Accessors infra_sdk.Accessors
}

func (s ScannerCreator) NewScanner(provider types.Provider, providerConfig types.ProviderConfig) (infra_sdk.Scanner, error) {
	switch provider.ProviderType {
	case "aws":
		return aws_account.Scanner{Accessor: s.Accessors.Aws(provider, providerConfig)}, nil
	case "gcp":
		// TODO: Implement GCP
		//return gcp_account.Scanner{Accessor: s.Accessors.Gcp(provider, providerConfig)}, nil
	case "azure":
		// TODO: Implement Azure
		//return azure_account.Scanner{Accessor: s.Accessors.Gcp(provider, providerConfig)}, nil
	}
	return nil, nil
}
