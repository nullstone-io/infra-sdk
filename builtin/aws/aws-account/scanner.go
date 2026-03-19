package aws_account

import (
	"context"
	"errors"
	"fmt"

	"github.com/nullstone-io/infra-sdk"
)

var (
	AllScanners = []ResourceScanner{
		// domain/subdomain
		ScanRoute53,

		// network
		ScanNetworks,

		// cluster
		ScanEcsClusters,

		// ingress
		ScanLoadBalancers,
		ScanApiGateways,
		ScanCdns,

		// datastore
		ScanS3Buckets,
		ScanEfsFileSystems,
		ScanRdsDatabases,
		ScanElastiCacheClusters,
		ScanMskClusters,
		ScanMqBrokers,
		ScanSqsQueues,
		ScanSnsTopics,
		ScanOpenSearchDomains,
	}
)

type Scanner struct {
	Accessor infra_sdk.AwsAccessor
}

func (s Scanner) Scan(ctx context.Context) ([]infra_sdk.ScanResource, error) {
	if s.Accessor == nil {
		return nil, nil
	}
	awsConfig, err := s.Accessor.NewConfig("")
	if err != nil {
		return nil, fmt.Errorf("error resolving aws config: %w", err)
	}
	if awsConfig == nil {
		return nil, nil
	}

	tracker := NewResourceScanTracker()
	for _, scanner := range AllScanners {
		tracker.Scan(ctx, *awsConfig, scanner)
	}
	tracker.Wait()
	if len(tracker.Errors) > 0 {
		err = errors.Join(tracker.Errors...)
	}
	return tracker.Resources, err
}
