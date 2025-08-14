package aws_account

import (
	"context"
	"errors"
	"fmt"
	"github.com/nullstone-io/infra-sdk"
	"github.com/nullstone-io/infra-sdk/access/aws"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

var (
	AllScanners = []ResourceScanner{
		ScanNetworks,
		ScanEcsClusters,
		ScanRdsDatabases,
	}
)

type Scanner struct {
	Assumer        aws.Assumer
	Provider       types.Provider
	ProviderConfig types.ProviderConfig
}

func (s Scanner) Scan(ctx context.Context) ([]infra_sdk.ScanResource, error) {
	awsConfig, err := aws.ResolveConfig(s.Assumer.AwsConfig(), s.Provider, s.ProviderConfig)
	if err != nil {
		return nil, fmt.Errorf("error resolving aws config: %w", err)
	}

	tracker := NewResourceScanTracker()
	for _, scanner := range AllScanners {
		tracker.Scan(ctx, awsConfig, scanner)
	}
	tracker.Wait()
	if len(tracker.Errors) > 0 {
		err = errors.Join(tracker.Errors...)
	}
	return tracker.Resources, err
}
