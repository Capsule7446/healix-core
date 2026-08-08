package scheduling

import (
	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

// CodeEntryStatesInvalid 表示入口状态集合无法与实例计划或串行状态机对齐。
const CodeEntryStatesInvalid fault.Code = "EXECUTION_ENTRY_STATES_INVALID"

// invalidEntryStatesError 构造入口状态校验失败的前置条件错误。
func invalidEntryStatesError() error {
	err, constructionErr := fault.New(
		fault.FailedPrecondition,
		CodeEntryStatesInvalid,
		"execution entry states are invalid",
	)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// SkipCause 说明入口因前序终止状态而被跳过的原因。
type SkipCause string

const (
	// SkipCausePriorFailure 表示前序入口失败后按停止策略跳过。
	SkipCausePriorFailure SkipCause = "PRIOR_FAILURE"
	// SkipCausePriorCancellation 表示前序入口取消后跳过。
	SkipCausePriorCancellation SkipCause = "PRIOR_CANCELLATION"
	// SkipCausePriorAbort 表示前序入口中止后跳过。
	SkipCausePriorAbort SkipCause = "PRIOR_ABORT"
)

// EntryState 是决策时按入口 ID 提供的当前状态及跳过原因。
type EntryState struct {
	EntryID   execution.EntryID
	Status    execution.EntryStatus
	SkipCause SkipCause
}

// ExecutionTransition 描述一次入口状态迁移及其跳过原因。
type ExecutionTransition struct {
	EntryID execution.EntryID
	From    execution.EntryStatus
	To      execution.EntryStatus
	Cause   SkipCause
}

// Decision 是串行调度器对下一入口或实例最终状态的决定。
type Decision struct {
	NextEntryID execution.EntryID
	Transitions []ExecutionTransition
	FinalStatus *execution.InstanceStatus
}

// DecideAdvance 使用运行快照作为成员关系、顺序和失败策略的唯一权威，作出串行调度决定。
func DecideAdvance(snapshot execution.InstanceSnapshot, states []EntryState) (Decision, error) {
	if snapshot.Digest() == "" {
		return Decision{}, invalidEntryStatesError()
	}
	plan := snapshot.Plan()
	entries := plan.Entries
	if len(states) != len(entries) {
		return Decision{}, invalidEntryStatesError()
	}

	byID := make(map[execution.EntryID]EntryState, len(states))
	for _, state := range states {
		if _, exists := byID[state.EntryID]; exists {
			return Decision{}, invalidEntryStatesError()
		}
		byID[state.EntryID] = state
	}
	ordered := make([]EntryState, len(entries))
	for i, entry := range entries {
		state, exists := byID[entry.ID]
		if !exists {
			return Decision{}, invalidEntryStatesError()
		}
		ordered[i] = state
		delete(byID, entry.ID)
	}
	if len(byID) != 0 {
		return Decision{}, invalidEntryStatesError()
	}
	stopIndex, stopCause, finalStatus, err := validateSerialShape(ordered, plan.FailurePolicy)
	if err != nil {
		return Decision{}, err
	}
	if stopIndex >= 0 {
		decision, err := stopAfter(entries, ordered, stopIndex, stopCause, finalStatus)
		if err != nil {
			return Decision{}, err
		}
		return decision, nil
	}

	failed := false
	for i, state := range ordered {
		switch state.Status {
		case execution.EntryRunning:
			return Decision{}, nil
		case execution.EntryPending:
			transition := ExecutionTransition{
				EntryID: entries[i].ID,
				From:    execution.EntryPending,
				To:      execution.EntryRunning,
			}
			if err := execution.ValidateEntryStatusTransition(transition.From, transition.To); err != nil {
				return Decision{}, err
			}
			return Decision{NextEntryID: entries[i].ID, Transitions: []ExecutionTransition{transition}}, nil
		case execution.EntryFailed:
			failed = true
		}
	}
	status := execution.Succeeded
	if failed {
		status = execution.Failed
	}
	return Decision{FinalStatus: &status}, nil
}

// validateSerialShape 校验入口状态是否符合串行执行形状，并找出应停止的位置、原因和最终状态。
func validateSerialShape(states []EntryState, policy execution.FailurePolicy) (int, SkipCause, execution.InstanceStatus, error) {
	frontierSeen := false
	stopIndex := -1
	var expectedCause SkipCause
	var finalStatus execution.InstanceStatus
	for i, state := range states {
		status := state.Status
		if !isKnownStatus(status) {
			return -1, "", "", invalidEntryStatesError()
		}
		if status == execution.EntrySkipped {
			if state.SkipCause == "" {
				return -1, "", "", invalidEntryStatesError()
			}
		} else if state.SkipCause != "" {
			return -1, "", "", invalidEntryStatesError()
		}

		if stopIndex >= 0 {
			if status == execution.EntryPending {
				continue
			}
			if status != execution.EntrySkipped {
				return -1, "", "", invalidEntryStatesError()
			}
			if state.SkipCause != expectedCause {
				return -1, "", "", invalidEntryStatesError()
			}
			continue
		}
		if status == execution.EntrySkipped {
			return -1, "", "", invalidEntryStatesError()
		}
		if status == execution.EntryPending {
			frontierSeen = true
			continue
		}
		if status == execution.EntryRunning {
			if frontierSeen {
				return -1, "", "", invalidEntryStatesError()
			}
			frontierSeen = true
			continue
		}
		if frontierSeen {
			return -1, "", "", invalidEntryStatesError()
		}
		expectedCause, finalStatus = stopFor(status, policy)
		if expectedCause != "" {
			stopIndex = i
		}
	}
	return stopIndex, expectedCause, finalStatus, nil
}

// stopFor 将终止入口状态按失败策略映射为后续跳过原因和实例最终状态。
func stopFor(status execution.EntryStatus, policy execution.FailurePolicy) (SkipCause, execution.InstanceStatus) {
	switch status {
	case execution.EntryFailed:
		if policy == execution.FailurePolicyStopOnFailure {
			return SkipCausePriorFailure, execution.Failed
		}
	case execution.EntryCanceled:
		return SkipCausePriorCancellation, execution.Canceled
	case execution.EntryAborted:
		return SkipCausePriorAbort, execution.Aborted
	}
	return "", ""
}

// isKnownStatus 判断入口状态是否属于调度器支持的状态集合。
func isKnownStatus(status execution.EntryStatus) bool {
	return status == execution.EntryPending || status == execution.EntryRunning || execution.IsTerminalEntryStatus(status)
}

// stopAfter 为停止入口之后仍处于待处理状态的入口生成跳过迁移，并设置实例最终状态。
func stopAfter(entries []execution.Entry, states []EntryState, index int, cause SkipCause, status execution.InstanceStatus) (Decision, error) {
	transitions := make([]ExecutionTransition, 0, len(entries)-index-1)
	for i := index + 1; i < len(entries); i++ {
		if states[i].Status == execution.EntryPending {
			transition := ExecutionTransition{EntryID: entries[i].ID, From: execution.EntryPending, To: execution.EntrySkipped, Cause: cause}
			if err := execution.ValidateEntryStatusTransition(transition.From, transition.To); err != nil {
				return Decision{}, invalidEntryStatesError()
			}
			transitions = append(transitions, transition)
		}
	}
	return Decision{Transitions: transitions, FinalStatus: &status}, nil
}
