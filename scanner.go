package infra_sdk

import (
	"context"
	"gopkg.in/nullstone-io/go-api-client.v0/types"
)

type ScanResource struct {
	UniqueId            string           `json:"uniqueId"`
	Name                string           `json:"name"`
	Taxonomy            ResourceTaxonomy `json:"taxonomy"`
	ServiceName         string           `json:"serviceName"`
	ServiceResourceName string           `json:"serviceResourceName"`
	Attributes          map[string]any   `json:"attributes"`
}

type ResourceTaxonomy struct {
	Category    types.CategoryName    `json:"category"`
	Subcategory types.SubcategoryName `json:"subcategory"`
	Provider    string                `json:"provider"`
	Platform    string                `json:"platform"`
	Subplatform string                `json:"subplatform"`
}

type Scanner interface {
	Scan(ctx context.Context) ([]ScanResource, error)
}
