package gcp_project

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/bigquery"
	infra_sdk "github.com/nullstone-io/infra-sdk"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

var (
	_ infra_sdk.Coster      = Coster{}
	_ infra_sdk.CostScanner = Coster{}
)

type Coster struct {
	Accessor infra_sdk.GcpBillingAccessor
}

func (c Coster) ProviderType() string { return "gcp" }

// Capabilities describes the resource-level BigQuery billing export: any grouping in one query,
// every measure, charge category carried, and the billed resource (see GcpResourceColumn).
func Capabilities() infra_sdk.CostCapabilities {
	return infra_sdk.CostCapabilities{MaxGroupBy: 0, Metrics: SupportedMetrics(), HasChargeCategory: true, HasResource: true}
}

// MonthSource is always the billing export: GCP has a single source.
func (c Coster) MonthSource(ctx context.Context, month time.Time) (infra_sdk.CostSource, error) {
	return infra_sdk.CostSource{Name: infra_sdk.CostSourceBillingExport, Capabilities: Capabilities()}, nil
}

// ReferenceCoster is the coster itself: the billing export is the bill.
func (c Coster) ReferenceCoster() infra_sdk.Coster { return c }

// GetCosts runs the query and folds every result row into a CostResult. A resource-grain month
// is hundreds of thousands of rows; a consumer that lands rows should use ScanCosts instead.
func (c Coster) GetCosts(ctx context.Context, query infra_sdk.CostQuery) (*infra_sdk.CostResult, error) {
	groupBy := query.GroupBy.Unique()
	aggregator := NewCostResultAggregator()
	err := c.read(ctx, query, func(row map[string]bigquery.Value) error {
		if err := aggregator.AddRow(row, groupBy); err != nil {
			return fmt.Errorf("error aggregating result: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return aggregator.CostResult, nil
}

// ScanCosts runs the query and hands each result row to fn as it is read from BigQuery. Nothing
// is held beyond the row in flight: the query already sums by group keys and period, so each
// row is one line item.
func (c Coster) ScanCosts(ctx context.Context, query infra_sdk.CostQuery, fn func(infra_sdk.CostRow) error) error {
	groupBy := query.GroupBy.Unique()
	return c.read(ctx, query, func(row map[string]bigquery.Value) error {
		costRow, err := toCostRow(row, groupBy)
		if err != nil {
			return err
		}
		return fn(costRow)
	})
}

// read runs the query against the billing export and calls fn once per result row.
func (c Coster) read(ctx context.Context, query infra_sdk.CostQuery, fn func(row map[string]bigquery.Value) error) error {
	ts, err := c.Accessor.GetTokenSource(ctx)
	if err != nil {
		return fmt.Errorf("error resolving gcp credentials: %w", err)
	}

	client, err := bigquery.NewClient(ctx, c.Accessor.GcpProjectId(), option.WithTokenSource(ts))
	if err != nil {
		return fmt.Errorf("error creating bigquery client: %w", err)
	}
	defer client.Close()

	table := fmt.Sprintf("%s.%s", c.Accessor.BillingDataset(), c.Accessor.BillingTable())
	builder := &QueryBuilder{Table: table}
	built := builder.Build(query)

	q := client.Query(built.SQL)
	q.Parameters = built.Params

	it, err := q.Read(ctx)
	if err != nil {
		return fmt.Errorf("error querying bigquery billing export: %w", err)
	}
	return drain(it, fn)
}

// rowIterator is the part of bigquery.RowIterator the read loop uses.
type rowIterator interface {
	Next(dst interface{}) error
}

// drain reads it to the end, calling fn once per row.
func drain(it rowIterator, fn func(row map[string]bigquery.Value) error) error {
	for {
		var row map[string]bigquery.Value
		err := it.Next(&row)
		if errors.Is(err, iterator.Done) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("error reading bigquery result row: %w", err)
		}
		if err := fn(row); err != nil {
			return err
		}
	}
}
