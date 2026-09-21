package gcp_project

import (
	"testing"
	"time"

	infra_sdk "github.com/nullstone-io/infra-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryBuilder_Build_BasicDaily(t *testing.T) {
	builder := &QueryBuilder{Table: "my_dataset.gcp_billing_export_v1_AABB"}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	result := builder.Build(infra_sdk.CostQuery{
		Start:       start,
		End:         end,
		Granularity: infra_sdk.CostGranularityDaily,
	})

	assert.Contains(t, result.SQL, "TIMESTAMP_TRUNC(usage_start_time, DAY)")
	assert.Contains(t, result.SQL, "INTERVAL 1 DAY")
	assert.Contains(t, result.SQL, "`my_dataset.gcp_billing_export_v1_AABB`")
	assert.Contains(t, result.SQL, "usage_start_time >= @start_time")
	assert.Contains(t, result.SQL, "usage_start_time < @end_time")

	// credit-type sets + start/end
	require.Len(t, result.Params, 4)
	assert.Equal(t, "negotiated_credit_types", result.Params[0].Name)
	assert.Equal(t, "amortized_credit_types", result.Params[1].Name)
	assert.Equal(t, "start_time", result.Params[2].Name)
	assert.Equal(t, "end_time", result.Params[3].Name)
}

func TestQueryBuilder_Build_MonthlyGranularity(t *testing.T) {
	builder := &QueryBuilder{Table: "ds.table"}
	result := builder.Build(infra_sdk.CostQuery{
		Start:       time.Now(),
		End:         time.Now(),
		Granularity: infra_sdk.CostGranularityMonthly,
	})

	assert.Contains(t, result.SQL, "TIMESTAMP_TRUNC(usage_start_time, MONTH)")
	// BigQuery rejects TIMESTAMP_ADD with a MONTH date part, so the month has to be added in the
	// DATE domain. Assert the whole expression -- substring checks let the invalid form through.
	assert.Contains(t, result.SQL,
		"TIMESTAMP(DATE_ADD(DATE(TIMESTAMP_TRUNC(usage_start_time, MONTH)), INTERVAL 1 MONTH)) AS period_end")
	assert.NotContains(t, result.SQL, "TIMESTAMP_ADD(TIMESTAMP_TRUNC(usage_start_time, MONTH), INTERVAL 1 MONTH)")
}

func TestQueryBuilder_Build_SubDayGranularitiesUseTimestampAdd(t *testing.T) {
	// TIMESTAMP_ADD is valid for date parts up to DAY, so those keep the simpler form.
	for _, tt := range []struct {
		granularity infra_sdk.CostGranularity
		expected    string
	}{
		{infra_sdk.CostGranularityHourly, "TIMESTAMP_ADD(TIMESTAMP_TRUNC(usage_start_time, HOUR), INTERVAL 1 HOUR) AS period_end"},
		{infra_sdk.CostGranularityDaily, "TIMESTAMP_ADD(TIMESTAMP_TRUNC(usage_start_time, DAY), INTERVAL 1 DAY) AS period_end"},
	} {
		t.Run(string(tt.granularity), func(t *testing.T) {
			builder := &QueryBuilder{Table: "ds.table"}
			result := builder.Build(infra_sdk.CostQuery{
				Start:       time.Now(),
				End:         time.Now(),
				Granularity: tt.granularity,
			})
			assert.Contains(t, result.SQL, tt.expected)
		})
	}
}

func TestQueryBuilder_Build_HourlyGranularity(t *testing.T) {
	builder := &QueryBuilder{Table: "ds.table"}
	result := builder.Build(infra_sdk.CostQuery{
		Start:       time.Now(),
		End:         time.Now(),
		Granularity: infra_sdk.CostGranularityHourly,
	})

	assert.Contains(t, result.SQL, "TIMESTAMP_TRUNC(usage_start_time, HOUR)")
	assert.Contains(t, result.SQL, "INTERVAL 1 HOUR")
}

func TestQueryBuilder_Build_DefaultGranularity(t *testing.T) {
	builder := &QueryBuilder{Table: "ds.table"}
	result := builder.Build(infra_sdk.CostQuery{
		Start: time.Now(),
		End:   time.Now(),
	})

	assert.Contains(t, result.SQL, "TIMESTAMP_TRUNC(usage_start_time, DAY)")
}

