// Package execution 拥有已提交测试任务实例的生命周期。
package execution

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fault"
)

// InstanceStatus 表示测试任务实例的生命周期状态。
type InstanceStatus string

const (
	// Queued 表示实例已入队但尚未运行。
	Queued InstanceStatus = "QUEUED"
	// Running 表示实例正在运行。
	Running InstanceStatus = "RUNNING"
	// Succeeded 表示实例成功终止。
	Succeeded InstanceStatus = "SUCCEEDED"
	// Failed 表示实例失败终止。
	Failed InstanceStatus = "FAILED"
	// Canceled 表示实例被取消终止。
	Canceled InstanceStatus = "CANCELED"
	// Aborted 表示实例被中止终止。
	Aborted InstanceStatus = "ABORTED"
)

// sha256DigestLength 是带 "sha256:" 前缀的摘要字符串固定长度。
const sha256DigestLength = 71

// ValidateInstanceStatusTransition 校验实例状态是否允许迁移到目标状态。
func ValidateInstanceStatusTransition(from, to InstanceStatus) error {
	allowed := (from == Queued && (to == Running || to == Canceled)) ||
		(from == Running && (to == Succeeded || to == Failed || to == Canceled || to == Aborted))
	if allowed {
		return nil
	}
	return mustExecutionFault(fault.FailedPrecondition, CodeInstanceStatusTransitionInvalid, "instance status transition is invalid")
}

// Instance 保存实例身份、封存快照摘要和持久化生命周期时间字段。
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

// NewInstance 校验新实例身份和排队生命周期，并在导出边界将未分类失败归入
// EXECUTION_CREATE_INSTANCE_SNAPSHOT_INVALID；原始细节仅保留在私有 cause 中。
func NewInstance(instance Instance, snapshot InstanceSnapshot) (Instance, error) {
	sealed, err := newInstance(instance, snapshot)
	if err != nil {
		return Instance{}, classifyCreateInstanceSnapshot(err)
	}
	return sealed, nil
}

// newInstance 在快照身份匹配时初始化实例的封存摘要和排队生命周期字段。
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

// validateInstanceLifecycleShape 只校验持久化生命周期字段，不依赖快照恢复或迁移意图。
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

// HydrateInstance 从持久化实例和快照恢复私有快照身份封印，并校验生命周期一致性。
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

// isSupportedInstanceSnapshotSchema 判断实例快照模式是否属于当前支持的版本集合。
func isSupportedInstanceSnapshotSchema(version InstanceSnapshotSchema) bool {
	return version == InstanceSnapshotSchemaV1 || version == InstanceSnapshotSchemaV2
}

// ValidateInstance 纯校验跨应用适配器边界返回的实例身份、生命周期形状和私有快照封印。
func ValidateInstance(instance Instance) error {
	if instance.ID.Validate() != nil || strings.TrimSpace(instance.ExecutionFlowID) == "" || strings.TrimSpace(instance.TestTaskVersionID) == "" || strings.TrimSpace(instance.EnvironmentID) == "" {
		return errors.New("instance identity is incomplete")
	}
	if !isSupportedInstanceSnapshotSchema(instance.SnapshotSchemaVersion) || len(instance.SnapshotDigest) != sha256DigestLength || !strings.HasPrefix(instance.SnapshotDigest, "sha256:") || instance.SnapshotDigest != instance.sealedSnapshotDigest {
		return errors.New("instance snapshot identity is invalid")
	}
	return validateInstanceLifecycleShape(instance)
}

// Transition 按允许的状态迁移和单调时间戳生成新的实例值，不修改接收者。
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
