package aws_account

import infra_sdk "github.com/nullstone-io/infra-sdk"

type AwsDimension string

func (d AwsDimension) ToUniversal() string {
	switch d {
	case "LINKED_ACCOUNT":
		return infra_sdk.UniversalDimensionAccount
	}
	return string(d)
}

type UniversalDimension string

func (d UniversalDimension) ToAws() string {
	switch d {
	case infra_sdk.UniversalDimensionAccount:
		return "LINKED_ACCOUNT"
	}
	return string(d)
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
