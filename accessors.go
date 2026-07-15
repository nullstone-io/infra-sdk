package infra_sdk

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"golang.org/x/oauth2"
)

type AwsAccessor interface {
	NewConfig(region string) (*aws.Config, error)
	AwsAccountId() string
}

type GcpAccessor interface {
	GetTokenSource(ctx context.Context) (oauth2.TokenSource, error)
	GcpProjectId() string
}

type GcpBillingAccessor interface {
	GcpAccessor
	BillingDataset() string
	BillingTable() string
}
