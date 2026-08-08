package execution

import "github.com/Capsule7446/healix-core/domain/fault"

// EntryStatus 描述一个工作流执行的生命周期，与承载它的 InstanceStatus 有意区分。
type EntryStatus string

const (
	// EntryPending 表示入口等待执行。
	EntryPending EntryStatus = "PENDING"
	// EntryRunning 表示入口正在执行。
	EntryRunning EntryStatus = "RUNNING"
	// EntrySucceeded 表示入口成功终止。
	EntrySucceeded EntryStatus = "SUCCEEDED"
	// EntryFailed 表示入口失败终止。
	EntryFailed EntryStatus = "FAILED"
	// EntryCanceled 表示入口被取消终止。
	EntryCanceled EntryStatus = "CANCELED"
	// EntryAborted 表示入口被中止终止。
	EntryAborted EntryStatus = "ABORTED"
	// EntrySkipped 表示入口因调度策略而跳过。
	EntrySkipped EntryStatus = "SKIPPED"
)

// ValidateEntryStatusTransition 校验入口状态迁移是否符合串行执行规则。
func ValidateEntryStatusTransition(from, to EntryStatus) error {
	return from.CanTransitionTo(to)
}

// IsTerminalEntryStatus 判断入口状态是否属于终止状态集合。
func IsTerminalEntryStatus(status EntryStatus) bool {
	switch status {
	case EntrySucceeded, EntryFailed, EntryCanceled, EntryAborted, EntrySkipped:
		return true
	default:
		return false
	}
}

// CanTransitionTo 判断当前入口状态是否允许迁移到目标状态。
func (from EntryStatus) CanTransitionTo(to EntryStatus) error {
	allowed := (from == EntryPending && (to == EntryRunning || to == EntryFailed || to == EntryCanceled || to == EntrySkipped)) ||
		(from == EntryRunning && (to == EntrySucceeded || to == EntryFailed || to == EntryCanceled || to == EntryAborted))
	if allowed {
		return nil
	}
	return mustExecutionFault(fault.FailedPrecondition, CodeStatusTransitionInvalid, "execution status transition is invalid")
}
