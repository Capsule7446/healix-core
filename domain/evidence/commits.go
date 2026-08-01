package evidence

import (
	"fmt"
	"strings"

	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

type HealCandidateReset struct {
	ExecutionID       execution.EntryID
	StepExecutionID   execution.StepExecutionID
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

// Validate reports every failure through one aggregate envelope carrying ordered
// violations. Collection indexes are 0-based and address the slice the caller
// passed; no commit, execution, step, validation, heal, group, or element target
// identity reaches public text.
func (c StepTransitionCommit) Validate() error {
	// The fact limit is checked before anything else, unlike the previous ordering,
	// because it also bounds the violation walks below: without it a maximum-size
	// commit would be walked in full before being rejected for being too large.
	// It carries its own code, so it cannot share this envelope anyway.
	if c.exceedsFactLimit() {
		return commitFactLimitExceededError()
	}
	var violations []fault.Violation
	if strings.TrimSpace(c.CommitID) == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "commitId", "commit id is required"))
	}
	if c.ExpectedRevision == 0 {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "expectedRevision", "expected revision must be non-zero"))
	}
	if c.Event.ID.Validate() != nil || c.Event.ExecutionID.Validate() != nil || c.Event.WorkflowStepID == "" || c.Event.DisplayName == "" || c.Event.Kind == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "event.identity", "event identity is required"))
	}
	if !isTerminalPhase(c.Event.Phase) {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "event.phase", "event phase must be terminal"))
	}
	if c.Event.Occurrence <= 0 {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "event.occurrence", "event occurrence must be positive"))
	}
	if c.Event.Timestamp <= 0 {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "event.timestamp", "event timestamp must be positive"))
	}
	violations = c.appendFinalValidationViolations(violations)
	violations = appendGroupTopologyViolations(violations, c.Event, c.FinalValidations, c.FinalValidationGroups)
	violations = c.appendHealObservationViolations(violations)
	violations = c.appendSelectorResetViolations(violations)
	if len(violations) != 0 {
		return stepTransitionCommitInvalidError(violations)
	}
	return nil
}

func (c StepTransitionCommit) exceedsFactLimit() bool {
	return len(c.FinalValidations) > maxStepTransitionFacts ||
		len(c.FinalValidationGroups) > maxStepTransitionFacts ||
		len(c.HealObservations) > maxStepTransitionFacts ||
		len(c.OriginalSelectorResets) > maxStepTransitionFacts ||
		len(c.FinalValidations)+len(c.FinalValidationGroups)+len(c.HealObservations)+len(c.OriginalSelectorResets) > maxStepTransitionFacts
}

func isTerminalPhase(phase string) bool {
	switch phase {
	case "SUCCEEDED", "FAILED", "CANCELED", "ABORTED":
		return true
	default:
		return false
	}
}

func (c StepTransitionCommit) appendFinalValidationViolations(violations []fault.Violation) []fault.Violation {
	for index, validation := range c.FinalValidations {
		if atCap(violations) {
			return violations
		}
		field := fmt.Sprintf("finalValidations.%d", index)
		// The observation's own fault is discarded rather than nested: its detail
		// belongs to this envelope's violations, and its text carries identities.
		if validation.Validate() != nil {
			violations = append(violations, mustViolation(fault.CodeFieldInvalid, field, "final validation is invalid"))
		}
		if !validation.Final || validation.StepExecutionID != c.Event.ID || validation.ExecutionID != c.Event.ExecutionID {
			violations = append(violations, mustViolation(fault.CodeFieldMismatch, field, "final validation must belong to the committed step and execution"))
		}
	}
	return violations
}

