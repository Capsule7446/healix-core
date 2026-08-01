package execution

import "github.com/Capsule7446/healix-core/domain/fault"

// EntryStatus describes the lifecycle of one workflow execution. It is
// intentionally distinct from InstanceStatus, which describes the containing instance.
type EntryStatus string

const (
	EntryPending   EntryStatus = "PENDING"
	EntryRunning   EntryStatus = "RUNNING"
	EntrySucceeded EntryStatus = "SUCCEEDED"
	EntryFailed    EntryStatus = "FAILED"
	EntryCanceled  EntryStatus = "CANCELED"
	EntryAborted   EntryStatus = "ABORTED"
	EntrySkipped   EntryStatus = "SKIPPED"
)

func ValidateEntryStatusTransition(from, to EntryStatus) error {
	return from.CanTransitionTo(to)
}

func IsTerminalEntryStatus(status EntryStatus) bool {
	switch status {
	case EntrySucceeded, EntryFailed, EntryCanceled, EntryAborted, EntrySkipped:
		return true
	default:
		return false
	}
}

func (from EntryStatus) CanTransitionTo(to EntryStatus) error {
	allowed := (from == EntryPending && (to == EntryRunning || to == EntryFailed || to == EntryCanceled || to == EntrySkipped)) ||
		(from == EntryRunning && (to == EntrySucceeded || to == EntryFailed || to == EntryCanceled || to == EntryAborted))
	if allowed {
		return nil
	}
	return mustExecutionFault(fault.FailedPrecondition, CodeStatusTransitionInvalid, "execution status transition is invalid")
}
