// Package execution owns the lifecycle of a submitted test-task instance.
package execution

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fault"
)

type InstanceStatus string

const (
	Queued    InstanceStatus = "QUEUED"
	Running   InstanceStatus = "RUNNING"
	Succeeded InstanceStatus = "SUCCEEDED"
	Failed    InstanceStatus = "FAILED"
	Canceled  InstanceStatus = "CANCELED"
	Aborted   InstanceStatus = "ABORTED"
)

const sha256DigestLength = 71

func ValidateInstanceStatusTransition(from, to InstanceStatus) error {
	allowed := (from == Queued && (to == Running || to == Canceled)) ||
		(from == Running && (to == Succeeded || to == Failed || to == Canceled || to == Aborted))
	if allowed {
		return nil
	}
	return mustExecutionFault(fault.FailedPrecondition, CodeInstanceStatusTransitionInvalid, "instance status transition is invalid")
}

type Instance struct {
	ID                    InstanceID
	ExecutionFlowID       string
	TestTaskVersionID     string
	SnapshotSchemaVersion InstanceSnapshotSchema
	SnapshotDigest        string
	Status                InstanceStatus
	EnvironmentID         string
	QueuePosition         int
	CreatedAt             int64
	QueuedAt              int64
	StartedAt             int64
	FinishedAt            int64
	sealedSnapshotDigest  string
}

// NewInstance classifies its own validation failure at this exported boundary: an
// uncoded identity or lifecycle-shape defect becomes
// EXECUTION_CREATE_INSTANCE_SNAPSHOT_INVALID, with the bare detail retained
// only on the private cause.
func NewInstance(instance Instance, snapshot InstanceSnapshot) (Instance, error) {
	sealed, err := newInstance(instance, snapshot)
	if err != nil {
		return Instance{}, classifyCreateInstanceSnapshot(err)
	}
	return sealed, nil
}

func newInstance(instance Instance, snapshot InstanceSnapshot) (Instance, error) {
	if snapshot.digest == "" || instance.ID != snapshot.InstanceID() || instance.ExecutionFlowID != snapshot.ExecutionFlowID() || instance.TestTaskVersionID != snapshot.TestTaskVersionID() {
		return Instance{}, errors.New("instance identity must match sealed snapshot")
	}
	if instance.Status != Queued || instance.CreatedAt <= 0 || instance.QueuedAt != instance.CreatedAt || instance.StartedAt != 0 || instance.FinishedAt != 0 || instance.QueuePosition < 0 {
		return Instance{}, errors.New("new instance must have a valid queued lifecycle shape")
	}
	instance.SnapshotSchemaVersion = snapshot.SchemaVersion()
	instance.SnapshotDigest = snapshot.Digest()
	instance.sealedSnapshotDigest = snapshot.Digest()
	return instance, nil
}

// validateInstanceLifecycleShape validates only persisted lifecycle fields and has no
// dependency on snapshot hydration or transition intent.
func validateInstanceLifecycleShape(instance Instance) error {
	if instance.CreatedAt <= 0 || instance.QueuedAt < instance.CreatedAt || instance.QueuePosition < 0 {
		return errors.New("instance lifecycle timestamps or queue position are invalid")
	}
	valid := false
	switch instance.Status {
	case Queued:
		valid = instance.StartedAt == 0 && instance.FinishedAt == 0
	case Running:
		valid = instance.StartedAt >= instance.QueuedAt && instance.FinishedAt == 0
	case Succeeded, Failed, Aborted:
		valid = instance.StartedAt >= instance.QueuedAt && instance.FinishedAt >= instance.StartedAt
	case Canceled:
		valid = instance.FinishedAt >= instance.QueuedAt && (instance.StartedAt == 0 || (instance.StartedAt >= instance.QueuedAt && instance.FinishedAt >= instance.StartedAt))
	default:
		return errors.New("instance status is invalid")
	}
	if !valid {
		return errors.New("instance lifecycle does not match status")
	}
	return nil
}

// HydrateInstance restores the private snapshot identity seal after durable storage.
func HydrateInstance(instance Instance, snapshot InstanceSnapshot) (Instance, error) {
	if snapshot.digest == "" || instance.ID != snapshot.InstanceID() || instance.ExecutionFlowID != snapshot.ExecutionFlowID() || instance.TestTaskVersionID != snapshot.TestTaskVersionID() || instance.SnapshotSchemaVersion != snapshot.SchemaVersion() || instance.SnapshotDigest != snapshot.Digest() {
		return Instance{}, errors.New("persisted instance identity must match hydrated snapshot")
	}
	if strings.TrimSpace(instance.TestTaskVersionID) == "" {
		return Instance{}, errors.New("persisted instance lifecycle is invalid")
	}
	if err := validateInstanceLifecycleShape(instance); err != nil {
		return Instance{}, fmt.Errorf("persisted instance lifecycle is invalid: %w", err)
	}
	instance.sealedSnapshotDigest = snapshot.Digest()
	return instance, nil
}

// ValidateInstance verifies a Instance returned across an application adapter boundary.
// It is pure and validates identity, lifecycle shape, and the private snapshot
// seal when snapshot identity is carried by the Instance.
func isSupportedInstanceSnapshotSchema(version InstanceSnapshotSchema) bool {
	return version == RunSnapshotSchemaV1 || version == RunSnapshotSchemaV2
}

func ValidateInstance(instance Instance) error {
	if instance.ID.Validate() != nil || strings.TrimSpace(instance.ExecutionFlowID) == "" || strings.TrimSpace(instance.TestTaskVersionID) == "" || strings.TrimSpace(instance.EnvironmentID) == "" {
		return errors.New("instance identity is incomplete")
	}
	if !isSupportedInstanceSnapshotSchema(instance.SnapshotSchemaVersion) || len(instance.SnapshotDigest) != sha256DigestLength || !strings.HasPrefix(instance.SnapshotDigest, "sha256:") || instance.SnapshotDigest != instance.sealedSnapshotDigest {
		return errors.New("instance snapshot identity is invalid")
	}
	return validateInstanceLifecycleShape(instance)
}

func (r Instance) Transition(to InstanceStatus, at int64) (Instance, error) {
	if !isSupportedInstanceSnapshotSchema(r.SnapshotSchemaVersion) || len(r.SnapshotDigest) != sha256DigestLength || !strings.HasPrefix(r.SnapshotDigest, "sha256:") || r.SnapshotDigest != r.sealedSnapshotDigest || strings.TrimSpace(r.TestTaskVersionID) == "" {
		return Instance{}, errors.New("instance snapshot identity is invalid")
	}
	if err := validateInstanceLifecycleShape(r); err != nil {
		return Instance{}, fmt.Errorf("instance source lifecycle is invalid: %w", err)
	}
	if err := ValidateInstanceStatusTransition(r.Status, to); err != nil {
		return Instance{}, err
	}
	prior := r.QueuedAt
	if r.StartedAt > prior {
		prior = r.StartedAt
	}
	if at < prior {
		return Instance{}, errors.New("instance transition timestamp is not monotonic")
	}
	next := r
	next.Status = to
	if to == Running {
		next.StartedAt = at
	} else {
		next.FinishedAt = at
	}
	if err := validateInstanceLifecycleShape(next); err != nil {
		return Instance{}, fmt.Errorf("instance transition result lifecycle is invalid: %w", err)
	}
	return next, nil
}
