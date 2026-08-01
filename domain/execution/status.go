package execution

import "github.com/Capsule7446/healix-core/domain/fault"

// ExecutionStatus describes the lifecycle of one workflow execution. It is
// intentionally distinct from InstanceStatus, which describes the containing instance.
type ExecutionStatus string

const (
	ExecutionPending   ExecutionStatus = "PENDING"
	ExecutionRunning   ExecutionStatus = "RUNNING"
	ExecutionSucceeded ExecutionStatus = "SUCCEEDED"
	ExecutionFailed    ExecutionStatus = "FAILED"
	ExecutionCanceled  ExecutionStatus = "CANCELED"
	ExecutionAborted   ExecutionStatus = "ABORTED"
	ExecutionSkipped   ExecutionStatus = "SKIPPED"
)

func ValidateExecutionStatusTransition(from, to ExecutionStatus) error {
	return from.CanTransitionTo(to)
}

func IsTerminalExecutionStatus(status ExecutionStatus) bool {
	switch status {
	case ExecutionSucceeded, ExecutionFailed, ExecutionCanceled, ExecutionAborted, ExecutionSkipped:
		return true
	default:
		return false
	}
}

func (from ExecutionStatus) CanTransitionTo(to ExecutionStatus) error {
	allowed := (from == ExecutionPending && (to == ExecutionRunning || to == ExecutionFailed || to == ExecutionCanceled || to == ExecutionSkipped)) ||
		(from == ExecutionRunning && (to == ExecutionSucceeded || to == ExecutionFailed || to == ExecutionCanceled || to == ExecutionAborted))
	if allowed {
		return nil
	}
	return mustExecutionFault(fault.FailedPrecondition, CodeStatusTransitionInvalid, "execution status transition is invalid")
}
