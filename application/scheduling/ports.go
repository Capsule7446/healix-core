package scheduling

import (
	"context"

	"github.com/Capsule7446/healix-core/domain/execution"
)

type RunCommands interface {
	CreateRun(context.Context, execution.Plan) error
	CancelRun(context.Context, string, int64) error
	DeleteRun(context.Context, string) error
}

type QueueOrderWriter interface {
	ReorderQueue(context.Context, []string) error
}
