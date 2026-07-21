package evidence

import "errors"

type ProgressPhase string

const (
	ProgressRunning       ProgressPhase = "RUNNING"
	ProgressHealing       ProgressPhase = "HEALING"
	ProgressTransitioning ProgressPhase = "TRANSITIONING"
	ProgressValidating    ProgressPhase = "VALIDATING"
)

type StepProgressEvent struct {
	ID             string
	ExecutionID    string
	WorkflowStepID string
	DisplayName    string
	Kind           string
	Phase          ProgressPhase
	Occurrence     int
	HierarchyPath  string
	Timestamp      int64
}

func (e StepProgressEvent) Validate() error {
	if e.ID == "" || e.ExecutionID == "" || e.WorkflowStepID == "" || e.DisplayName == "" || e.Kind == "" {
		return errors.New("step progress event identity is required")
	}
	switch e.Phase {
	case ProgressRunning, ProgressHealing, ProgressTransitioning, ProgressValidating:
	default:
		return errors.New("step progress event requires a non-terminal phase")
	}
	if e.Occurrence <= 0 || e.Timestamp <= 0 {
		return errors.New("step progress event occurrence and timestamp must be positive")
	}
	return nil
}

// StepPhaseEvent is the framework-neutral terminal execution timeline event.
type StepPhaseEvent struct {
	ID             string
	ExecutionID    string
	WorkflowStepID string
	DisplayName    string
	Kind           string
	Phase          string
	Occurrence     int
	HierarchyPath  string
	Timestamp      int64
	ErrorMessage   string
}
