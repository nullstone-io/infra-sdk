package aws

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

type Assumer struct {
	AccessKeyID     string
	SecretAccessKey string
}

func (a Assumer) AwsConfig() aws.Config {
	return aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider(a.AccessKeyID, a.SecretAccessKey, ""),
	}
}
