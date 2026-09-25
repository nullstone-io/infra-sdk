package aws_names

import (
	"testing"

	infra_sdk "github.com/nullstone-io/infra-sdk"
	"github.com/stretchr/testify/assert"
)

func TestAwsRecordType_ToChargeCategory(t *testing.T) {
	assert.Equal(t, infra_sdk.CostChargeCategoryUsage, AwsRecordType("Usage").ToChargeCategory())
	assert.Equal(t, infra_sdk.CostChargeCategoryUsage, AwsRecordType("SavingsPlanCoveredUsage").ToChargeCategory())
	assert.Equal(t, infra_sdk.CostChargeCategoryPurchase, AwsRecordType("RIFee").ToChargeCategory())
	assert.Equal(t, infra_sdk.CostChargeCategoryCredit, AwsRecordType("Refund").ToChargeCategory())
	assert.Equal(t, infra_sdk.CostChargeCategoryAdjustment, AwsRecordType("SavingsPlanNegation").ToChargeCategory())
	assert.Equal(t, infra_sdk.CostChargeCategoryAdjustment, AwsRecordType("SomethingNew").ToChargeCategory())
}
