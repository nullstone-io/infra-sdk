package gcp_project

import (
	"context"
	"errors"
	"fmt"

	"cloud.google.com/go/bigquery"
	infra_sdk "github.com/nullstone-io/infra-sdk"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

type Coster struct {
	Accessor infra_sdk.GcpBillingAccessor
}

func (c Coster) GetCosts(ctx context.Context, query infra_sdk.CostQuery) (*infra_sdk.CostResult, error) {
	ts, err := c.Accessor.GetTokenSource(ctx)
	if err != nil {
		return nil, fmt.Errorf("error resolving gcp credentials: %w", err)
	}

	client, err := bigquery.NewClient(ctx, c.Accessor.GcpProjectId(), option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("error creating bigquery client: %w", err)
	}
	defer client.Close()

	groupBy := query.GroupBy.Unique()
	table := fmt.Sprintf("%s.%s", c.Accessor.BillingDataset(), c.Accessor.BillingTable())
	builder := &QueryBuilder{Table: table}
	built := builder.Build(query)

	q := client.Query(built.SQL)
	q.Parameters = built.Params

	it, err := q.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("error querying bigquery billing export: %w", err)
	}

	aggregator := NewCostResultAggregator()
	for {
		var row map[string]bigquery.Value
		err := it.Next(&row)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading bigquery result row: %w", err)
		}
		if err := aggregator.AddRow(row, groupBy); err != nil {
			return nil, fmt.Errorf("error aggregating result: %w", err)
		}
	}

	return aggregator.CostResult, nil
}
