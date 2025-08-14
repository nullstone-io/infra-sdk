package aws_account

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	infra_sdk "github.com/nullstone-io/infra-sdk"
	"sync"
)

type ResourceScanner func(ctx context.Context, config aws.Config) ([]infra_sdk.ScanResource, error)

func NewResourceScanTracker() *ResourceScanTracker {
	return &ResourceScanTracker{
		Resources: []infra_sdk.ScanResource{},
		Errors:    []error{},
	}
}

type ResourceScanTracker struct {
	Resources []infra_sdk.ScanResource
	Errors    []error

	mu sync.Mutex
	wg sync.WaitGroup
}

func (r *ResourceScanTracker) Scan(ctx context.Context, config aws.Config, rs ResourceScanner) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()

		resources, err := rs(ctx, config)

		r.mu.Lock()
		defer r.mu.Unlock()
		if err != nil {
			r.Errors = append(r.Errors, err)
		}
		r.Resources = append(r.Resources, resources...)
	}()
}

func (r *ResourceScanTracker) Wait() {
	r.wg.Wait()
}
