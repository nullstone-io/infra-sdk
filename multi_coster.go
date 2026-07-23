package infra_sdk

import (
	"context"
	"errors"
	"sync"
)

// MultiCoster runs cost queries against multiple costers
// Results are combined into a single CostResult
type MultiCoster struct {
	Costers []Coster
}

type costerResult struct {
	costResult *CostResult
	provider   string
	err        error
}

func (c *MultiCoster) GetCosts(ctx context.Context, query CostQuery) (*CostResult, error) {
	results := make(chan costerResult, len(c.Costers))
	var wg sync.WaitGroup
	// Run each coster concurrently
	for _, cur := range c.Costers {
		wg.Add(1)
		go func(coster Coster) {
			defer wg.Done()
			provider := ""
			if typed, ok := coster.(ProviderTyped); ok {
				provider = typed.ProviderType()
			}
			costResult, err := coster.GetCosts(ctx, query)
			results <- costerResult{
				costResult: costResult,
				provider:   provider,
				err:        err,
			}
		}(cur)
	}

	// Wait for all costers to finish
	go func() {
		wg.Wait()
		close(results)
	}()

	// Combine results real-time
	var errs []error
	combinedResult := NewCostResult()
	for res := range results {
		if res.err != nil {
			errs = append(errs, res.err)
			continue
		}
		for _, series := range res.costResult.Series {
			combinedResult.MergeSeries(series, res.provider)
		}
	}

	return combinedResult, errors.Join(errs...)
}
