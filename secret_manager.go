package infra_sdk

import (
	"context"
	"fmt"

	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

type SecretManager interface {
	List(ctx context.Context, location types.SecretLocation) ([]types.Secret, error)
	Create(ctx context.Context, identity types.SecretIdentity, value string) (*types.Secret, error)
	Update(ctx context.Context, identity types.SecretIdentity, value string) (*types.Secret, error)
}

var (
	_ SecretManager = MultiSecretManager{}
)

type MultiSecretManager struct {
	Managers map[string]SecretManager
}

func (m MultiSecretManager) List(ctx context.Context, location types.SecretLocation) ([]types.Secret, error) {
	manager, ok := m.Managers[location.Platform]
	if !ok {
		return nil, fmt.Errorf("secret manager does not support %q platform", location.Platform)
	}
	return manager.List(ctx, location)
}

func (m MultiSecretManager) Create(ctx context.Context, identity types.SecretIdentity, value string) (*types.Secret, error) {
	manager, ok := m.Managers[identity.Platform]
	if !ok {
		return nil, fmt.Errorf("secret manager does not support %q platform", identity.Platform)
	}
	return manager.Create(ctx, identity, value)
}

func (m MultiSecretManager) Update(ctx context.Context, identity types.SecretIdentity, value string) (*types.Secret, error) {
	manager, ok := m.Managers[identity.Platform]
	if !ok {
		return nil, fmt.Errorf("secret manager does not support %q platform", identity.Platform)
	}
	return manager.Update(ctx, identity, value)
}
