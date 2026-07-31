package evidence

import (
	"errors"
	"fmt"
	"strings"
)

type HealCandidateReset struct {
	ExecutionID       string
	StepExecutionID   string
	ElementTargetID   string
	BaseNodeVersionID string
	ObservedAt        int64
}

type StepRevision uint64

const maxStepTransitionFacts = 10_000

type StepTransitionCommit struct {
	CommitID               string
	ExpectedRevision       StepRevision
	Event                  StepPhaseEvent
	FinalValidations       []ValidationObservation
	FinalValidationGroups  []ValidationGroupTerminalObservation
	HealObservations       []HealObservation
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
		len(c.FinalValidationGroups) > maxStepTransitionFacts ||
		len(c.HealObservations) > maxStepTransitionFacts ||
		len(c.OriginalSelectorResets) > maxStepTransitionFacts ||
		len(c.FinalValidations)+len(c.FinalValidationGroups)+len(c.HealObservations)+len(c.OriginalSelectorResets) > maxStepTransitionFacts {
		return errors.New("step transition commit exceeds fact limit")
	}
	for _, validation := range c.FinalValidations {
		if err := validation.Validate(); err != nil {
			return fmt.Errorf("final validation %s: %w", validation.ID, err)
		}
		if !validation.Final || validation.StepExecutionID != c.Event.ID || validation.ExecutionID != c.Event.ExecutionID {
			return errors.New("final validation must belong to committed step and execution")
		}
	}
	if err := validateValidationGroupTopology(c.Event, c.FinalValidations, c.FinalValidationGroups); err != nil {
		return err
	}
	seenHealIDs := make(map[string]struct{}, len(c.HealObservations))
	for _, heal := range c.HealObservations {
		if _, exists := seenHealIDs[heal.ID]; exists {
			return errors.New("heal observation identity is duplicated")
		}
		seenHealIDs[heal.ID] = struct{}{}
		if err := heal.Validate(); err != nil {
			return fmt.Errorf("heal observation %s: %w", heal.ID, err)
		}
		if heal.StepExecutionID != c.Event.ID || heal.ExecutionID != c.Event.ExecutionID {
			return errors.New("heal observation must belong to committed step and execution")
		}
		if heal.Succeeded && c.Event.Phase != "SUCCEEDED" {
			return errors.New("successful heal observation requires a succeeded terminal step")
		}
	}
	type resetIdentity struct {
		ExecutionID       string
		StepExecutionID   string
		ElementTargetID   string
		BaseNodeVersionID string
	}
	seenResets := make(map[resetIdentity]struct{}, len(c.OriginalSelectorResets))
	for _, reset := range c.OriginalSelectorResets {
		identity := resetIdentity{reset.ExecutionID, reset.StepExecutionID, reset.ElementTargetID, reset.BaseNodeVersionID}
		if _, exists := seenResets[identity]; exists {
			return errors.New("heal candidate reset identity is duplicated")
		}
		seenResets[identity] = struct{}{}
		if c.Event.Phase != "SUCCEEDED" {
			return errors.New("original selector reset requires a succeeded terminal step")
		}
		if reset.ExecutionID != c.Event.ExecutionID || reset.StepExecutionID != c.Event.ID || reset.ElementTargetID == "" || reset.BaseNodeVersionID == "" || reset.ObservedAt <= 0 {
			return errors.New("heal candidate reset must belong to committed step and execution")
		}
	}
	return nil
}