func (c StepTransitionCommit) appendHealObservationViolations(violations []fault.Violation) []fault.Violation {
	seenHealIDs := make(map[string]struct{}, len(c.HealObservations))
	for index, heal := range c.HealObservations {
		if atCap(violations) {
			return violations
		}
		field := func(name string) string { return fmt.Sprintf("healObservations.%d.%s", index, name) }
		if _, exists := seenHealIDs[heal.ID]; exists {
			violations = append(violations, mustViolation(fault.CodeFieldDuplicate, field("id"), "heal observation id is duplicated"))
		}
		seenHealIDs[heal.ID] = struct{}{}
		if heal.Validate() != nil {
			violations = append(violations, mustViolation(fault.CodeFieldInvalid, fmt.Sprintf("healObservations.%d", index), "heal observation is invalid"))
		}
		if heal.StepExecutionID != c.Event.ID || heal.ExecutionID != c.Event.ExecutionID {
			violations = append(violations, mustViolation(fault.CodeFieldMismatch, fmt.Sprintf("healObservations.%d", index), "heal observation must belong to the committed step and execution"))
		}
		if heal.Succeeded && c.Event.Phase != "SUCCEEDED" {
			violations = append(violations, mustViolation(fault.CodeFieldMismatch, field("succeeded"), "a succeeded heal observation requires a succeeded terminal step"))
		}
	}
	return violations
}

func (c StepTransitionCommit) appendSelectorResetViolations(violations []fault.Violation) []fault.Violation {
	type resetIdentity struct {
		ExecutionID       execution.EntryID
		StepExecutionID   execution.StepExecutionID
		ElementTargetID   string
		BaseNodeVersionID string
	}
	seenResets := make(map[resetIdentity]struct{}, len(c.OriginalSelectorResets))
	for index, reset := range c.OriginalSelectorResets {
		if atCap(violations) {
			return violations
		}
		field := fmt.Sprintf("originalSelectorResets.%d", index)
		identity := resetIdentity{reset.ExecutionID, reset.StepExecutionID, reset.ElementTargetID, reset.BaseNodeVersionID}
		if _, exists := seenResets[identity]; exists {
			violations = append(violations, mustViolation(fault.CodeFieldDuplicate, field, "heal candidate reset identity is duplicated"))
		}
		seenResets[identity] = struct{}{}
		if c.Event.Phase != "SUCCEEDED" {
			violations = append(violations, mustViolation(fault.CodeFieldMismatch, field, "an original selector reset requires a succeeded terminal step"))
		}
		if reset.ExecutionID != c.Event.ExecutionID || reset.StepExecutionID != c.Event.ID || reset.ElementTargetID == "" || reset.BaseNodeVersionID == "" || reset.ObservedAt <= 0 {
			violations = append(violations, mustViolation(fault.CodeFieldMismatch, field, "heal candidate reset must belong to the committed step and execution"))
		}
	}
	return violations
}

