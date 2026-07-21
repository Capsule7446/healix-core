package execution

import (
	"context"

	"github.com/Capsule7446/healix-core/domain/workspace"
)

// FactCommitter persists an atomic terminal step transition and its final facts.
type FactCommitter interface {
	CommitStepTransition(context.Context, workspace.StepTransitionCommit) (workspace.StepTransitionCommitResult, error)
}

// ProgressWriter persists non-terminal execution observations.
type ProgressWriter interface {
	RecordStepProgress(context.Context, workspace.StepPhaseEvent) error
	RecordValidationProgress(context.Context, workspace.ValidationObservation) error
	AttachTerminalStepError(context.Context, workspace.StepPhaseEvent) error
}

// RunCoordinator owns application-level run lifecycle transitions.
type RunCoordinator interface {
	Create(context.Context, workspace.TestTaskRunPlan) error
	ClaimNext(context.Context, int64) (workspace.TestTaskRunPlan, bool, error)
	StartWorkflow(context.Context, string, int64) error
	FinishWorkflow(context.Context, string, workspace.ExecutionStatus, int64) error
	Fail(context.Context, string, string, int64) error
	Finalize(context.Context, string, workspace.TestTaskRunStatus, int64) error
	Cancel(context.Context, string, int64) error
}
