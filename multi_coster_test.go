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