func appendGroupTopologyViolations(violations []fault.Violation, event StepPhaseEvent, validations []ValidationObservation, groups []ValidationGroupTerminalObservation) []fault.Violation {
	type memberKey struct {
		GroupID         string
		BranchID        string
		ElementTargetID string
	}
	members := make(map[memberKey]ValidationObservation, len(validations))
	seenValidationIDs := make(map[string]struct{}, len(validations))
	for index, validation := range validations {
		if atCap(violations) {
			return violations
		}
		if _, exists := seenValidationIDs[validation.ID]; exists {
			violations = append(violations, mustViolation(fault.CodeFieldDuplicate, fmt.Sprintf("finalValidations.%d.id", index), "final validation id is duplicated"))
		}
		seenValidationIDs[validation.ID] = struct{}{}
		if validation.GroupID == "" {
			continue
		}
		key := memberKey{GroupID: validation.GroupID, BranchID: validation.BranchID, ElementTargetID: validation.ElementTargetID}
		if _, exists := members[key]; exists {
			violations = append(violations, mustViolation(fault.CodeFieldDuplicate, fmt.Sprintf("finalValidations.%d.groupMembership", index), "final validation group membership is duplicated"))
			continue
		}
		members[key] = validation
	}
	seenGroups := make(map[string]struct{}, len(groups))
	seenGroupIDs := make(map[string]struct{}, len(groups))
	for index, group := range groups {
		if atCap(violations) {
			return violations
		}
		element := fmt.Sprintf("finalValidationGroups.%d", index)
		field := func(name string) string { return element + "." + name }
		// The group's own fault is discarded rather than nested: its text carries
		// identities, and its detail belongs to this envelope's violations.
		if group.Validate() != nil {
			violations = append(violations, mustViolation(fault.CodeFieldInvalid, element, "final validation group is invalid"))
		}
		if _, exists := seenGroupIDs[group.ID]; exists {
			violations = append(violations, mustViolation(fault.CodeFieldDuplicate, field("id"), "final validation group id is duplicated"))
		}
		seenGroupIDs[group.ID] = struct{}{}
		if group.StepExecutionID != event.ID || group.ExecutionID != event.ExecutionID {
			violations = append(violations, mustViolation(fault.CodeFieldMismatch, element, "final validation group must belong to the committed step and execution"))
		}
		if !validationTerminalMatchesPhase(group.TerminalReason, event.Phase) {
			violations = append(violations, mustViolation(fault.CodeFieldMismatch, field("terminalReason"), "final validation group terminal reason contradicts the step phase"))
		}
		if _, exists := seenGroups[group.GroupID]; exists {
			violations = append(violations, mustViolation(fault.CodeFieldDuplicate, field("groupId"), "final validation group is declared more than once"))
		}
		seenGroups[group.GroupID] = struct{}{}
		for memberIndex, expected := range group.ExpectedMembers() {
			if atCap(violations) {
				return violations
			}
			memberField := fmt.Sprintf("%s.expectedMembers.%d", element, memberIndex)
			key := memberKey{GroupID: group.GroupID, BranchID: expected.BranchID, ElementTargetID: expected.ElementTargetID}
			member, exists := members[key]
			if !exists {
				violations = append(violations, mustViolation(fault.CodeFieldMismatch, memberField, "expected group member has no final validation"))
				continue
			}
			if member.BranchDisposition == ValidationBranchWon && group.TerminalReason != ValidationTerminalPassed {
				violations = append(violations, mustViolation(fault.CodeFieldMismatch, memberField, "a group that did not pass cannot have a won member"))
			}
			if (member.BranchDisposition == ValidationBranchWon || member.BranchDisposition == ValidationBranchSatisfiedNotWinner) && !member.Passed {
				violations = append(violations, mustViolation(fault.CodeFieldMismatch, memberField, "a satisfied branch member must have passed"))
			}
			if group.TerminalReason == ValidationTerminalPassed {
				if expected.BranchID == group.WinningBranchID && member.BranchDisposition != ValidationBranchWon {
					violations = append(violations, mustViolation(fault.CodeFieldMismatch, memberField, "the winning branch member must be marked won"))
				}
				if expected.BranchID != group.WinningBranchID && member.BranchDisposition == ValidationBranchWon {
					violations = append(violations, mustViolation(fault.CodeFieldMismatch, memberField, "a non-winning branch member must not be marked won"))
				}
			}
			delete(members, key)
		}
	}
	// members has been drained of every member its group consumed, so whatever is
	// left is unaccounted for. Ranging over the map would report a random one of
	// the leftovers, which means the same commit could be rejected with different
	// detail on a different run. Walking the source slice instead makes the
	// reported failures a function of the input alone.
	for index, validation := range validations {
		if atCap(violations) {
			return violations
		}
		if validation.GroupID == "" {
			continue
		}
		key := memberKey{GroupID: validation.GroupID, BranchID: validation.BranchID, ElementTargetID: validation.ElementTargetID}
		if _, leftover := members[key]; !leftover {
			continue
		}
		delete(members, key)
		field := fmt.Sprintf("finalValidations.%d.groupMembership", index)
		if _, grouped := seenGroups[validation.GroupID]; grouped {
			violations = append(violations, mustViolation(fault.CodeFieldMismatch, field, "final validation is not an expected member of its group"))
			continue
		}
		violations = append(violations, mustViolation(fault.CodeFieldMismatch, field, "grouped final validation has no terminal group fact"))
	}
	return violations
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
