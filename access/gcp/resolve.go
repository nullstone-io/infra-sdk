package gcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/impersonate"
	"google.golang.org/api/option"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

var (
	ErrMissingAuthTypeInGcpCredentials         = errors.New(`missing "auth_type" in gcp credentials`)
	ErrUnsupportedAuthTypeInGcpCredentials     = errors.New(`unsupported "auth_type" in gcp credentials`)
	ErrMissingEmailInServiceAccountOutput      = errors.New(`missing "email" in output for service account`)
	ErrMissingPrivateKeyInServiceAccountOutput = errors.New(`missing "private_key" in output for service account`)
)

func ResolveTokenSource(ctx context.Context, assumer Assumer, provider types.Provider) (oauth2.TokenSource, error) {
	gcpCreds := types.GcpCredentials{}
	if err := json.Unmarshal(provider.Credentials, &gcpCreds); err != nil {
		return nil, fmt.Errorf("invalid gcp credentials: %s", err)
	}

	switch gcpCreds.AuthType {
	case "":
		return nil, ErrMissingAuthTypeInGcpCredentials
	case types.GcpAuthTypeServiceAccount:
		return ResolveKeyTokenSource(ctx, gcpCreds.ServiceAccountKey, "https://www.googleapis.com/auth/cloud-platform")
	case types.GcpAuthTypeServiceAccountImpersonation:
		return ResolveImpersonationTokenSource(ctx, assumer, gcpCreds.Impersonation, "https://www.googleapis.com/auth/cloud-platform")
	default:
		return nil, ErrUnsupportedAuthTypeInGcpCredentials
	}
}

func ResolveKeyTokenSource(ctx context.Context, a types.GcpServiceAccountKey, scopes ...string) (oauth2.TokenSource, error) {
	if a.PrivateKey == "" {
		return nil, ErrMissingPrivateKeyInServiceAccountOutput
	}

	decoded, err := base64.StdEncoding.DecodeString(a.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("service account private key is not base64-encoded: %w", err)
	}
	cfg, err := google.JWTConfigFromJSON(decoded, scopes...)
	if err != nil {
		return nil, fmt.Errorf("unable to read service account credentials json file: %w", err)
	}
	return cfg.TokenSource(ctx), nil
}

func ResolveImpersonationTokenSource(ctx context.Context, assumer Assumer, a types.GcpServiceAccountImpersonation, scopes ...string) (oauth2.TokenSource, error) {
	if a.ServiceAccountEmail == "" {
		return nil, ErrMissingEmailInServiceAccountOutput
	}

	// Create a token source that can impersonate the target service account
	return impersonate.CredentialsTokenSource(ctx, impersonate.CredentialsConfig{
		TargetPrincipal: a.ServiceAccountEmail,
		Scopes:          scopes,
	}, option.WithTokenSource(assumer.Credentials.TokenSource))
}
