package gcp_project

import (
	"testing"

	infra_sdk "github.com/nullstone-io/infra-sdk"
	"github.com/stretchr/testify/assert"
)

func TestGcpLabel_ToUniversal(t *testing.T) {
	tests := []struct {
		label    GcpLabel
		expected string
	}{
		{"stack", infra_sdk.UniversalTagStack},
		{"env", infra_sdk.UniversalTagEnv},
		{"block", infra_sdk.UniversalTagBlock},
		{"custom-label", "custom-label"},
	}
	for _, tt := range tests {
		t.Run(string(tt.label), func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.label.ToUniversal())
		})
	}
}

func TestUniversalTag_ToGcp(t *testing.T) {
	tests := []struct {
		tag      UniversalTag
		expected string
	}{
		{infra_sdk.UniversalTagStack, "stack"},
		{infra_sdk.UniversalTagEnv, "env"},
		{infra_sdk.UniversalTagBlock, "block"},
		{"custom-tag", "custom-tag"},
	}
	for _, tt := range tests {
		t.Run(string(tt.tag), func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.tag.ToGcp())
		})
	}
}

func TestUniversalDimension_ToGcpColumn(t *testing.T) {
	assert.Equal(t, "project.id", UniversalDimension(infra_sdk.UniversalDimensionAccount).ToGcpColumn())
	assert.Equal(t, "custom-dim", UniversalDimension("custom-dim").ToGcpColumn())
}

func TestGcpDimension_ToUniversal(t *testing.T) {
	assert.Equal(t, infra_sdk.UniversalDimensionAccount, GcpDimension("project.id").ToUniversal())
	assert.Equal(t, "custom-dim", GcpDimension("custom-dim").ToUniversal())
}

func TestRoundTrip_Tags(t *testing.T) {
	tags := []string{infra_sdk.UniversalTagStack, infra_sdk.UniversalTagEnv, infra_sdk.UniversalTagBlock}
	for _, tag := range tags {
		gcpLabel := UniversalTag(tag).ToGcp()
		roundTripped := GcpLabel(gcpLabel).ToUniversal()
		assert.Equal(t, tag, roundTripped, "round trip failed for %s", tag)
	}
}
