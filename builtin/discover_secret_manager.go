package builtin

import (
	infra_sdk "github.com/nullstone-io/infra-sdk"
	"github.com/nullstone-io/infra-sdk/builtin/aws"
	"github.com/nullstone-io/infra-sdk/builtin/gcp"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

type SecretManagerCreator struct {
	Accessors infra_sdk.Accessors
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
		return aws.SecretManager{Accessor: c.Accessors.Aws(provider, providerConfig)}, nil
	case "gcp":
		return gcp.SecretManager{Accessor: c.Accessors.Gcp(provider, providerConfig)}, nil
	case "azure":
		// TODO: Implement Azure
	}
	return nil, nil
}
