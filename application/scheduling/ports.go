package scheduling

import (
	"context"

	"github.com/Capsule7446/healix-core/domain/workspace"
)

// Scheduler owns queueing and run lifecycle orchestration.
type Scheduler interface {
	CreateRun(context.Context, workspace.TestTaskRunPlan) error
	CancelRun(context.Context, string, int64) error
	ReorderQueue(context.Context, []string) error
	DeleteRun(context.Context, string) error
	RecoverInterrupted(context.Context, int64) error
	ClaimNext(context.Context, int64) (workspace.TestTaskRunPlan, bool, error)
	StartExecution(context.Context, string, int64) error
	FinishExecution(context.Context, string, workspace.ExecutionStatus, int64) error
	FailRun(context.Context, string, string, int64) error
	FinalizeRun(context.Context, string, workspace.TestTaskRunStatus, int64) error
}
