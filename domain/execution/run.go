// Package execution owns the lifecycle of a submitted test-task run.
package execution

import (
	"errors"
	"fmt"
)

type RunStatus string

const (
	Queued    RunStatus = "QUEUED"
	Running   RunStatus = "RUNNING"
	Succeeded RunStatus = "SUCCEEDED"
	Failed    RunStatus = "FAILED"
	Canceled  RunStatus = "CANCELED"
	Aborted   RunStatus = "ABORTED"
)

var ErrInvalidRunStatusTransition = errors.New("invalid run status transition")

func ValidateRunStatusTransition(from, to RunStatus) error {
	allowed := (from == Queued && (to == Running || to == Canceled)) ||
		(from == Running && (to == Succeeded || to == Failed || to == Aborted))
	if allowed {
		return nil
	}
	return fmt.Errorf("%w: %s -> %s", ErrInvalidRunStatusTransition, from, to)
}

type Run struct {
	ID            string
	TestTaskID    string
	Status        RunStatus
	EnvironmentID string
	QueuePosition int
	CreatedAt     int64
	QueuedAt      int64
	StartedAt     int64
	FinishedAt    int64
}

func (r Run) Transition(to RunStatus) (Run, error) {
	if err := ValidateRunStatusTransition(r.Status, to); err != nil {
		return Run{}, err
	}
	next := r
	next.Status = to
	return next, nil
}
