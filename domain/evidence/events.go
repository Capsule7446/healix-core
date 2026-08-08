package evidence

import (
	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

// ProgressPhase 表示步骤进度事件的非终止阶段。
type ProgressPhase string

const (
	// ProgressRunning 表示步骤正在执行。
	ProgressRunning ProgressPhase = "RUNNING"
	// ProgressHealing 表示步骤正在执行自愈。
	ProgressHealing ProgressPhase = "HEALING"
	// ProgressTransitioning 表示步骤正在迁移状态。
	ProgressTransitioning ProgressPhase = "TRANSITIONING"
	// ProgressValidating 表示步骤正在校验结果。
	ProgressValidating ProgressPhase = "VALIDATING"
)

// StepProgressEvent 记录步骤执行期间的非终止进度及其执行坐标。
type StepProgressEvent struct {
	ID                 execution.StepExecutionID
	EntryID            execution.EntryID
	InvocationPath     execution.InvocationPath
	FlowFragmentStepID string
	DisplayName        string
	Kind               string
	Phase              ProgressPhase
	Occurrence         int
	HierarchyPath      string
	Timestamp          int64
}

// Validate 校验进度事件的身份、非终止阶段、Occurrence 和时间戳。
func (e StepProgressEvent) Validate() error {
	var violations []fault.Violation
	if e.ID.Validate() != nil || e.EntryID.Validate() != nil || e.FlowFragmentStepID == "" || e.DisplayName == "" || e.Kind == "" {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "identity", "event identity is required"))
	}
	switch e.Phase {
	case ProgressRunning, ProgressHealing, ProgressTransitioning, ProgressValidating:
	default:
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "phase", "event phase must be non-terminal"))
	}
	violations = appendOccurrenceViolations(violations, e.Occurrence, "")
	if e.Timestamp <= 0 {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "timestamp", "event timestamp must be positive"))
	}
	if len(violations) != 0 {
		return stepProgressEventInvalidError(violations)
	}
	return nil
}

// StepPhaseEvent 是框架无关的终止执行时间线事件。
type StepPhaseEvent struct {
	ID                 execution.StepExecutionID
	EntryID            execution.EntryID
	InvocationPath     execution.InvocationPath
	FlowFragmentStepID string
	DisplayName        string
	Kind               string
	Phase              string
	Occurrence         int
	HierarchyPath      string
	Timestamp          int64
}
