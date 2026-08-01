// Package evidence owns framework-neutral execution fact identity and terminality.
package evidence

import (
	"strings"

	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

type Phase string

const (
	PhaseSucceeded Phase = "SUCCEEDED"
	PhaseFailed    Phase = "FAILED"
	PhaseCanceled  Phase = "CANCELED"
	PhaseAborted   Phase = "ABORTED"
)

func (p Phase) IsTerminal() bool {
	return p == PhaseSucceeded || p == PhaseFailed || p == PhaseCanceled || p == PhaseAborted
}

type StepFact struct {
	ID              string
	InstanceID      execution.InstanceID
	EntryID         execution.EntryID
	StepExecutionID execution.StepExecutionID
	Occurrence      int
	Phase           Phase
	ObservedAt      int64
}

// Validate never echoes the phase. Phase is a closed set, so a non-terminal value
// is either one of the known non-terminal states or arbitrary caller input; the
// caller can read its own phase back from the fact either way.
func (f StepFact) Validate() error {
	var violations []fault.Violation
	if strings.TrimSpace(f.ID) == "" || f.InstanceID.Validate() != nil || f.EntryID.Validate() != nil || f.StepExecutionID.Validate() != nil {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "identity", "step fact identity is required"))
	}
	if !f.Phase.IsTerminal() {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "phase", "step fact phase must be terminal"))
	}
	if f.ObservedAt <= 0 {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "observedAt", "step fact observation time must be positive"))
	}
	if len(violations) != 0 {
		return stepFactInvalidError(violations)
	}
	return nil
}

type CommitResult struct {
	CommitID string
	Applied  bool
}
