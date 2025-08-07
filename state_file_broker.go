package infra_sdk

import (
	"context"
)

type StateFileBroker interface {
	Lock(ctx context.Context) error
	Unlock(ctx context.Context) error
	Initialize(ctx context.Context) error
	Save(ctx context.Context, stateFile StateFile) error
}
