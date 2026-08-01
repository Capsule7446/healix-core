package scheduling

import (
	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

const CodeEntryStatesInvalid fault.Code = "EXECUTION_ENTRY_STATES_INVALID"

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

type SkipCause string

const (
	SkipCausePriorFailure      SkipCause = "PRIOR_FAILURE"
	SkipCausePriorCancellation SkipCause = "PRIOR_CANCELLATION"
	SkipCausePriorAbort        SkipCause = "PRIOR_ABORT"
)

type EntryState struct {
	ExecutionID string
	Status      execution.ExecutionStatus
	SkipCause   SkipCause
}

type ExecutionTransition struct {
	ExecutionID string
	From        execution.ExecutionStatus
	To          execution.ExecutionStatus
	Cause       SkipCause
}

type Decision struct {
	NextExecutionID string
	Transitions     []ExecutionTransition
	FinalStatus     *execution.InstanceStatus
}

// DecideAdvance makes a serial scheduling decision using the run snapshot as the
// sole authority for membership, order, and failure policy.
func DecideAdvance(snapshot execution.InstanceSnapshot, states []EntryState) (Decision, error) {
	if snapshot.Digest() == "" {
		return Decision{}, invalidEntryStatesError()
	}
	plan := snapshot.Plan()
	entries := plan.Entries
	if len(states) != len(entries) {
		return Decision{}, invalidEntryStatesError()
	}

	byID := make(map[string]EntryState, len(states))
	for _, state := range states {
		if _, exists := byID[state.ExecutionID]; exists {
			return Decision{}, invalidEntryStatesError()
		}
		byID[state.ExecutionID] = state
	}
	ordered := make([]EntryState, len(entries))
	for i, entry := range entries {
		state, exists := byID[entry.ExecutionID]
		if !exists {
			return Decision{}, invalidEntryStatesError()
		}
		ordered[i] = state
		delete(byID, entry.ExecutionID)
	}
	if len(byID) != 0 {
		return Decision{}, invalidEntryStatesError()
	}
	stopIndex, stopCause, finalStatus, err := validateSerialShape(ordered, plan.FailurePolicy)
	if err != nil {
		return Decision{}, err
	}
	if stopIndex >= 0 {
		return stopAfter(entries, ordered, stopIndex, stopCause, finalStatus), nil
	}

	failed := false
	for i, state := range ordered {
		switch state.Status {
		case execution.ExecutionRunning:
			return Decision{}, nil
		case execution.ExecutionPending:
			return Decision{NextExecutionID: entries[i].ExecutionID}, nil
		case execution.ExecutionFailed:
			failed = true
		}
	}
	status := execution.Succeeded
	if failed {
		status = execution.Failed
	}
	return Decision{FinalStatus: &status}, nil
}

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
		if status == execution.ExecutionSkipped {
			if state.SkipCause == "" {
				return -1, "", "", invalidEntryStatesError()
			}
		} else if state.SkipCause != "" {
			return -1, "", "", invalidEntryStatesError()
		}

		if stopIndex >= 0 {
			if status == execution.ExecutionPending {
				continue
			}
			if status != execution.ExecutionSkipped {
				return -1, "", "", invalidEntryStatesError()
			}
			if state.SkipCause != expectedCause {
				return -1, "", "", invalidEntryStatesError()
			}
			continue
		}
		if status == execution.ExecutionSkipped {
			return -1, "", "", invalidEntryStatesError()
		}
		if status == execution.ExecutionPending {
			frontierSeen = true
			continue
		}
		if status == execution.ExecutionRunning {
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

func stopFor(status execution.ExecutionStatus, policy execution.FailurePolicy) (SkipCause, execution.InstanceStatus) {
	switch status {
	case execution.ExecutionFailed:
		if policy == execution.FailurePolicyStopOnFailure {
			return SkipCausePriorFailure, execution.Failed
		}
	case execution.ExecutionCanceled:
		return SkipCausePriorCancellation, execution.Canceled
	case execution.ExecutionAborted:
		return SkipCausePriorAbort, execution.Aborted
	}
	return "", ""
}

func isKnownStatus(status execution.ExecutionStatus) bool {
	return status == execution.ExecutionPending || status == execution.ExecutionRunning || execution.IsTerminalExecutionStatus(status)
}

func stopAfter(entries []execution.Entry, states []EntryState, index int, cause SkipCause, status execution.InstanceStatus) Decision {
	transitions := make([]ExecutionTransition, 0, len(entries)-index-1)
	for i := index + 1; i < len(entries); i++ {
		if states[i].Status == execution.ExecutionPending {
			transitions = append(transitions, ExecutionTransition{ExecutionID: entries[i].ExecutionID, From: execution.ExecutionPending, To: execution.ExecutionSkipped, Cause: cause})
		}
	}
	return Decision{Transitions: transitions, FinalStatus: &status}
}
