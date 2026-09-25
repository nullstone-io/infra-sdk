package aws_names

import infra_sdk "github.com/nullstone-io/infra-sdk"

type AwsDimension string

func (d AwsDimension) ToUniversal() string {
	switch d {
	case "LINKED_ACCOUNT":
		return infra_sdk.UniversalDimensionAccount
	case "SERVICE":
		return infra_sdk.UniversalDimensionService
	case "RECORD_TYPE":
		return infra_sdk.UniversalDimensionChargeCategory
	}
	return string(d)
}

type UniversalDimension string

func (d UniversalDimension) ToAws() string {
	switch d {
	case infra_sdk.UniversalDimensionAccount:
		return "LINKED_ACCOUNT"
	case infra_sdk.UniversalDimensionService:
		return "SERVICE"
	case infra_sdk.UniversalDimensionChargeCategory:
		return "RECORD_TYPE"
	}
	return string(d)
}

// AwsRecordType is a value of the Cost Explorer RECORD_TYPE dimension (the CUR line_item_type).
type AwsRecordType string

// ToChargeCategory maps a Cost Explorer record type onto the FOCUS ChargeCategory.
// This follows the AWS FOCUS 1.0/1.2 data export mapping of line_item_type.
func (r AwsRecordType) ToChargeCategory() infra_sdk.CostChargeCategory {
	switch r {
	case "Usage", "DiscountedUsage", "SavingsPlanCoveredUsage", "Support":
		return infra_sdk.CostChargeCategoryUsage
	case "Fee", "RIFee", "SavingsPlanUpfrontFee", "SavingsPlanRecurringFee":
		return infra_sdk.CostChargeCategoryPurchase
	case "Tax":
		return infra_sdk.CostChargeCategoryTax
	case "Credit", "Refund":
		return infra_sdk.CostChargeCategoryCredit
	case "SavingsPlanNegation", "Enterprise Discount Program Discount", "Solution Provider Program Discount",
		"Bundled Discount", "Private Rate Discount", "Distributor Discount":
		return infra_sdk.CostChargeCategoryAdjustment
	}
	return infra_sdk.CostChargeCategoryAdjustment
}

type AwsTag string

func (t AwsTag) ToUniversal() string {
	switch t {
	case "Stack":
		return infra_sdk.UniversalTagStack
	case "Env":
		return infra_sdk.UniversalTagEnv
	case "Block":
		return infra_sdk.UniversalTagBlock
	}
	return string(t)
}

type UniversalTag string

func (t UniversalTag) ToAws() string {
	switch t {
	case infra_sdk.UniversalTagStack:
		return "Stack"
	case infra_sdk.UniversalTagEnv:
		return "Env"
	case infra_sdk.UniversalTagBlock:
		return "Block"
	}
	return string(t)
}
