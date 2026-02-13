package infra_sdk

import (
	"context"
	"errors"
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
	result := make([]types.Secret, 0)
	var errs []error
	for _, manager := range m.Managers {
		cur, err := manager.List(ctx, location)
		if err != nil {
			errs = append(errs, err)
		} else {
			result = append(result, cur...)
		}
	}
	if len(errs) > 0 {
		return result, errors.Join(errs...)
	}
	return result, nil
}

func (m MultiSecretManager) Create(ctx context.Context, identity types.SecretIdentity, value string) (*types.Secret, error) {
	manager, err := m.findManager(identity)
	if err != nil {
		return nil, err
	}
	return manager.Create(ctx, identity, value)
}

func (m MultiSecretManager) Update(ctx context.Context, identity types.SecretIdentity, value string) (*types.Secret, error) {
	manager, err := m.findManager(identity)
	if err != nil {
		return nil, err
	}
	return manager.Update(ctx, identity, value)
}

func (m MultiSecretManager) findManager(identity types.SecretIdentity) (SecretManager, error) {
	if len(m.Managers) == 0 {
		return nil, fmt.Errorf("no cloud platforms are configured")
	}

	if identity.Platform == "" {
		if len(m.Managers) > 1 {
			return nil, fmt.Errorf("multiple cloud platforms are configured, you must specify a cloud platform")
		}
		if len(m.Managers) == 1 {
			for _, cur := range m.Managers {
				return cur, nil
			}
		}
	}

	manager, ok := m.Managers[identity.Platform]
	if !ok {
		return nil, fmt.Errorf("secret manager does not support %q platform", identity.Platform)
	}
	return manager, nil
}

var ErrSecretAlreadyExists = errors.New("secret already exists")
var ErrDoesNotExist = errors.New("secret does not exist")
