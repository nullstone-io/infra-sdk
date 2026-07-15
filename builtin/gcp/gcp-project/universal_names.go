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

type UniversalDimension string

func (d UniversalDimension) ToGcpColumn() string {
	switch d {
	case infra_sdk.UniversalDimensionAccount:
		return "project.id"
	}
	return string(d)
}

type GcpDimension string

func (d GcpDimension) ToUniversal() string {
	switch d {
	case "project.id":
		return infra_sdk.UniversalDimensionAccount
	}
	return string(d)
}
