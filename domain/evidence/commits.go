package evidence

import (
	"errors"
	"fmt"
	"strings"
)

type HealObservationCommit struct {
	Observation       HealObservation
	PromotedVersionID string
}

type HealCandidateReset struct {
	NodeID            string
	BaseNodeVersionID string
	ObservedAt        int64
}

type StepTransitionCommit struct {
	CommitID               string
	Event                  StepPhaseEvent
	FinalValidations       []ValidationObservation
	HealObservations       []HealObservationCommit
	OriginalSelectorResets []HealCandidateReset
}

func (c StepTransitionCommit) Validate() error {
	if strings.TrimSpace(c.CommitID) == "" {
		return errors.New("step transition commit id is required")
	}
	if c.Event.ID == "" || c.Event.ExecutionID == "" || c.Event.WorkflowStepID == "" || c.Event.DisplayName == "" || c.Event.Kind == "" {
		return errors.New("step transition commit event identity is required")
	}
	switch c.Event.Phase {
	case "SUCCEEDED", "FAILED", "CANCELED", "ABORTED":
	default:
		return errors.New("step transition commit requires a terminal phase")
	}
	if c.Event.Occurrence <= 0 || c.Event.Timestamp <= 0 {
		return errors.New("step transition commit occurrence and timestamp must be positive")
	}
	for _, validation := range c.FinalValidations {
		if err := validation.Validate(); err != nil {
			return fmt.Errorf("final validation %s: %w", validation.ID, err)
		}
		if !validation.Final || validation.StepExecutionID != c.Event.ID || validation.ExecutionID != c.Event.ExecutionID {
			return errors.New("final validation must belong to committed step and execution")
		}
	}
	for _, heal := range c.HealObservations {
		if err := heal.Observation.Validate(); err != nil {
			return fmt.Errorf("heal observation %s: %w", heal.Observation.ID, err)
		}
		if heal.Observation.StepExecutionID != c.Event.ID || heal.Observation.ExecutionID != c.Event.ExecutionID || heal.PromotedVersionID == "" {
			return errors.New("heal observation commit has invalid identity")
		}
	}
	for _, reset := range c.OriginalSelectorResets {
		if reset.NodeID == "" || reset.BaseNodeVersionID == "" || reset.ObservedAt <= 0 {
			return errors.New("heal candidate reset is missing a required field")
		}
	}
	return nil
}

type NodeVersionPromotion struct {
	NodeID    string
	VersionID string
}

type StepTransitionCommitResult struct {
	Promotions []NodeVersionPromotion
}
