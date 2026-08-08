package evidence

import "github.com/Capsule7446/healix-core/domain/fault"

const (
	// CodeStepTransitionCommitInvalid 表示步骤迁移提交的事实或拓扑校验失败。
	CodeStepTransitionCommitInvalid fault.Code = "EVIDENCE_STEP_TRANSITION_COMMIT_INVALID"
	// CodeCommitFactLimitExceeded 表示步骤迁移提交超过事实数量上限。
	CodeCommitFactLimitExceeded fault.Code = "EVIDENCE_COMMIT_FACT_LIMIT_EXCEEDED"
	// CodeStepProgressEventInvalid 表示步骤进度事件校验失败。
	CodeStepProgressEventInvalid fault.Code = "EVIDENCE_STEP_PROGRESS_EVENT_INVALID"
	// CodeStepFactInvalid 表示步骤终止事实校验失败。
	CodeStepFactInvalid fault.Code = "EVIDENCE_STEP_FACT_INVALID"
	// CodeHealObservationInvalid 表示自愈观测校验失败。
	CodeHealObservationInvalid fault.Code = "EVIDENCE_HEAL_OBSERVATION_INVALID"
	// CodeValidationObservationInvalid 表示验证观测校验失败。
	CodeValidationObservationInvalid fault.Code = "EVIDENCE_VALIDATION_OBSERVATION_INVALID"
	// CodeValidationGroupObservationInvalid 表示验证组终态观测校验失败。
	CodeValidationGroupObservationInvalid fault.Code = "EVIDENCE_VALIDATION_GROUP_OBSERVATION_INVALID"
)

// 本上下文中的提交、步骤执行、实例、验证、自愈、分组和元素目标身份均由调用方提供，
// 不会进入公开文本。子校验失败降级为外层封套违规，不嵌套第二个 fault，便于宿主直接分类。

// stepTransitionCommitInvalidError 构造步骤迁移提交无效的聚合错误。
func stepTransitionCommitInvalidError(violations []fault.Violation) error {
	return mustEvidenceFault(fault.InvalidArgument, CodeStepTransitionCommitInvalid, "step transition commit is invalid", fault.WithViolations(capViolations(violations)...))
}

// commitFactLimitExceededError 构造事实数量超限错误，处置方式是拆分提交而非修复字段。
func commitFactLimitExceededError() error {
	return mustEvidenceFault(fault.OutOfRange, CodeCommitFactLimitExceeded, "step transition commit exceeds its fact limit")
}

// stepProgressEventInvalidError 构造步骤进度事件无效的聚合错误。
func stepProgressEventInvalidError(violations []fault.Violation) error {
	return mustEvidenceFault(fault.InvalidArgument, CodeStepProgressEventInvalid, "step progress event is invalid", fault.WithViolations(capViolations(violations)...))
}

// stepFactInvalidError 构造步骤事实无效的聚合错误。
func stepFactInvalidError(violations []fault.Violation) error {
	return mustEvidenceFault(fault.InvalidArgument, CodeStepFactInvalid, "step fact is invalid", fault.WithViolations(capViolations(violations)...))
}

// healObservationInvalidError 构造自愈观测无效的聚合错误。
func healObservationInvalidError(violations []fault.Violation) error {
	return mustEvidenceFault(fault.InvalidArgument, CodeHealObservationInvalid, "heal observation is invalid", fault.WithViolations(capViolations(violations)...))
}

// validationObservationInvalidError 构造验证观测无效的聚合错误。
func validationObservationInvalidError(violations []fault.Violation) error {
	return mustEvidenceFault(fault.InvalidArgument, CodeValidationObservationInvalid, "validation observation is invalid", fault.WithViolations(capViolations(violations)...))
}

// validationGroupObservationInvalidError 构造验证组终态观测无效的聚合错误。
func validationGroupObservationInvalidError(violations []fault.Violation) error {
	return mustEvidenceFault(fault.InvalidArgument, CodeValidationGroupObservationInvalid, "validation group observation is invalid", fault.WithViolations(capViolations(violations)...))
}

// capViolations 在聚合违规项超过封套上限时保留确定性前缀，避免不可信输入将校验变成 panic。
func capViolations(violations []fault.Violation) []fault.Violation {
	if len(violations) > fault.MaxViolations {
		return violations[:fault.MaxViolations]
	}
	return violations
}

// atCap 判断违规封套是否已满，使集合遍历停止并保留与 capViolations 相同的输入前缀。
func atCap(violations []fault.Violation) bool {
	return len(violations) >= fault.MaxViolations
}

// mustViolation 构造已知有效的字段违规；内部构造失败表示程序契约错误并触发 panic。
func mustViolation(code fault.Code, field, message string) fault.Violation {
	violation, constructionErr := fault.NewViolation(code, field, message)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return violation
}

// mustEvidenceFault 构造 evidence 领域错误；内部构造失败表示程序契约错误并触发 panic。
func mustEvidenceFault(kind fault.Kind, code fault.Code, message string, options ...fault.Option) error {
	err, constructionErr := fault.New(kind, code, message, options...)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}
