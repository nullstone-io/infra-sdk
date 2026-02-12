package gcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	infra_sdk "github.com/nullstone-io/infra-sdk"
	"github.com/nullstone-io/infra-sdk/access/gcp"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

var (
	_ infra_sdk.SecretManager = SecretManager{}
)

type SecretManager struct {
	Assumer        gcp.Assumer
	Provider       types.Provider
	ProviderConfig *types.GcpProviderConfig
}

func (s SecretManager) List(ctx context.Context, location types.SecretLocation) ([]types.Secret, error) {
	if s.ProviderConfig == nil || s.ProviderConfig.ProviderName == "" {
		return nil, nil
	}
	client, err := s.smClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	parent := fmt.Sprintf("projects/%s", location.GcpProjectId)
	it := client.ListSecrets(ctx, &secretmanagerpb.ListSecretsRequest{Parent: parent})

	result := make([]types.Secret, 0)
	for {
		secret, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error listing secrets: %w", err)
		}
		result = append(result, types.Secret{
			Identity: s.secretIdentityFromGcp(secret.Name),
			Metadata: map[string]any{
				"labels": secret.Labels,
			},
			Value:    "",
			Redacted: true,
		})
	}
	return result, nil
}

func (s SecretManager) Create(ctx context.Context, identity types.SecretIdentity, value string) (*types.Secret, error) {
	if s.ProviderConfig == nil || s.ProviderConfig.ProviderName == "" {
		return nil, nil
	}
	client, err := s.smClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	secret, err := client.CreateSecret(ctx, &secretmanagerpb.CreateSecretRequest{
		Parent:   fmt.Sprintf("projects/%s", identity.GcpProjectId),
		SecretId: identity.Name,
		Secret: &secretmanagerpb.Secret{
			Replication: &secretmanagerpb.Replication{
				Replication: &secretmanagerpb.Replication_Automatic_{
					Automatic: &secretmanagerpb.Replication_Automatic{},
				},
			},
		},
	})
	if err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return nil, infra_sdk.ErrSecretAlreadyExists
		}
		return nil, fmt.Errorf("error creating secret: %w", err)
	}

	_, err = client.AddSecretVersion(ctx, &secretmanagerpb.AddSecretVersionRequest{
		Parent: secret.Name,
		Payload: &secretmanagerpb.SecretPayload{
			Data: []byte(value),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("error adding secret version: %w", err)
	}

	return &types.Secret{
		Identity: identity,
		Metadata: nil,
		Value:    "",
		Redacted: false,
	}, nil
}

func (s SecretManager) Update(ctx context.Context, identity types.SecretIdentity, value string) (*types.Secret, error) {
	if s.ProviderConfig == nil || s.ProviderConfig.ProviderName == "" {
		return nil, nil
	}
	client, err := s.smClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	_, err = client.AddSecretVersion(ctx, &secretmanagerpb.AddSecretVersionRequest{
		Parent: fmt.Sprintf("projects/%s", identity.GcpProjectId),
		Payload: &secretmanagerpb.SecretPayload{
			Data: []byte(value),
		},
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, infra_sdk.ErrDoesNotExist
		}
		return nil, fmt.Errorf("error updating secret: %w", err)
	}

	return &types.Secret{
		Identity: identity,
		Metadata: nil,
		Value:    "",
		Redacted: false,
	}, nil
}

func (s SecretManager) smClient(ctx context.Context) (*secretmanager.Client, error) {
	tokenSource, err := gcp.ResolveTokenSource(ctx, s.Assumer, s.Provider)
	if err != nil {
		return nil, fmt.Errorf("error resolving gcp credentials: %w", err)
	}

	client, err := secretmanager.NewClient(ctx, option.WithTokenSource(tokenSource))
	if err != nil {
		return nil, fmt.Errorf("error creating gcp secret manager client: %w", err)
	}
	return client, nil
}

func (s SecretManager) secretIdentityFromGcp(secretName string) types.SecretIdentity {
	identity := types.SecretIdentity{
		SecretLocation: types.SecretLocation{
			Platform:     types.SecretLocationPlatformGcp,
			GcpProjectId: s.Provider.ProviderId,
		},
	}
	// secretName is one of:
	// - "projects/{project}/secrets/{secretId}"
	// - "projects/{project}/locations/{location}/secrets/{secretId}"
	parts := strings.Split(secretName, "/")
	if len(parts) == 4 {
		identity.GcpProjectId = parts[1]
		identity.Name = parts[len(parts)-1]
	} else if len(parts) == 6 {
		identity.GcpProjectId = parts[1]
		identity.Name = parts[len(parts)-1]
	}
	return identity
}
