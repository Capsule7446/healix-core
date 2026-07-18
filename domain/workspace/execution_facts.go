package workspace

import (
	"errors"
	"fmt"
	"strings"
)

// HealObservationCommit keeps the preallocated promotion identity beside the
// immutable observation fact so a retry cannot create a different version.
type HealObservationCommit struct {
	Observation       HealObservation
	PromotedVersionID string
}

type HealCandidateReset struct {
	NodeID            string
	BaseNodeVersionID string
	ObservedAt        int64
}

// StepTransitionCommit is the atomic unit of work for one terminal phase
// change. Final validation and healing facts can only be persisted here.
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
	if strings.TrimSpace(c.Event.ID) == "" || strings.TrimSpace(c.Event.ExecutionID) == "" ||
		strings.TrimSpace(c.Event.WorkflowStepID) == "" {
		return errors.New("step transition commit event identity is required")
	}
	if strings.TrimSpace(c.Event.DisplayName) == "" || strings.TrimSpace(c.Event.Kind) == "" {
		return errors.New("step transition commit event display name and kind are required")
	}
	switch c.Event.Phase {
	case "SUCCEEDED", "FAILED", "CANCELED":
	default:
		return errors.New("step transition commit requires a terminal phase")
	}
	if c.Event.Occurrence <= 0 || c.Event.Timestamp <= 0 {
		return errors.New("step transition commit occurrence and timestamp must be positive")
	}
	for _, observation := range c.FinalValidations {
		if err := observation.Validate(); err != nil {
			return fmt.Errorf("final validation %s: %w", observation.ID, err)
		}
		if !observation.Final || observation.StepExecutionID != c.Event.ID || observation.ExecutionID != c.Event.ExecutionID {
			return errors.New("final validation must belong to the committed step and execution")
		}
	}
	for _, effect := range c.HealObservations {
		observation := effect.Observation
		if err := observation.Validate(); err != nil {
			return fmt.Errorf("heal observation %s: %w", observation.ID, err)
		}
		if observation.StepExecutionID != c.Event.ID || observation.ExecutionID != c.Event.ExecutionID {
			return errors.New("heal observation must belong to the committed step and execution")
		}
		if strings.TrimSpace(effect.PromotedVersionID) == "" {
			return errors.New("heal observation commit is missing a promoted version identity")
		}
	}
	for _, reset := range c.OriginalSelectorResets {
		if strings.TrimSpace(reset.NodeID) == "" || strings.TrimSpace(reset.BaseNodeVersionID) == "" || reset.ObservedAt <= 0 {
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
