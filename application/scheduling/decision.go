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
	EntryID   execution.EntryID
	Status    execution.EntryStatus
	SkipCause SkipCause
}

type ExecutionTransition struct {
	EntryID execution.EntryID
	From    execution.EntryStatus
	To      execution.EntryStatus
	Cause   SkipCause
}

type Decision struct {
	NextEntryID execution.EntryID
	Transitions []ExecutionTransition
	FinalStatus *execution.InstanceStatus
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
		return stopAfter(entries, ordered, stopIndex, stopCause, finalStatus), nil
	}

	failed := false
	for i, state := range ordered {
		switch state.Status {
		case execution.EntryRunning:
			return Decision{}, nil
		case execution.EntryPending:
			return Decision{NextEntryID: entries[i].ID}, nil
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

func isKnownStatus(status execution.EntryStatus) bool {
	return status == execution.EntryPending || status == execution.EntryRunning || execution.IsTerminalEntryStatus(status)
}

func stopAfter(entries []execution.Entry, states []EntryState, index int, cause SkipCause, status execution.InstanceStatus) Decision {
	transitions := make([]ExecutionTransition, 0, len(entries)-index-1)
	for i := index + 1; i < len(entries); i++ {
		if states[i].Status == execution.EntryPending {
			transitions = append(transitions, ExecutionTransition{EntryID: entries[i].ID, From: execution.EntryPending, To: execution.EntrySkipped, Cause: cause})
		}
	}
	return Decision{Transitions: transitions, FinalStatus: &status}
}
