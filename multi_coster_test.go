package infra_sdk

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockCoster struct {
	result *CostResult
	err    error
}

func (m *mockCoster) GetCosts(ctx context.Context, query CostQuery) (*CostResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

type mockProviderCoster struct {
	mockCoster
	providerType string
}

func (m *mockProviderCoster) ProviderType() string { return m.providerType }

func TestMultiCoster_StampsProvider(t *testing.T) {
	now := time.Now().UTC()
	dayAgo := now.Add(-24 * time.Hour)

	newResult := func(account, value string) *CostResult {
		result := NewCostResult()
		result.AddDatapoint("cost", CostSeriesGroupKeys{{Name: UniversalDimensionAccount, Value: account}},
			CostSeriesDatapoint{Start: dayAgo, End: now, Value: value, Unit: "USD"})
		return result
	}

	mc := &MultiCoster{Costers: []Coster{
		&mockProviderCoster{mockCoster: mockCoster{result: newResult("111", "10.00")}, providerType: "aws"},
		&mockProviderCoster{mockCoster: mockCoster{result: newResult("222", "20.00")}, providerType: "gcp"},
		// A coster that cannot name its provider still contributes its costs.
		&mockCoster{result: newResult("333", "30.00")},
	}}

	result, err := mc.GetCosts(context.Background(), CostQuery{})
	require.NoError(t, err)
	require.Len(t, result.Series, 3)

	byAccount := map[string]string{}
	for _, series := range result.Series {
		byAccount[series.GroupKeys[0].Value] = series.Provider
	}
	assert.Equal(t, "aws", byAccount["111"])
	assert.Equal(t, "gcp", byAccount["222"])
	assert.Equal(t, "", byAccount["333"])
}

func TestMultiCoster_GetCosts(t *testing.T) {
	now := time.Now().UTC()
	dayAgo := now.Add(-24 * time.Hour)
	tests := []struct {
		name        string
		costers     []Coster
		expectError bool
		validate    func(t *testing.T, result *CostResult)
	}{
		{
			name:        "no costers returns empty result",
			costers:     []Coster{},
			expectError: false,
			validate: func(t *testing.T, result *CostResult) {
				assert.Empty(t, result.Series)
			},
		},
		{
			name: "single coster returns its result",
			costers: []Coster{
				&mockCoster{
					result: &CostResult{
						Series: map[string]CostSeries{
							"nullstone.io/cloud-account$123:cost": {
								MetricName: "cost",
								GroupKeys:  CostSeriesGroupKeys{{Name: UniversalDimensionAccount, Value: "123"}},
								Points: []CostSeriesDatapoint{{
									Start: dayAgo,
									End:   now,
									Value: "100.00",
									Unit:  "USD",
								}},
							},
						},
					},
				},
			},
			expectError: false,
			validate: func(t *testing.T, result *CostResult) {
				require.Len(t, result.Series, 1)
				series, exists := result.Series["nullstone.io/cloud-account$123:cost"]
				require.True(t, exists)
				assert.Equal(t, "cost", series.MetricName)
				require.Len(t, series.Points, 1)
				assert.Equal(t, "100.00", series.Points[0].Value)
			},
		},
		{
			name: "multiple costers with no overlaps are combined",
			costers: []Coster{
				&mockCoster{
					result: &CostResult{
						Series: map[string]CostSeries{
							"nullstone-io/cloud-account$123": {
								MetricName: "cost",
								GroupKeys:  CostSeriesGroupKeys{{Name: UniversalDimensionAccount, Value: "123"}},
								Points: []CostSeriesDatapoint{{
									Start: dayAgo,
									End:   now,
									Value: "100.00",
									Unit:  "USD",
								}},
							},
						},
					},
				},
				&mockCoster{
					result: &CostResult{
						Series: map[string]CostSeries{
							"nullstone-io/cloud-account$456": {
								MetricName: "cost",
								GroupKeys:  CostSeriesGroupKeys{{Name: UniversalDimensionAccount, Value: "456"}},
								Points: []CostSeriesDatapoint{{
									Start: dayAgo,
									End:   now,
									Value: "200.00",
									Unit:  "USD",
								}},
							},
						},
					},
				},
			},
			expectError: false,
			validate: func(t *testing.T, result *CostResult) {
				require.Len(t, result.Series, 2)

				series1, exists := result.Series["nullstone.io/cloud-account$123:cost"]
				require.True(t, exists)
				assert.Equal(t, "cost", series1.MetricName)
				require.Len(t, series1.Points, 1)
				assert.Equal(t, "100.00", series1.Points[0].Value)

				series2, exists := result.Series["nullstone.io/cloud-account$456:cost"]
				require.True(t, exists)
				assert.Equal(t, "cost", series2.MetricName)
				require.Len(t, series2.Points, 1)
				assert.Equal(t, "200.00", series2.Points[0].Value)
			},
		},
		{
			name: "multiple costers with overlapping series are deduped",
			costers: []Coster{
				&mockCoster{
					result: &CostResult{
						Series: map[string]CostSeries{
							"nullstone-io/cloud-account$123": {
								MetricName: "cost",
								GroupKeys:  CostSeriesGroupKeys{{Name: UniversalDimensionAccount, Value: "123"}},
								Points: []CostSeriesDatapoint{{
									Start: dayAgo,
									End:   now,
									Value: "100.00",
									Unit:  "USD",
								}},
							},
						},
					},
				},
				&mockCoster{
					result: &CostResult{
						Series: map[string]CostSeries{
							"nullstone-io/cloud-account$123": {
								MetricName: "cost",
								GroupKeys:  CostSeriesGroupKeys{{Name: UniversalDimensionAccount, Value: "123"}},
								Points: []CostSeriesDatapoint{{
									Start: dayAgo,
									End:   now,
									Value: "200.00",
									Unit:  "USD",
								}},
							},
						},
					},
				},
			},
			expectError: false,
			validate: func(t *testing.T, result *CostResult) {
				require.Len(t, result.Series, 1)
				series, exists := result.Series["nullstone.io/cloud-account$123:cost"]
				require.True(t, exists)
				assert.Equal(t, "cost", series.MetricName)
				// Should only have one point since the second one is a duplicate
				require.Len(t, series.Points, 1)
			},
		},
		{
			name: "error from any coster is returned",
			costers: []Coster{
				&mockCoster{
					err: assert.AnError,
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &MultiCoster{
				Costers: tt.costers,
			}

			result, err := mc.GetCosts(context.Background(), CostQuery{})
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				tt.validate(t, result)
			}
		})
	}
}