func TestQueryBuilder_Build_WithFilterTags(t *testing.T) {
	builder := &QueryBuilder{Table: "ds.table"}
	result := builder.Build(infra_sdk.CostQuery{
		Start:       time.Now(),
		End:         time.Now(),
		Granularity: infra_sdk.CostGranularityDaily,
		FilterTags: []infra_sdk.CostFilterTag{
			{Key: infra_sdk.UniversalTagStack, Values: []string{"my-stack"}},
			{Key: infra_sdk.UniversalTagEnv, Values: []string{"dev", "staging"}},
		},
	})

	assert.Contains(t, result.SQL, "EXISTS(SELECT 1 FROM UNNEST(labels) l WHERE l.key = @filter_key_0 AND l.value IN UNNEST(@filter_vals_0))")
	assert.Contains(t, result.SQL, "EXISTS(SELECT 1 FROM UNNEST(labels) l WHERE l.key = @filter_key_1 AND l.value IN UNNEST(@filter_vals_1))")

	// 2 credit-type sets, start_time, end_time, + 2 key params + 2 val params = 8
	require.Len(t, result.Params, 8)

	// Verify the filter key params use GCP label names
	paramMap := map[string]interface{}{}
	for _, p := range result.Params {
		paramMap[p.Name] = p.Value
	}
	assert.Equal(t, "stack", paramMap["filter_key_0"])
	assert.Equal(t, []string{"my-stack"}, paramMap["filter_vals_0"])
	assert.Equal(t, "env", paramMap["filter_key_1"])
	assert.Equal(t, []string{"dev", "staging"}, paramMap["filter_vals_1"])
}

func TestQueryBuilder_Build_WithGroupByDimension(t *testing.T) {
	builder := &QueryBuilder{Table: "ds.table"}
	result := builder.Build(infra_sdk.CostQuery{
		Start:       time.Now(),
		End:         time.Now(),
		Granularity: infra_sdk.CostGranularityDaily,
		GroupBy: infra_sdk.CostGroupIdentifiers{
			{Dimension: infra_sdk.UniversalDimensionAccount},
		},
	})

	assert.Contains(t, result.SQL, "project.id AS dim_0")
	assert.Contains(t, result.SQL, "GROUP BY period_start, period_end, dim_0, currency")
}

func TestQueryBuilder_Build_WithGroupByTag(t *testing.T) {
	builder := &QueryBuilder{Table: "ds.table"}
	result := builder.Build(infra_sdk.CostQuery{
		Start:       time.Now(),
		End:         time.Now(),
		Granularity: infra_sdk.CostGranularityDaily,
		GroupBy: infra_sdk.CostGroupIdentifiers{
			{TagKey: infra_sdk.UniversalTagEnv},
		},
	})

	assert.Contains(t, result.SQL, "(SELECT l.value FROM UNNEST(labels) l WHERE l.key = @grp_label_0) AS label_0")
	assert.Contains(t, result.SQL, "GROUP BY period_start, period_end, label_0, currency")

	paramMap := map[string]interface{}{}
	for _, p := range result.Params {
		paramMap[p.Name] = p.Value
	}
	assert.Equal(t, "env", paramMap["grp_label_0"])
}

func TestQueryBuilder_Build_MixedGroupBy(t *testing.T) {
	builder := &QueryBuilder{Table: "ds.table"}
	result := builder.Build(infra_sdk.CostQuery{
		Start:       time.Now(),
		End:         time.Now(),
		Granularity: infra_sdk.CostGranularityDaily,
		GroupBy: infra_sdk.CostGroupIdentifiers{
			{Dimension: infra_sdk.UniversalDimensionAccount},
			{TagKey: infra_sdk.UniversalTagBlock},
		},
	})

	assert.Contains(t, result.SQL, "project.id AS dim_0")
	assert.Contains(t, result.SQL, "(SELECT l.value FROM UNNEST(labels) l WHERE l.key = @grp_label_1) AS label_1")
	assert.Contains(t, result.SQL, "GROUP BY period_start, period_end, dim_0, label_1, currency")
}

