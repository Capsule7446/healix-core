// Package evidence owns framework-neutral execution fact identity and terminality.
package evidence

import (
	"errors"
	"fmt"
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
	ID            string
	RunID         string
	ExecutionID   string
	StepExecution string
	Phase         Phase
	ObservedAt    int64
}

func (f StepFact) Validate() error {
	if f.ID == "" || f.RunID == "" || f.ExecutionID == "" || f.StepExecution == "" {
		return errors.New("step fact requires identity")
	}
	if !f.Phase.IsTerminal() {
		return fmt.Errorf("step fact phase %q is not terminal", f.Phase)
	}
	if f.ObservedAt <= 0 {
		return errors.New("step fact requires positive observation time")
	}
	return nil
}

type CommitResult struct {
	CommitID string
	Applied  bool
}