func validateValidationGroupTopology(event StepPhaseEvent, validations []ValidationObservation, groups []ValidationGroupTerminalObservation) error {
	type memberKey struct {
		GroupID         string
		BranchID        string
		ElementTargetID string
	}
	members := make(map[memberKey]ValidationObservation, len(validations))
	seenValidationIDs := make(map[string]struct{}, len(validations))
	for _, validation := range validations {
		if _, exists := seenValidationIDs[validation.ID]; exists {
			return errors.New("final validation identity is duplicated")
		}
		seenValidationIDs[validation.ID] = struct{}{}
		if validation.GroupID == "" {
			continue
		}
		key := memberKey{GroupID: validation.GroupID, BranchID: validation.BranchID, ElementTargetID: validation.ElementTargetID}
		if _, exists := members[key]; exists {
			return errors.New("final validation member is duplicated")
		}
		members[key] = validation
	}
	seenGroups := make(map[string]struct{}, len(groups))
	seenGroupIDs := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		if err := group.Validate(); err != nil {
			return fmt.Errorf("final validation group %s: %w", group.ID, err)
		}
		if _, exists := seenGroupIDs[group.ID]; exists {
			return errors.New("final validation group identity is duplicated")
		}
		seenGroupIDs[group.ID] = struct{}{}
		if group.StepExecutionID != event.ID || group.ExecutionID != event.ExecutionID {
			return errors.New("final validation group must belong to committed step and execution")
		}
		if !validationTerminalMatchesPhase(group.TerminalReason, event.Phase) {
			return errors.New("final validation group terminal reason contradicts step phase")
		}
		if _, exists := seenGroups[group.GroupID]; exists {
			return errors.New("final validation group is duplicated")
		}
		seenGroups[group.GroupID] = struct{}{}
		for _, expected := range group.ExpectedMembers() {
			member, exists := members[memberKey{GroupID: group.GroupID, BranchID: expected.BranchID, ElementTargetID: expected.ElementTargetID}]
			if !exists {
				return errors.New("final validation group is missing an expected member")
			}
			if member.BranchDisposition == ValidationBranchWon && group.TerminalReason != ValidationTerminalPassed {
				return errors.New("non-passed validation group member is marked won")
			}
			if (member.BranchDisposition == ValidationBranchWon || member.BranchDisposition == ValidationBranchSatisfiedNotWinner) && !member.Passed {
				return errors.New("satisfied validation branch member did not pass")
			}
			if group.TerminalReason == ValidationTerminalPassed {
				if expected.BranchID == group.WinningBranchID && member.BranchDisposition != ValidationBranchWon {
					return errors.New("winning validation branch member is not marked won")
				}
				if expected.BranchID != group.WinningBranchID && member.BranchDisposition == ValidationBranchWon {
					return errors.New("non-winning validation branch member is marked won")
				}
			}
			delete(members, memberKey{GroupID: group.GroupID, BranchID: expected.BranchID, ElementTargetID: expected.ElementTargetID})
		}
	}
	// members has been drained of every member its group consumed, so whatever is
	// left is unaccounted for. Ranging over the map would report a random one of
	// the leftovers, which means the same commit could be rejected with a different
	// error on a different run. Walking the source slice instead makes the reported
	// failure a function of the input alone.
	for _, validation := range validations {
		if validation.GroupID == "" {
			continue
		}
		key := memberKey{GroupID: validation.GroupID, BranchID: validation.BranchID, ElementTargetID: validation.ElementTargetID}
		if _, leftover := members[key]; !leftover {
			continue
		}
		if _, grouped := seenGroups[validation.GroupID]; grouped {
			return errors.New("final validation group contains an unexpected member")
		}
		return errors.New("grouped final validation has no terminal group fact")
	}
	return nil
}

func validationTerminalMatchesPhase(reason ValidationTerminalReason, phase string) bool {
	switch reason {
	case ValidationTerminalPassed:
		return phase == "SUCCEEDED"
	case ValidationTerminalTimeout, ValidationTerminalSystemError:
		return phase == "FAILED"
	case ValidationTerminalCanceled:
		return phase == "CANCELED" || phase == "ABORTED"
	default:
		return false
	}
}

type NodeVersionPromotion struct {
	ElementTargetID string
	VersionID       string
}

type StepTransitionCommitResult struct {
	Revision   StepRevision
	WasApplied bool
	Promotions []NodeVersionPromotion
}
