package evidence

import (
	"fmt"
	"strings"

	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

// HealCandidateReset 记录自愈成功后重置原始选择器所需的步骤、元素目标和节点版本身份。
type HealCandidateReset struct {
	EntryID           execution.EntryID
	StepExecutionID   execution.StepExecutionID
	Occurrence        int
	ElementTargetID   string
	BaseNodeVersionID string
	ObservedAt        int64
}

// StepRevision 是步骤事实提交的单调修订号。
type StepRevision uint64

// maxStepTransitionFacts 限制一次步骤迁移提交中各事实集合及总事实数量。
const maxStepTransitionFacts = 10_000

// StepTransitionCommit 携带一次原子步骤迁移提交的事件、终态观测、自愈观测和选择器重置。
type StepTransitionCommit struct {
	CommitID               string
	ExpectedRevision       StepRevision
	Event                  StepPhaseEvent
	FinalValidations       []ValidationObservation
	FinalValidationGroups  []ValidationGroupTerminalObservation
	HealObservations       []HealObservation
	OriginalSelectorResets []HealCandidateReset
}

// Validate 通过一个携带有序违规项的聚合封套报告全部失败。集合索引从 0 开始并对应调用方
// 传入的切片；提交、执行、步骤、校验、自愈、分组或元素目标身份均不会进入公开文本。
func (c StepTransitionCommit) Validate() error {
	// 事实上限必须先校验，因为它同时限制下方违规遍历的工作量；超过上限的提交直接返回
	// 自身错误码，不与字段违规共享封套。
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
	if c.Event.ID.Validate() != nil || c.Event.EntryID.Validate() != nil || c.Event.FlowFragmentStepID == "" || c.Event.DisplayName == "" || c.Event.Kind == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "event.identity", "event identity is required"))
	}
	if !isTerminalPhase(c.Event.Phase) {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "event.phase", "event phase must be terminal"))
	}
	violations = appendOccurrenceViolations(violations, c.Event.Occurrence, "event")
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

// exceedsFactLimit 判断任一事实集合或事实总数是否超过提交上限。
func (c StepTransitionCommit) exceedsFactLimit() bool {
	return len(c.FinalValidations) > maxStepTransitionFacts ||
		len(c.FinalValidationGroups) > maxStepTransitionFacts ||
		len(c.HealObservations) > maxStepTransitionFacts ||
		len(c.OriginalSelectorResets) > maxStepTransitionFacts ||
		len(c.FinalValidations)+len(c.FinalValidationGroups)+len(c.HealObservations)+len(c.OriginalSelectorResets) > maxStepTransitionFacts
}

// isTerminalPhase 判断字符串是否属于终止阶段集合。
func isTerminalPhase(phase string) bool {
	switch phase {
	case "SUCCEEDED", "FAILED", "CANCELED", "ABORTED":
		return true
	default:
		return false
	}
}

// appendFinalValidationViolations 校验终态校验观测，并检查其是否属于已提交步骤和执行。
func (c StepTransitionCommit) appendFinalValidationViolations(violations []fault.Violation) []fault.Violation {
	for index, validation := range c.FinalValidations {
		if atCap(violations) {
			return violations
		}
		field := fmt.Sprintf("finalValidations.%d", index)
		// 观测自身错误不会嵌套；其细节应归入当前封套的违规项，且其文本可能携带身份信息。
		if validation.Validate() != nil {
			violations = append(violations, mustViolation(fault.CodeFieldInvalid, field, "final validation is invalid"))
		}
		if !validation.Final || validation.StepExecutionID != c.Event.ID || validation.EntryID != c.Event.EntryID || validation.Occurrence != c.Event.Occurrence {
			violations = append(violations, mustViolation(fault.CodeFieldMismatch, field, "final validation must belong to the committed step and execution"))
		}
	}
	return violations
}

// appendHealObservationViolations 校验自愈观测的唯一性、身份归属和成功阶段约束。
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
		if heal.StepExecutionID != c.Event.ID || heal.EntryID != c.Event.EntryID || heal.Occurrence != c.Event.Occurrence {
			violations = append(violations, mustViolation(fault.CodeFieldMismatch, fmt.Sprintf("healObservations.%d", index), "heal observation must belong to the committed step and execution"))
		}
		if heal.Succeeded && c.Event.Phase != "SUCCEEDED" {
			violations = append(violations, mustViolation(fault.CodeFieldMismatch, field("succeeded"), "a succeeded heal observation requires a succeeded terminal step"))
		}
	}
	return violations
}

// appendSelectorResetViolations 校验原始选择器重置的唯一性、阶段和步骤身份归属。
func (c StepTransitionCommit) appendSelectorResetViolations(violations []fault.Violation) []fault.Violation {
	type resetIdentity struct {
		EntryID           execution.EntryID
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
		identity := resetIdentity{reset.EntryID, reset.StepExecutionID, reset.ElementTargetID, reset.BaseNodeVersionID}
		if _, exists := seenResets[identity]; exists {
			violations = append(violations, mustViolation(fault.CodeFieldDuplicate, field, "heal candidate reset identity is duplicated"))
		}
		seenResets[identity] = struct{}{}
		if c.Event.Phase != "SUCCEEDED" {
			violations = append(violations, mustViolation(fault.CodeFieldMismatch, field, "an original selector reset requires a succeeded terminal step"))
		}
		if reset.EntryID != c.Event.EntryID || reset.StepExecutionID != c.Event.ID || reset.Occurrence != c.Event.Occurrence || reset.ElementTargetID == "" || reset.BaseNodeVersionID == "" || reset.ObservedAt <= 0 {
			violations = append(violations, mustViolation(fault.CodeFieldMismatch, field, "heal candidate reset must belong to the committed step and execution"))
		}
	}
	return violations
}

// appendGroupTopologyViolations 校验终态校验与校验组之间的成员拓扑、唯一性和阶段一致性。
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
		// 校验组自身错误不会嵌套；其文本可能携带身份信息，细节应归入当前封套的违规项。
		if group.Validate() != nil {
			violations = append(violations, mustViolation(fault.CodeFieldInvalid, element, "final validation group is invalid"))
		}
		if _, exists := seenGroupIDs[group.ID]; exists {
			violations = append(violations, mustViolation(fault.CodeFieldDuplicate, field("id"), "final validation group id is duplicated"))
		}
		seenGroupIDs[group.ID] = struct{}{}
		if group.StepExecutionID != event.ID || group.EntryID != event.EntryID || group.Occurrence != event.Occurrence {
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
	// members 已移除每个分组消费的成员，剩余项即为未被分组记录的成员。按源切片遍历
	// 剩余成员，确保同一提交的违规报告只由输入决定，而不受映射遍历顺序影响。
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

// validationTerminalMatchesPhase 判断校验组终止原因是否与步骤终止阶段一致。
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

// NodeVersionPromotion 描述提交后需要提升为正式版本的元素目标节点版本。
type NodeVersionPromotion struct {
	ElementTargetID string
	VersionID       string
}

// StepTransitionCommitResult 保存提交后的修订号、应用标记和节点版本提升列表。
type StepTransitionCommitResult struct {
	Revision   StepRevision
	WasApplied bool
	Promotions []NodeVersionPromotion
}
