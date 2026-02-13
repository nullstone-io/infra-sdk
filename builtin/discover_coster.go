package builtin

import (
	infra_sdk "github.com/nullstone-io/infra-sdk"
	aws_access "github.com/nullstone-io/infra-sdk/access/aws"
	gcp_access "github.com/nullstone-io/infra-sdk/access/gcp"
	aws_account "github.com/nullstone-io/infra-sdk/builtin/aws/aws-account"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

type CosterCreator struct {
	AwsAssumer aws_access.Assumer
	GcpAssumer gcp_access.Assumer
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
		return aws_account.Coster{
			Assumer:  s.AwsAssumer,
			Provider: provider,
		}, nil
	case "gcp":
		// TODO: Implement GCP
	}
	return nil, nil
}
