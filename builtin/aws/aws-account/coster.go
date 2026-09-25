package aws_account

import (
	"context"
	"fmt"
	"time"

	infra_sdk "github.com/nullstone-io/infra-sdk"
	aws_data_export "github.com/nullstone-io/infra-sdk/builtin/aws/aws-data-export"
)

// Coster reports costs for an AWS account. Cost Explorer is always available and answers every
// month; a FOCUS 1.2 Data Export, when configured, answers the months it has delivered with the
// full set of measures and resource-level detail. Consumers ask one coster and never see the
// split: GetCosts routes each billing month to the richest source that covers it, and
// MonthSource tells them which source that was and what it could report.
//
// Cost Explorer remains the reference for reconciliation (ReferenceCoster) because it is the
// provider's own bill; an exported month is checked against it, not the other way round.
type Coster struct {
	Accessor infra_sdk.AwsAccessor
	// Export is the location of a FOCUS 1.2 Data Export. Nil means Cost Explorer only, which is
	// what the live query path wants: reading an export streams gzip CSV from S3 per request.
	Export *aws_data_export.Location
	// ExportRegion is the export bucket's region. Empty means us-east-1.
	ExportRegion string

	// costExplorer and export stand in for the real sources in tests.
	costExplorer infra_sdk.Coster
	export       exportSource
}

// exportSource is the part of aws_data_export.Coster the routing needs.
type exportSource interface {
	infra_sdk.Coster
	HasMonth(ctx context.Context, month time.Time) (string, bool, error)
}

func (c Coster) ProviderType() string { return "aws" }

func (c Coster) costExplorerCoster() infra_sdk.Coster {
	if c.costExplorer != nil {
		return c.costExplorer
	}
	return CostExplorerCoster{Accessor: c.Accessor}
}

func (c Coster) exportCoster() exportSource {
	if c.export != nil {
		return c.export
	}
	if c.Export == nil {
		return nil
	}
	return aws_data_export.Coster{Accessor: c.Accessor, Location: *c.Export, Region: c.ExportRegion}
}

// ReferenceCoster is Cost Explorer: the provider's own bill, which exported months reconcile against.
func (c Coster) ReferenceCoster() infra_sdk.Coster {
	return c.costExplorerCoster()
}

// MonthSource reports the Data Export when it has delivered the month, else Cost Explorer.
func (c Coster) MonthSource(ctx context.Context, month time.Time) (infra_sdk.CostSource, error) {
	if export := c.exportCoster(); export != nil {
		etag, ok, err := export.HasMonth(ctx, month)
		if err != nil {
			return infra_sdk.CostSource{}, fmt.Errorf("error checking data export for %s: %w", month.UTC().Format("2006-01"), err)
		}
		if ok {
			return infra_sdk.CostSource{Name: infra_sdk.CostSourceFocusExport, Version: etag, Capabilities: aws_data_export.Capabilities()}, nil
		}
	}
	return infra_sdk.CostSource{Name: infra_sdk.CostSourceCostExplorer, Capabilities: CostExplorerCapabilities()}, nil
}

// GetCosts answers the query month by month from the source that covers each month, merging the
// results. A query that fits inside one source is passed through as is. Hourly granularity is
// always Cost Explorer: the export is delivered daily.
func (c Coster) GetCosts(ctx context.Context, query infra_sdk.CostQuery) (*infra_sdk.CostResult, error) {
	ce := c.costExplorerCoster()
	export := c.exportCoster()
	if export == nil || query.Granularity == infra_sdk.CostGranularityHourly {
		return ce.GetCosts(ctx, query)
	}

	runs, err := c.sourceRuns(ctx, query)
	if err != nil {
		return nil, err
	}
	if len(runs) == 1 && runs[0].start.Equal(query.Start) && runs[0].end.Equal(query.End) {
		return runs[0].coster(ce, export).GetCosts(ctx, query)
	}

	result := infra_sdk.NewCostResult()
	for _, run := range runs {
		sub := query
		sub.Start, sub.End = run.start, run.end
		res, err := run.coster(ce, export).GetCosts(ctx, sub)
		if err != nil {
			return nil, err
		}
		if res == nil {
			continue
		}
		for _, series := range res.Series {
			result.MergeSeries(series, "")
		}
	}
	return result, nil
}

// sourceRun is a contiguous stretch of [start, end) answered by one source.
type sourceRun struct {
	start, end time.Time
	exported   bool
}

func (r sourceRun) coster(ce infra_sdk.Coster, export exportSource) infra_sdk.Coster {
	if r.exported {
		return export
	}
	return ce
}

// sourceRuns splits the query window at every month boundary where the source changes, clipped
// to the window itself.
func (c Coster) sourceRuns(ctx context.Context, query infra_sdk.CostQuery) ([]sourceRun, error) {
	runs := make([]sourceRun, 0)
	for _, month := range aws_data_export.MonthsInRange(query.Start, query.End) {
		source, err := c.MonthSource(ctx, month)
		if err != nil {
			return nil, err
		}
		exported := source.Name == infra_sdk.CostSourceFocusExport
		start, end := month, month.AddDate(0, 1, 0)
		if start.Before(query.Start) {
			start = query.Start
		}
		if end.After(query.End) {
			end = query.End
		}
		if n := len(runs); n > 0 && runs[n-1].exported == exported {
			runs[n-1].end = end
			continue
		}
		runs = append(runs, sourceRun{start: start, end: end, exported: exported})
	}
	return runs, nil
}
