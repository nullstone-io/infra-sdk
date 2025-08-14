package aws

import (
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
	"time"
)

var (
	assumeRoleSessionName = "nullstone-executor"
	assumeRoleDuration    = time.Hour
)

func ResolveConfig(assumerAwsConfig aws.Config, provider types.Provider, cfg types.ProviderConfig) (aws.Config, error) {
	region := "us-east-1"
	if cfg.Aws != nil && cfg.Aws.Region != "" {
		region = cfg.Aws.Region
	}

	awsConfig := aws.Config{Region: region}
	creds, err := ResolveCredentials(assumerAwsConfig, provider)
	if err != nil {
		return awsConfig, err
	}
	awsConfig.Credentials = creds
	return awsConfig, nil

}

func ResolveCredentials(assumerAwsConfig aws.Config, provider types.Provider) (aws.CredentialsProvider, error) {
	awsCreds := types.AwsCredentials{}
	if err := json.Unmarshal(provider.Credentials, &awsCreds); err != nil {
		return nil, fmt.Errorf("invalid aws credentials: %s", err)
	}

	stsClient := sts.NewFromConfig(assumerAwsConfig)
	if awsCreds.AuthType == types.AwsAuthTypeAssumeRole {
		roleArn := fmt.Sprintf("arn:aws:iam::%s:role/%s", provider.ProviderId, awsCreds.AssumeRoleName)
		creds := stscreds.NewAssumeRoleProvider(stsClient, roleArn, func(o *stscreds.AssumeRoleOptions) {
			o.RoleSessionName = assumeRoleSessionName
			o.Duration = assumeRoleDuration
			o.ExternalID = &awsCreds.AssumeRoleExternalId
		})
		return creds, nil
	}

	creds := credentials.NewStaticCredentialsProvider(awsCreds.AccessKeyId, awsCreds.SecretAccessKey, "")
	return creds, nil
}
