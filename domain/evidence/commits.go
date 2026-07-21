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

type StepRevision uint64

const (
	maxStepTransitionFacts = 10_000
	maxStepTransitionBytes = 1 << 20
)

type StepTransitionCommit struct {
	CommitID               string
	ExpectedRevision       StepRevision
	Event                  StepPhaseEvent
	FinalValidations       []ValidationObservation
	HealObservations       []HealObservationCommit
	OriginalSelectorResets []HealCandidateReset
}

func (c StepTransitionCommit) Validate() error {
	if strings.TrimSpace(c.CommitID) == "" {
		return errors.New("step transition commit id is required")
	}
	if c.ExpectedRevision == 0 {
		return errors.New("step transition expected revision must be non-zero")
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
	if len(c.FinalValidations) > maxStepTransitionFacts ||
		len(c.HealObservations) > maxStepTransitionFacts ||
		len(c.OriginalSelectorResets) > maxStepTransitionFacts ||
		len(c.FinalValidations)+len(c.HealObservations)+len(c.OriginalSelectorResets) > maxStepTransitionFacts {
		return errors.New("step transition commit exceeds fact limit")
	}
	if stepTransitionPayloadBytes(c) > maxStepTransitionBytes {
		return errors.New("step transition commit exceeds byte limit")
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

func stepTransitionPayloadBytes(commit StepTransitionCommit) int {
	total := len(commit.CommitID) + stepEventBytes(commit.Event)
	for _, validation := range commit.FinalValidations {
		total += validationBytes(validation)
	}
	for _, heal := range commit.HealObservations {
		total += healObservationBytes(heal.Observation) + len(heal.PromotedVersionID)
	}
	for _, reset := range commit.OriginalSelectorResets {
		total += len(reset.NodeID) + len(reset.BaseNodeVersionID)
	}
	return total
}

func stepEventBytes(event StepPhaseEvent) int {
	return len(event.ID) + len(event.ExecutionID) + len(event.WorkflowStepID) + len(event.DisplayName) +
		len(event.Kind) + len(event.Phase) + len(event.HierarchyPath) + len(event.ErrorMessage)
}

func validationBytes(observation ValidationObservation) int {
	return len(observation.ID) + len(observation.RunID) + len(observation.ExecutionID) +
		len(observation.StepExecutionID) + len(observation.ValidationStepID) + len(observation.NodeID) +
		len(observation.NodeVersionID) + len(observation.AssertionKind) + len(observation.Expected) +
		len(observation.Actual) + len(observation.Reason) + len(observation.Selector.Type) +
		len(observation.Selector.Value) + len(observation.HealReviewStatus)
}

func healObservationBytes(observation HealObservation) int {
	total := len(observation.ID) + len(observation.RunID) + len(observation.ExecutionID) +
		len(observation.StepExecutionID) + len(observation.NodeID) + len(observation.BaseNodeVersionID) +
		len(observation.CandidateHash) + len(observation.Selector.Type) + len(observation.Selector.Value) +
		len(observation.DecisionBand)
	fingerprint := observation.Fingerprint
	total += len(fingerprint.Tag) + len(fingerprint.Text) + len(fingerprint.ARIA.Role) + len(fingerprint.ARIA.Name) +
		len(fingerprint.Neighbors.Prev) + len(fingerprint.Neighbors.Next) + len(fingerprint.Neighbors.ParentTag) +
		len(fingerprint.LabelText) + len(fingerprint.FormID)
	for key, value := range fingerprint.Attributes {
		total += len(key) + len(value)
	}
	for _, path := range fingerprint.Path {
		total += len(path)
	}
	for _, info := range fingerprint.Framework {
		total += len(info.Kind) + len(info.Version) + len(info.Evidence)
	}
	return total
}

type NodeVersionPromotion struct {
	NodeID    string
	VersionID string
}

type StepTransitionCommitResult struct {
	Revision   StepRevision
	WasApplied bool
	Promotions []NodeVersionPromotion
}
