package execution

import (
	"errors"
	"fmt"
)

// ExecutionStatus describes the lifecycle of one workflow execution. It is
// intentionally distinct from RunStatus, which describes the containing run.
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

var ErrInvalidExecutionStatusTransition = errors.New("invalid execution status transition")

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
	return fmt.Errorf("%w: %s -> %s", ErrInvalidExecutionStatusTransition, from, to)
}
