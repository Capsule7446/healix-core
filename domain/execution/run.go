// Package execution owns the lifecycle of a submitted test-task run.
package execution

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fault"
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

const sha256DigestLength = 71

func ValidateRunStatusTransition(from, to RunStatus) error {
	allowed := (from == Queued && (to == Running || to == Canceled)) ||
		(from == Running && (to == Succeeded || to == Failed || to == Canceled || to == Aborted))
	if allowed {
		return nil
	}
	return mustExecutionFault(fault.FailedPrecondition, CodeInstanceStatusTransitionInvalid, "instance status transition is invalid")
}

type Run struct {
	ID                    string
	ExecutionFlowID       string
	TestTaskVersionID     string
	SnapshotSchemaVersion RunSnapshotSchema
	SnapshotDigest        string
	Status                RunStatus
	EnvironmentID         string
	QueuePosition         int
	CreatedAt             int64
	QueuedAt              int64
	StartedAt             int64
	FinishedAt            int64
	sealedSnapshotDigest  string
}

func NewRun(run Run, snapshot RunSnapshot) (Run, error) {
	if snapshot.digest == "" || run.ID != snapshot.RunID() || run.ExecutionFlowID != snapshot.ExecutionFlowID() || run.TestTaskVersionID != snapshot.TestTaskVersionID() {
		return Run{}, errors.New("run identity must match sealed snapshot")
	}
	if run.Status != Queued || run.CreatedAt <= 0 || run.QueuedAt != run.CreatedAt || run.StartedAt != 0 || run.FinishedAt != 0 || run.QueuePosition < 0 {
		return Run{}, errors.New("new run must have a valid queued lifecycle shape")
	}
	run.SnapshotSchemaVersion = snapshot.SchemaVersion()
	run.SnapshotDigest = snapshot.Digest()
	run.sealedSnapshotDigest = snapshot.Digest()
	return run, nil
}

// validateRunLifecycleShape validates only persisted lifecycle fields and has no
// dependency on snapshot hydration or transition intent.
func validateRunLifecycleShape(run Run) error {
	if run.CreatedAt <= 0 || run.QueuedAt < run.CreatedAt || run.QueuePosition < 0 {
		return errors.New("run lifecycle timestamps or queue position are invalid")
	}
	valid := false
	switch run.Status {
	case Queued:
		valid = run.StartedAt == 0 && run.FinishedAt == 0
	case Running:
		valid = run.StartedAt >= run.QueuedAt && run.FinishedAt == 0
	case Succeeded, Failed, Aborted:
		valid = run.StartedAt >= run.QueuedAt && run.FinishedAt >= run.StartedAt
	case Canceled:
		valid = run.FinishedAt >= run.QueuedAt && (run.StartedAt == 0 || (run.StartedAt >= run.QueuedAt && run.FinishedAt >= run.StartedAt))
	default:
		return errors.New("run status is invalid")
	}
	if !valid {
		return errors.New("run lifecycle does not match status")
	}
	return nil
}

// HydrateRun restores the private snapshot identity seal after durable storage.
func HydrateRun(run Run, snapshot RunSnapshot) (Run, error) {
	if snapshot.digest == "" || run.ID != snapshot.RunID() || run.ExecutionFlowID != snapshot.ExecutionFlowID() || run.TestTaskVersionID != snapshot.TestTaskVersionID() || run.SnapshotSchemaVersion != snapshot.SchemaVersion() || run.SnapshotDigest != snapshot.Digest() {
		return Run{}, errors.New("persisted run identity must match hydrated snapshot")
	}
	if strings.TrimSpace(run.TestTaskVersionID) == "" {
		return Run{}, errors.New("persisted run lifecycle is invalid")
	}
	if err := validateRunLifecycleShape(run); err != nil {
		return Run{}, fmt.Errorf("persisted run lifecycle is invalid: %w", err)
	}
	run.sealedSnapshotDigest = snapshot.Digest()
	return run, nil
}

// ValidateRun verifies a Run returned across an application adapter boundary.
// It is pure and validates identity, lifecycle shape, and the private snapshot
// seal when snapshot identity is carried by the Run.
func isSupportedRunSnapshotSchema(version RunSnapshotSchema) bool {
	return version == RunSnapshotSchemaV1 || version == RunSnapshotSchemaV2
}

func ValidateRun(run Run) error {
	if strings.TrimSpace(run.ID) == "" || strings.TrimSpace(run.ExecutionFlowID) == "" || strings.TrimSpace(run.TestTaskVersionID) == "" || strings.TrimSpace(run.EnvironmentID) == "" {
		return errors.New("run identity is incomplete")
	}
	if !isSupportedRunSnapshotSchema(run.SnapshotSchemaVersion) || len(run.SnapshotDigest) != sha256DigestLength || !strings.HasPrefix(run.SnapshotDigest, "sha256:") || run.SnapshotDigest != run.sealedSnapshotDigest {
		return errors.New("run snapshot identity is invalid")
	}
	return validateRunLifecycleShape(run)
}

func (r Run) Transition(to RunStatus, at int64) (Run, error) {
	if !isSupportedRunSnapshotSchema(r.SnapshotSchemaVersion) || len(r.SnapshotDigest) != sha256DigestLength || !strings.HasPrefix(r.SnapshotDigest, "sha256:") || r.SnapshotDigest != r.sealedSnapshotDigest || strings.TrimSpace(r.TestTaskVersionID) == "" {
		return Run{}, errors.New("run snapshot identity is invalid")
	}
	if err := validateRunLifecycleShape(r); err != nil {
		return Run{}, fmt.Errorf("run source lifecycle is invalid: %w", err)
	}
	if err := ValidateRunStatusTransition(r.Status, to); err != nil {
		return Run{}, err
	}
	prior := r.QueuedAt
	if r.StartedAt > prior {
		prior = r.StartedAt
	}
	if at < prior {
		return Run{}, errors.New("run transition timestamp is not monotonic")
	}
	next := r
	next.Status = to
	if to == Running {
		next.StartedAt = at
	} else {
		next.FinishedAt = at
	}
	if err := validateRunLifecycleShape(next); err != nil {
		return Run{}, fmt.Errorf("run transition result lifecycle is invalid: %w", err)
	}
	return next, nil
}
