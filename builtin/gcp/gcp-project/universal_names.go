package gcp_project

import infra_sdk "github.com/nullstone-io/infra-sdk"

type GcpLabel string

func (l GcpLabel) ToUniversal() string {
	switch l {
	case "stack":
		return infra_sdk.UniversalTagStack
	case "env":
		return infra_sdk.UniversalTagEnv
	case "block":
		return infra_sdk.UniversalTagBlock
	}
	return string(l)
}

type UniversalTag string

func (t UniversalTag) ToGcp() string {
	switch t {
	case infra_sdk.UniversalTagStack:
		return "stack"
	case infra_sdk.UniversalTagEnv:
		return "env"
	case infra_sdk.UniversalTagBlock:
		return "block"
	}
	return string(t)
}

// GcpResourceColumn is the expression that identifies the billed resource in the resource-level
// billing export (gcp_billing_export_resource_v1_*, the only table the integration accepts).
// resource.global_name is the full resource name (//compute.googleapis.com/projects/.../instances/x)
// and plays the FOCUS ResourceId role; some services only populate the short resource.name, and
// rows with no resource at all (fees, some network SKUs) leave both NULL, which groups as "".
const GcpResourceColumn = "IFNULL(resource.global_name, resource.name)"

type UniversalDimension string

func (d UniversalDimension) ToGcpColumn() string {
	switch d {
	case infra_sdk.UniversalDimensionAccount:
		return "project.id"
	case infra_sdk.UniversalDimensionService:
		return "service.description"
	case infra_sdk.UniversalDimensionChargeCategory:
		return "cost_type"
	case infra_sdk.UniversalDimensionResource:
		return GcpResourceColumn
	}
	return string(d)
}

type GcpDimension string

func (d GcpDimension) ToUniversal() string {
	switch d {
	case "project.id":
		return infra_sdk.UniversalDimensionAccount
	case "service.description":
		return infra_sdk.UniversalDimensionService
	case "cost_type":
		return infra_sdk.UniversalDimensionChargeCategory
	case GcpResourceColumn:
		return infra_sdk.UniversalDimensionResource
	}
	return string(d)
}

// GcpCostType is a value of the billing export's cost_type column.
type GcpCostType string

// ToChargeCategory maps a billing export cost_type onto the FOCUS ChargeCategory.
// The standard export never emits Purchase or Credit rows: credits ride along on the usage row
// they discount (see the credits[] handling in QueryBuilder).
func (t GcpCostType) ToChargeCategory() infra_sdk.CostChargeCategory {
	switch t {
	case "regular":
		return infra_sdk.CostChargeCategoryUsage
	case "tax":
		return infra_sdk.CostChargeCategoryTax
	case "adjustment", "rounding_error":
		return infra_sdk.CostChargeCategoryAdjustment
	}
	return infra_sdk.CostChargeCategoryAdjustment
}
