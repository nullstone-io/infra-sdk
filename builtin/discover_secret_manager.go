package builtin

import (
	infra_sdk "github.com/nullstone-io/infra-sdk"
	aws_access "github.com/nullstone-io/infra-sdk/access/aws"
	gcp_access "github.com/nullstone-io/infra-sdk/access/gcp"
	"github.com/nullstone-io/infra-sdk/builtin/aws"
	"github.com/nullstone-io/infra-sdk/builtin/gcp"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

type SecretManagerCreator struct {
	AwsAssumer aws_access.Assumer
	GcpAssumer gcp_access.Assumer
}

func (c SecretManagerCreator) NewSecretManager(providers []types.Provider, providerConfig types.ProviderConfig) (infra_sdk.MultiSecretManager, error) {
	mc := infra_sdk.MultiSecretManager{Managers: map[string]infra_sdk.SecretManager{}}
	for _, cur := range providers {
		manager, err := c.DiscoverSecretManager(cur, providerConfig)
		if err != nil {
			return mc, err
		}
		mc.Managers[cur.ProviderType] = manager
	}
	return mc, nil
}

func (c SecretManagerCreator) DiscoverSecretManager(provider types.Provider, providerConfig types.ProviderConfig) (infra_sdk.SecretManager, error) {
	switch provider.ProviderType {
	case "aws":
		return aws.SecretManager{
			Assumer:        c.AwsAssumer,
			Provider:       provider,
			ProviderConfig: providerConfig.Aws,
		}, nil
	case "gcp":
		return gcp.SecretManager{
			Assumer:        c.GcpAssumer,
			Provider:       provider,
			ProviderConfig: providerConfig.Gcp,
		}, nil
	}
	return nil, nil
}
