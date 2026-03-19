package bootstrap

import (
	"os"

	aws2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	infra_sdk "github.com/nullstone-io/infra-sdk"
	aws_account "github.com/nullstone-io/infra-sdk/builtin/aws/aws-account"
)

func NewCoster() (aws_account.Coster, error) {
	return aws_account.Coster{
		Accessor: AwsAccessor{
			AccessKeyID:     os.Getenv("BOOTSTRAP_AWS_ACCESS_KEY_ID"),
			SecretAccessKey: os.Getenv("BOOTSTRAP_AWS_SECRET_ACCESS_KEY"),
			AccountId:       os.Getenv("BOOTSTRAP_AWS_ACCOUNT_ID"),
		},
	}, nil
}

var _ infra_sdk.AwsAccessor = AwsAccessor{}

type AwsAccessor struct {
	AccessKeyID     string
	SecretAccessKey string
	AccountId       string
}

func (a AwsAccessor) NewConfig(region string) (*aws2.Config, error) {
	return &aws2.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(a.AccessKeyID, a.SecretAccessKey, ""),
	}, nil
}

func (a AwsAccessor) AwsAccountId() string {
	return a.AccountId
}
