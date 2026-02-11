package aws

import (
	"context"

	infra_sdk "github.com/nullstone-io/infra-sdk"
	"github.com/nullstone-io/infra-sdk/access/aws"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

var (
	_ infra_sdk.SecretManager = SecretManager{}
)

type SecretManager struct {
	Assumer  aws.Assumer
	Provider types.Provider
}

func (s SecretManager) List(ctx context.Context, location types.SecretLocation) ([]types.Secret, error) {
	//TODO implement me
	panic("implement me")
}

func (s SecretManager) Create(ctx context.Context, identity types.SecretIdentity, value string) (*types.Secret, error) {
	//TODO implement me
	panic("implement me")
}

func (s SecretManager) Update(ctx context.Context, identity types.SecretIdentity, value string) (*types.Secret, error) {
	//TODO implement me
	panic("implement me")
}