func TestQueryBuilder_Build_SelectsFocusMeasures(t *testing.T) {
	builder := &QueryBuilder{Table: "ds.table"}
	result := builder.Build(infra_sdk.CostQuery{
		Start:       time.Now(),
		End:         time.Now(),
		Granularity: infra_sdk.CostGranularityDaily,
	})

	assert.Contains(t, result.SQL, "SUM(IFNULL(cost_at_list, cost)) AS list_cost")
	assert.Contains(t, result.SQL, "WHERE c.type IN UNNEST(@negotiated_credit_types)), 0)) AS contracted_cost")
	assert.Contains(t, result.SQL, "WHERE c.type IN UNNEST(@amortized_credit_types)), 0)) AS effective_cost")
	assert.Contains(t, result.SQL, "SUM(cost) + SUM(IFNULL((SELECT SUM(c.amount) FROM UNNEST(credits) c), 0)) AS billed_cost")
	assert.NotContains(t, result.SQL, "total_cost")

	paramMap := map[string]interface{}{}
	for _, p := range result.Params {
		paramMap[p.Name] = p.Value
	}
	assert.Equal(t, []string{"DISCOUNT", "RESELLER_MARGIN"}, paramMap["negotiated_credit_types"])
	// amortized credits are a superset of negotiated credits
	amortized := paramMap["amortized_credit_types"].([]string)
	assert.Subset(t, amortized, []string{"DISCOUNT", "RESELLER_MARGIN", "COMMITTED_USAGE_DISCOUNT", "SUSTAINED_USAGE_DISCOUNT"})
	assert.NotContains(t, amortized, "PROMOTION")
}

func TestQueryBuilder_Build_WithAbsentFilterTag(t *testing.T) {
	builder := &QueryBuilder{Table: "ds.table"}
	result := builder.Build(infra_sdk.CostQuery{
		Start:       time.Now(),
		End:         time.Now(),
		Granularity: infra_sdk.CostGranularityDaily,
		FilterTags: []infra_sdk.CostFilterTag{
			{Key: infra_sdk.UniversalTagStack, Values: []string{""}},
		},
	})

	// An empty value asks for rows without the label; there is no value list to bind
	assert.Contains(t, result.SQL, "NOT EXISTS(SELECT 1 FROM UNNEST(labels) l WHERE l.key = @filter_key_0)")
	assert.NotContains(t, result.SQL, "@filter_vals_0")
	for _, p := range result.Params {
		assert.NotEqual(t, "filter_vals_0", p.Name)
	}
}

func TestQueryBuilder_Build_WithPresentAndAbsentFilterTag(t *testing.T) {
	builder := &QueryBuilder{Table: "ds.table"}
	result := builder.Build(infra_sdk.CostQuery{
		Start:       time.Now(),
		End:         time.Now(),
		Granularity: infra_sdk.CostGranularityDaily,
		FilterTags: []infra_sdk.CostFilterTag{
			{Key: infra_sdk.UniversalTagStack, Values: []string{"my-stack", ""}},
		},
	})

	assert.Contains(t, result.SQL,
		"(EXISTS(SELECT 1 FROM UNNEST(labels) l WHERE l.key = @filter_key_0 AND l.value IN UNNEST(@filter_vals_0)) OR NOT EXISTS(SELECT 1 FROM UNNEST(labels) l WHERE l.key = @filter_key_0))")

	paramMap := map[string]interface{}{}
	for _, p := range result.Params {
		paramMap[p.Name] = p.Value
	}
	// the absent marker is stripped from the bound values
	assert.Equal(t, []string{"my-stack"}, paramMap["filter_vals_0"])
}

func TestQueryBuilder_Build_WithGroupByChargeCategory(t *testing.T) {
	builder := &QueryBuilder{Table: "ds.table"}
	result := builder.Build(infra_sdk.CostQuery{
		Start:       time.Now(),
		End:         time.Now(),
		Granularity: infra_sdk.CostGranularityDaily,
		GroupBy: infra_sdk.CostGroupIdentifiers{
			{Dimension: infra_sdk.UniversalDimensionChargeCategory},
		},
	})

	assert.Contains(t, result.SQL, "cost_type AS dim_0")
	assert.Contains(t, result.SQL, "GROUP BY period_start, period_end, dim_0, currency")
}
