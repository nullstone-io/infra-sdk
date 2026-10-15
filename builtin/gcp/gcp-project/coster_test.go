package gcp_project

import (
	"testing"

	infra_sdk "github.com/nullstone-io/infra-sdk"
	"github.com/stretchr/testify/assert"
)

func TestCapabilities(t *testing.T) {
	caps := Capabilities()
	assert.Equal(t, 0, caps.MaxGroupBy, "the billing export accepts any number of groupings in one query")
	assert.Equal(t, infra_sdk.AllCostMetrics(), caps.Metrics)
	assert.True(t, caps.HasChargeCategory)
	assert.True(t, caps.HasResource, "the resource-level export carries the billed resource")
}
