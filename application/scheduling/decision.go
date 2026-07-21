package scheduling

import (
	"errors"
	"fmt"

	"github.com/Capsule7446/healix-core/domain/execution"
)

var ErrInvalidEntryStates = errors.New("invalid ordered execution states")

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
	FinalStatus     *execution.RunStatus
}

// DecideAdvance makes a serial scheduling decision using the sealed plan as the
// sole authority for membership, order, and failure policy.
func DecideAdvance(plan execution.Plan, states []EntryState) (Decision, error) {
	if err := plan.Validate(); err != nil {
		return Decision{}, fmt.Errorf("%w: %w", ErrInvalidEntryStates, err)
	}
	entries := plan.Entries()
	if len(states) != len(entries) {
		return Decision{}, fmt.Errorf("%w: state count %d does not match plan count %d", ErrInvalidEntryStates, len(states), len(entries))
	}

	byID := make(map[string]EntryState, len(states))
	for _, state := range states {
		if _, exists := byID[state.ExecutionID]; exists {
			return Decision{}, fmt.Errorf("%w: duplicate execution identity %q", ErrInvalidEntryStates, state.ExecutionID)
		}
		byID[state.ExecutionID] = state
	}
	ordered := make([]EntryState, len(entries))
	for i, entry := range entries {
		state, exists := byID[entry.ExecutionID]
		if !exists {
			return Decision{}, fmt.Errorf("%w: missing execution identity %q", ErrInvalidEntryStates, entry.ExecutionID)
		}
		ordered[i] = state
		delete(byID, entry.ExecutionID)
	}
	if len(byID) != 0 {
		return Decision{}, fmt.Errorf("%w: state contains identity outside plan", ErrInvalidEntryStates)
	}
	stopIndex, stopCause, finalStatus, err := validateSerialShape(ordered, plan.FailurePolicy())
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

func validateSerialShape(states []EntryState, policy execution.FailurePolicy) (int, SkipCause, execution.RunStatus, error) {
	frontierSeen := false
	stopIndex := -1
	var expectedCause SkipCause
	var finalStatus execution.RunStatus
	for i, state := range states {
		status := state.Status
		if !isKnownStatus(status) {
			return -1, "", "", fmt.Errorf("%w: unknown status %q", ErrInvalidEntryStates, status)
		}
		if status == execution.ExecutionSkipped {
			if state.SkipCause == "" {
				return -1, "", "", fmt.Errorf("%w: skipped execution at position %d requires a cause", ErrInvalidEntryStates, i+1)
			}
		} else if state.SkipCause != "" {
			return -1, "", "", fmt.Errorf("%w: non-skipped execution at position %d has skip cause %q", ErrInvalidEntryStates, i+1, state.SkipCause)
		}

		if stopIndex >= 0 {
			if status == execution.ExecutionPending {
				continue
			}
			if status != execution.ExecutionSkipped {
				return -1, "", "", fmt.Errorf("%w: status %q at position %d follows stop-causing status", ErrInvalidEntryStates, status, i+1)
			}
			if state.SkipCause != expectedCause {
				return -1, "", "", fmt.Errorf("%w: skip cause %q at position %d, expected %q", ErrInvalidEntryStates, state.SkipCause, i+1, expectedCause)
			}
			continue
		}
		if status == execution.ExecutionSkipped {
			return -1, "", "", fmt.Errorf("%w: skipped execution at position %d has no causal predecessor", ErrInvalidEntryStates, i+1)
		}
		if status == execution.ExecutionPending {
			frontierSeen = true
			continue
		}
		if status == execution.ExecutionRunning {
			if frontierSeen {
				return -1, "", "", fmt.Errorf("%w: running execution at position %d follows active frontier", ErrInvalidEntryStates, i+1)
			}
			frontierSeen = true
			continue
		}
		if frontierSeen {
			return -1, "", "", fmt.Errorf("%w: terminal execution at position %d follows active frontier", ErrInvalidEntryStates, i+1)
		}
		expectedCause, finalStatus = stopFor(status, policy)
		if expectedCause != "" {
			stopIndex = i
		}
	}
	return stopIndex, expectedCause, finalStatus, nil
}

func stopFor(status execution.ExecutionStatus, policy execution.FailurePolicy) (SkipCause, execution.RunStatus) {
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

func stopAfter(entries []execution.WorkflowEntry, states []EntryState, index int, cause SkipCause, status execution.RunStatus) Decision {
	transitions := make([]ExecutionTransition, 0, len(entries)-index-1)
	for i := index + 1; i < len(entries); i++ {
		if states[i].Status == execution.ExecutionPending {
			transitions = append(transitions, ExecutionTransition{ExecutionID: entries[i].ExecutionID, From: execution.ExecutionPending, To: execution.ExecutionSkipped, Cause: cause})
		}
	}
	return Decision{Transitions: transitions, FinalStatus: &status}
}
