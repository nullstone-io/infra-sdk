package bootstrap

import (
	"encoding/json"
	"os"

	"github.com/nullstone-io/infra-sdk/access/aws"
	aws_account "github.com/nullstone-io/infra-sdk/builtin/aws/aws-account"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

func NewCoster() (aws_account.Coster, error) {
	credentials, _ := json.Marshal(types.AwsCredentials{
		AuthType:             types.AwsAuthTypeAssumeRole,
		AssumeRoleName:       os.Getenv("AWS_ASSUME_ROLE_NAME"),
		AssumeRoleExternalId: os.Getenv("AWS_ASSUME_ROLE_EXTERNAL_ID"),
	})

	return aws_account.Coster{
		Assumer: aws.Assumer{
			AccessKeyID:     os.Getenv("ASSUMER_AWS_ACCESS_KEY_ID"),
			SecretAccessKey: os.Getenv("ASSUMER_AWS_SECRET_ACCESS_KEY"),
		},
		Provider: types.Provider{
			ProviderType: "aws",
			ProviderId:   os.Getenv("AWS_ACCOUNT_ID"),
			Credentials:  credentials,
		},
	}, nil
}
