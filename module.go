package infra_sdk

import "context"

type Module interface {
	Refresh(ctx context.Context, stateFileBroker StateFileBroker) (StateFile, error)
}
