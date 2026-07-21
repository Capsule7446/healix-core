package execution

import (
	"context"
	"errors"

	"github.com/Capsule7446/healix-core/domain/evidence"
)

var (
	ErrStepRevisionConflict   = errors.New("step revision conflict")
	ErrCommitIdentityConflict = errors.New("step transition commit identity conflict")
)

type WorkerScope struct {
	RunID      string
	ClaimToken string
}

// FactCommitter persists an atomic terminal step transition and its final facts.
// It must fence the worker scope and verify every promotion/reset target against
// the sealed node dependencies of the committed step.
type FactCommitter interface {
	CommitStepTransition(context.Context, WorkerScope, evidence.StepTransitionCommit) (evidence.StepTransitionCommitResult, error)
}

// ProgressWriter persists non-terminal execution observations under the active worker claim.
type ProgressWriter interface {
	RecordStepProgress(context.Context, WorkerScope, evidence.StepProgressEvent) error
	RecordValidationProgress(context.Context, WorkerScope, evidence.ValidationProgressObservation) error
}
