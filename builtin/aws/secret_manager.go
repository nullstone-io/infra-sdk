package aws

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	sm_types "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	infra_sdk "github.com/nullstone-io/infra-sdk"
	"github.com/nullstone-io/infra-sdk/access/aws"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

var (
	_ infra_sdk.SecretManager = SecretManager{}
)

type SecretManager struct {
	Assumer        aws.Assumer
	Provider       types.Provider
	ProviderConfig *types.AwsProviderConfig
}

func (s SecretManager) List(ctx context.Context, location types.SecretLocation) ([]types.Secret, error) {
	if s.ProviderConfig == nil || s.ProviderConfig.ProviderName == "" {
		return nil, nil
	}
	client, err := s.smClient(location.AwsRegion)
	if err != nil {
		return nil, err
	}

	input := &secretsmanager.ListSecretsInput{}
	out, err := client.ListSecrets(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("error listing secrets: %w", err)
	}
	result := make([]types.Secret, 0)
	for _, cur := range out.SecretList {
		result = append(result, types.Secret{
			Identity: s.secretIdentityFromAws(cur.ARN, cur.Name, cur.PrimaryRegion),
			Metadata: map[string]any{
				"description": cur.Description,
				"tags":        cur.Tags,
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
	client, err := s.smClient(identity.AwsRegion)
	if err != nil {
		return nil, err
	}

	out, err := client.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name:         &identity.Name,
		SecretString: &value,
	})
	if err != nil {
		var ree *sm_types.ResourceExistsException
		if errors.As(err, &ree) {
			return nil, infra_sdk.ErrSecretAlreadyExists
		}
		return nil, fmt.Errorf("error creating secret: %w", err)
	}

	return &types.Secret{
		Identity: s.secretIdentityFromAws(out.ARN, out.Name, &identity.AwsRegion),
		Metadata: nil,
		Value:    "",
		Redacted: false,
	}, nil
}

func (s SecretManager) Update(ctx context.Context, identity types.SecretIdentity, value string) (*types.Secret, error) {
	if s.ProviderConfig == nil || s.ProviderConfig.ProviderName == "" {
		return nil, nil
	}
	client, err := s.smClient(identity.AwsRegion)
	if err != nil {
		return nil, err
	}

	out, err := client.UpdateSecret(ctx, &secretsmanager.UpdateSecretInput{
		SecretId:     &identity.Name,
		SecretString: &value,
	})
	if err != nil {
		var rnfe *sm_types.ResourceNotFoundException
		if errors.As(err, &rnfe) {
			return nil, infra_sdk.ErrDoesNotExist
		}
		return nil, fmt.Errorf("error updating secret: %w", err)
	}

	return &types.Secret{
		Identity: s.secretIdentityFromAws(out.ARN, out.Name, &identity.AwsRegion),
		Metadata: nil,
		Value:    "",
		Redacted: false,
	}, nil
}

func (s SecretManager) smClient(region string) (*secretsmanager.Client, error) {
	awsConfig, err := aws.ResolveConfig(s.Assumer.AwsConfig(), s.Provider, s.ProviderConfig, region)
	if err != nil {
		return nil, fmt.Errorf("error resolving aws config: %w", err)
	}
	return secretsmanager.NewFromConfig(awsConfig), nil
}

func (s SecretManager) secretIdentityFromAws(secretArn *string, name *string, primaryRegion *string) types.SecretIdentity {
	identity := types.SecretIdentity{
		Name: unptr(name),
		SecretLocation: types.SecretLocation{
			Platform:     types.SecretLocationPlatformAws,
			AwsRegion:    unptr(primaryRegion),
			AwsAccountId: s.Provider.ProviderId,
		},
	}
	if a, err := arn.Parse(unptr(secretArn)); err == nil {
		identity.AwsRegion = a.Region
		identity.AwsAccountId = a.AccountID
		identity.Name = a.Resource
	}
	return identity
}

func unptr[T any](t *T) T {
	if t != nil {
		return *t
	}
	var x T
	return x
}
