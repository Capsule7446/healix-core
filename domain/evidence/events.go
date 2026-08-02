package evidence

import (
	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

type ProgressPhase string

const (
	ProgressRunning       ProgressPhase = "RUNNING"
	ProgressHealing       ProgressPhase = "HEALING"
	ProgressTransitioning ProgressPhase = "TRANSITIONING"
	ProgressValidating    ProgressPhase = "VALIDATING"
)

type StepProgressEvent struct {
	ID                 execution.StepExecutionID
	EntryID            execution.EntryID
	InvocationPath     execution.InvocationPath
	FlowFragmentStepID string
	DisplayName        string
	Kind               string
	Phase              ProgressPhase
	Occurrence         int
	HierarchyPath      string
	Timestamp          int64
}

func (e StepProgressEvent) Validate() error {
	var violations []fault.Violation
	if e.ID.Validate() != nil || e.EntryID.Validate() != nil || e.FlowFragmentStepID == "" || e.DisplayName == "" || e.Kind == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "identity", "event identity is required"))
	}
	switch e.Phase {
	case ProgressRunning, ProgressHealing, ProgressTransitioning, ProgressValidating:
	default:
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "phase", "event phase must be non-terminal"))
	}
	violations = appendOccurrenceViolations(violations, e.Occurrence, "")
	if e.Timestamp <= 0 {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "timestamp", "event timestamp must be positive"))
	}
	if len(violations) != 0 {
		return stepProgressEventInvalidError(violations)
	}
	return nil
}

// StepPhaseEvent is the framework-neutral terminal execution timeline event.
type StepPhaseEvent struct {
	ID                 execution.StepExecutionID
	EntryID            execution.EntryID
	InvocationPath     execution.InvocationPath
	FlowFragmentStepID string
	DisplayName        string
	Kind               string
	Phase              string
	Occurrence         int
	HierarchyPath      string
	Timestamp          int64
	ErrorMessage       string
}
