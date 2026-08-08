// Package evidence 拥有框架无关的执行事实身份和终止性。
package evidence

import (
	"strings"

	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
)

// Phase 表示执行事实的终止阶段。
type Phase string

const (
	// PhaseSucceeded 表示执行成功终止。
	PhaseSucceeded Phase = "SUCCEEDED"
	// PhaseFailed 表示执行失败终止。
	PhaseFailed Phase = "FAILED"
	// PhaseCanceled 表示执行被取消终止。
	PhaseCanceled Phase = "CANCELED"
	// PhaseAborted 表示执行被中止终止。
	PhaseAborted Phase = "ABORTED"
)

// IsTerminal 判断阶段是否属于已知终止集合。
func (p Phase) IsTerminal() bool {
	return p == PhaseSucceeded || p == PhaseFailed || p == PhaseCanceled || p == PhaseAborted
}

// StepFact 记录一次框架无关的步骤终止事实及其发生序号和观测时间。
type StepFact struct {
	ID              string
	InstanceID      execution.InstanceID
	EntryID         execution.EntryID
	StepExecutionID execution.StepExecutionID
	Occurrence      int
	Phase           Phase
	ObservedAt      int64
}

// Validate 校验步骤事实的身份、终止阶段、Occurrence 和观测时间；失败详情不会回显阶段值。
func (f StepFact) Validate() error {
	var violations []fault.Violation
	if strings.TrimSpace(f.ID) == "" || f.InstanceID.Validate() != nil || f.EntryID.Validate() != nil || f.StepExecutionID.Validate() != nil {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "identity", "step fact identity is required"))
	}
	if !f.Phase.IsTerminal() {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "phase", "step fact phase must be terminal"))
	}
	violations = appendOccurrenceViolations(violations, f.Occurrence, "")
	if f.ObservedAt <= 0 {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "observedAt", "step fact observation time must be positive"))
	}
	if len(violations) != 0 {
		return stepFactInvalidError(violations)
	}
	return nil
}
