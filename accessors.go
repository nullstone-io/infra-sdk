package infra_sdk

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"golang.org/x/oauth2"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

type Accessors struct {
	Aws func(provider types.Provider, providerConfig types.ProviderConfig) AwsAccessor
	Gcp func(provider types.Provider, providerConfig types.ProviderConfig) GcpAccessor
}

type AwsAccessor interface {
	NewConfig(region string) (*aws.Config, error)
	AwsAccountId() string
}

type GcpAccessor interface {
	GetTokenSource(ctx context.Context) (oauth2.TokenSource, error)
	GcpProjectId() string
}
