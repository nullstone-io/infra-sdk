package infra_sdk

const (
	UniversalTagStack = "nullstone.io/stack"
	UniversalTagEnv   = "nullstone.io/env"
	UniversalTagBlock = "nullstone.io/block"

	UniversalDimensionAccount = "nullstone.io/cloud-account"
	UniversalDimensionService = "nullstone.io/service"
	// UniversalDimensionChargeCategory groups by the kind of charge (FOCUS ChargeCategory).
	// Costers translate their native record/cost types to CostChargeCategory values.
	UniversalDimensionChargeCategory = "nullstone.io/charge-category"
	// UniversalDimensionResource groups by the individual billed resource (FOCUS ResourceId).
	// Only costers backed by resource-level billing data (AWS Data Exports, GCP billing export)
	// can answer it; Cost Explorer cannot.
	UniversalDimensionResource = "nullstone.io/resource"
)
