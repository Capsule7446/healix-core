package execution

import (
	"context"

	"github.com/Capsule7446/healix-core/domain/evidence"
	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/workspace"
)

// FactCommitter persists an atomic terminal step transition and its final facts.
type FactCommitter interface {
	CommitStepTransition(context.Context, evidence.StepTransitionCommit) (evidence.StepTransitionCommitResult, error)
}

// ProgressWriter persists non-terminal execution observations.
type ProgressWriter interface {
	RecordStepProgress(context.Context, evidence.StepPhaseEvent) error
	RecordValidationProgress(context.Context, evidence.ValidationObservation) error
	AttachTerminalStepError(context.Context, evidence.StepPhaseEvent) error
}

// RunCoordinator owns application-level run lifecycle transitions.
type RunCoordinator interface {
	Create(context.Context, execution.Plan) error
	ClaimNext(context.Context, int64) (execution.Plan, bool, error)
	StartWorkflow(context.Context, string, int64) error
	FinishWorkflow(context.Context, string, workspace.ExecutionStatus, int64) error
	Fail(context.Context, string, string, int64) error
	Finalize(context.Context, string, workspace.TestTaskRunStatus, int64) error
	Cancel(context.Context, string, int64) error
}
