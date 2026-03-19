package builtin

import (
	infra_sdk "github.com/nullstone-io/infra-sdk"
	aws_account "github.com/nullstone-io/infra-sdk/builtin/aws/aws-account"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

type CosterCreator struct {
	Accessors infra_sdk.Accessors
}

func (s CosterCreator) NewMultiCoster(providers []types.Provider) (infra_sdk.MultiCoster, error) {
	mc := infra_sdk.MultiCoster{Costers: []infra_sdk.Coster{}}
	for _, cur := range providers {
		coster, err := s.DiscoverCoster(cur)
		if err != nil {
			return mc, err
		}
		mc.Costers = append(mc.Costers, coster)
	}
	return mc, nil
}

func (s CosterCreator) DiscoverCoster(provider types.Provider) (infra_sdk.Coster, error) {
	switch provider.ProviderType {
	case "aws":
		return aws_account.Coster{Accessor: s.Accessors.Aws}, nil
	case "gcp":
		// TODO: Implement GCP
		//return gcp_account.Coster{Accessor: s.Accessors.Gcp}, nil
	case "azure":
		// TODO: Implement Azure
	}
	return nil, nil
}
