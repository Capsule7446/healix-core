package automation

import "github.com/Capsule7446/healix-core/domain/fault"

const (
	// CodeExecutionFlowInvalid 表示执行流程输入聚合校验失败。
	CodeExecutionFlowInvalid fault.Code = "AUTOMATION_EXECUTION_FLOW_INVALID"
	// CodeExecutionFlowHistoryInvalid 表示执行流程版本历史无效。
	CodeExecutionFlowHistoryInvalid fault.Code = "AUTOMATION_EXECUTION_FLOW_HISTORY_INVALID"
	// CodeExecutionFlowDependencyInvalid 表示执行流程依赖解析无效。
	CodeExecutionFlowDependencyInvalid fault.Code = "AUTOMATION_EXECUTION_FLOW_DEPENDENCY_INVALID"
	// CodeSamplingPublicationContentInvalid 表示采样发布内容无效。
	CodeSamplingPublicationContentInvalid fault.Code = "AUTOMATION_SAMPLING_PUBLICATION_CONTENT_INVALID"

	// CodePersistedRevisionInvalid 表示持久化修订无效。
	CodePersistedRevisionInvalid fault.Code = "AUTOMATION_PERSISTED_REVISION_INVALID"
	// CodeRevisionExhausted 表示修订值已耗尽。
	CodeRevisionExhausted fault.Code = "AUTOMATION_REVISION_EXHAUSTED"
	// CodePersistedVersionNumberInvalid 表示持久化版本号无效。
	CodePersistedVersionNumberInvalid fault.Code = "AUTOMATION_PERSISTED_VERSION_NUMBER_INVALID"
	// CodeHealCandidateIdentityInvalid 表示自愈候选身份无效。
	CodeHealCandidateIdentityInvalid fault.Code = "AUTOMATION_HEAL_CANDIDATE_IDENTITY_INVALID"
	// CodeHealCandidateStateInvalid 表示自愈候选状态不允许当前操作。
	CodeHealCandidateStateInvalid fault.Code = "AUTOMATION_HEAL_CANDIDATE_STATE_INVALID"
	// CodeHealCandidateReviewStatusInvalid 表示自愈候选审查状态无效。
	CodeHealCandidateReviewStatusInvalid fault.Code = "AUTOMATION_HEAL_CANDIDATE_REVIEW_STATUS_INVALID"
	// CodeHealCandidateReviewCommandInvalid 表示自愈候选审查命令无效。
	CodeHealCandidateReviewCommandInvalid fault.Code = "AUTOMATION_HEAL_CANDIDATE_REVIEW_COMMAND_INVALID"
	// CodeHealApprovalStatusInvalid 表示自愈审批状态无效。
	CodeHealApprovalStatusInvalid fault.Code = "AUTOMATION_HEAL_APPROVAL_STATUS_INVALID"
	// CodeHealDecisionBandInvalid 表示自愈决策区间无效。
	CodeHealDecisionBandInvalid fault.Code = "AUTOMATION_HEAL_DECISION_BAND_INVALID"
	// CodeHealConfidenceInvalid 表示自愈置信度无效。
	CodeHealConfidenceInvalid fault.Code = "AUTOMATION_HEAL_CONFIDENCE_INVALID"
	// CodeHealStreakStateInvalid 表示自愈连续状态无效。
	CodeHealStreakStateInvalid fault.Code = "AUTOMATION_HEAL_STREAK_STATE_INVALID"
	// CodeHealObservationInvalid 表示自愈观测无效。
	CodeHealObservationInvalid fault.Code = "AUTOMATION_HEAL_OBSERVATION_INVALID"
	// CodeHealSequenceConflict 表示自愈序列与持久化顺序冲突。
	CodeHealSequenceConflict fault.Code = "AUTOMATION_HEAL_SEQUENCE_CONFLICT"
	// CodeHealProvenanceConflict 表示自愈观测与持久化来源冲突。
	CodeHealProvenanceConflict fault.Code = "AUTOMATION_HEAL_PROVENANCE_CONFLICT"
	// CodeHealStreakRejectionInvalid 表示当前自愈连续状态不能被拒绝。
	CodeHealStreakRejectionInvalid fault.Code = "AUTOMATION_HEAL_STREAK_REJECTION_INVALID"

	// CodeElementTargetInvalid 表示元素目标内容无效。
	CodeElementTargetInvalid fault.Code = "AUTOMATION_ELEMENT_TARGET_INVALID"
	// CodeElementTargetHistoryInvalid 表示元素目标版本历史无效。
	CodeElementTargetHistoryInvalid fault.Code = "AUTOMATION_ELEMENT_TARGET_HISTORY_INVALID"
	// CodeFlowFragmentInvalid 表示流程片段内容无效。
	CodeFlowFragmentInvalid fault.Code = "AUTOMATION_FLOW_FRAGMENT_INVALID"
	// CodeFlowFragmentHistoryInvalid 表示流程片段版本历史无效。
	CodeFlowFragmentHistoryInvalid fault.Code = "AUTOMATION_FLOW_FRAGMENT_HISTORY_INVALID"
	// CodeEnvironmentInvalid 表示环境内容无效。
	CodeEnvironmentInvalid fault.Code = "AUTOMATION_ENVIRONMENT_INVALID"
	// CodeAggregateTransitionInvalid 表示自动化聚合状态转换无效。
	CodeAggregateTransitionInvalid fault.Code = "AUTOMATION_AGGREGATE_TRANSITION_INVALID"
)

// persistedRevisionInvalidError 构造持久化修订非零校验失败。
func persistedRevisionInvalidError() error {
	return mustAutomationFault(fault.FailedPrecondition, CodePersistedRevisionInvalid, "persisted revision must be non-zero")
}

// persistedVersionNumberInvalidError 构造持久化版本号为正数的校验失败。
func persistedVersionNumberInvalidError() error {
	return mustAutomationFault(fault.FailedPrecondition, CodePersistedVersionNumberInvalid, "persisted version number must be positive")
}

// revisionExhaustedError 构造修订值耗尽错误。
func revisionExhaustedError() error {
	return mustAutomationFault(fault.ResourceExhausted, CodeRevisionExhausted, "revision value is exhausted")
}

// healCandidateIdentityInvalidError 构造自愈候选身份无效错误。
func healCandidateIdentityInvalidError() error {
	return mustAutomationFault(fault.InvalidArgument, CodeHealCandidateIdentityInvalid, "heal candidate identity is invalid")
}

// healCandidateStateInvalidError 构造自愈候选状态不允许当前操作的错误。
func healCandidateStateInvalidError() error {
	return mustAutomationFault(fault.FailedPrecondition, CodeHealCandidateStateInvalid, "heal candidate state does not allow this operation")
}

// healCandidateReviewStatusInvalidError 构造自愈候选审查状态无效错误。
func healCandidateReviewStatusInvalidError() error {
	return mustAutomationFault(fault.InvalidArgument, CodeHealCandidateReviewStatusInvalid, "heal candidate review status is invalid")
}

// healCandidateReviewCommandInvalidError 构造自愈候选审查命令无效错误。
func healCandidateReviewCommandInvalidError() error {
	return mustAutomationFault(fault.InvalidArgument, CodeHealCandidateReviewCommandInvalid, "heal candidate review command is invalid")
}

// healApprovalStatusInvalidError 构造自愈审批状态无效错误。
func healApprovalStatusInvalidError() error {
	return mustAutomationFault(fault.InvalidArgument, CodeHealApprovalStatusInvalid, "heal approval status is invalid")
}

// healDecisionBandInvalidError 构造自愈决策区间无效错误。
func healDecisionBandInvalidError() error {
	return mustAutomationFault(fault.InvalidArgument, CodeHealDecisionBandInvalid, "heal decision band is invalid")
}

// healConfidenceInvalidError 构造自愈置信度无效错误。
func healConfidenceInvalidError() error {
	return mustAutomationFault(fault.InvalidArgument, CodeHealConfidenceInvalid, "heal confidence is invalid")
}

// healStreakStateInvalidError 包装自愈连续状态无效原因并保留内部原因。
func healStreakStateInvalidError(cause error) error {
	return wrapAutomationFault(cause, fault.FailedPrecondition, CodeHealStreakStateInvalid, "persisted heal streak state is invalid")
}

// healObservationInvalidError 包装自愈观测无效原因并保留内部原因。
func healObservationInvalidError(cause error) error {
	return wrapAutomationFault(cause, fault.InvalidArgument, CodeHealObservationInvalid, "heal observation is invalid")
}

// healSequenceConflictError 包装自愈序列顺序冲突原因。
func healSequenceConflictError(cause error) error {
	return wrapAutomationFault(cause, fault.Conflict, CodeHealSequenceConflict, "heal sequence conflicts with persisted ordering")
}

// healProvenanceConflictError 包装自愈持久化来源冲突原因。
func healProvenanceConflictError(cause error) error {
	return wrapAutomationFault(cause, fault.Conflict, CodeHealProvenanceConflict, "heal observation conflicts with persisted provenance")
}

// healStreakRejectionInvalidError 包装当前状态不能拒绝自愈连续的原因。
func healStreakRejectionInvalidError(cause error) error {
	return wrapAutomationFault(cause, fault.FailedPrecondition, CodeHealStreakRejectionInvalid, "heal streak cannot be rejected in its current state")
}

// executionFlowInvalidError 构造执行流程输入的聚合校验错误。
// 一个顶层错误携带按输入顺序排列的字段违规，而不是为每个字段生成错误码或拼接文本。
// 超过 fault.MaxViolations 的违规保留前缀并丢弃，因为输入不受信任，构造失败会导致 panic。
func executionFlowInvalidError(violations []fault.Violation) error {
	if len(violations) > fault.MaxViolations {
		violations = violations[:fault.MaxViolations]
	}
	return mustAutomationFault(fault.InvalidArgument, CodeExecutionFlowInvalid, "execution flow input is invalid", fault.WithViolations(violations...))
}

// executionFlowHistoryInvalidError 校验版本历史本身的顺序、唯一性和来源链，而非单个版本形状。
// 它使用 FAILED_PRECONDITION，因为修复动作是修复持久化历史，而不是修改本次输入字段。
func executionFlowHistoryInvalidError(violations []fault.Violation) error {
	return mustAutomationFault(fault.FailedPrecondition, CodeExecutionFlowHistoryInvalid, "execution flow version history is invalid", fault.WithViolations(capViolations(violations)...))
}

// executionFlowDependencyInvalidError 校验快照集合、引用图及其参数绑定的依赖解析结果。
// 它独立于字段级执行流程错误，因为调用方应重新解析目录图，而不是编辑单个字段。
func executionFlowDependencyInvalidError(violations []fault.Violation) error {
	return mustAutomationFault(fault.InvalidArgument, CodeExecutionFlowDependencyInvalid, "execution flow dependency resolution is invalid", fault.WithViolations(capViolations(violations)...))
}

// classifySamplingPublicationContent 在采样发布的边界归类内容形状错误。
// 内部检查保留为普通 Go 错误并作为私有原因传递；已分类的节点聚合错误或修订错误原样透传，避免嵌套错误码。
// 本错误码表示内容层问题，与 application/automation 产生的事务层 AUTOMATION_SAMPLING_PUBLICATION_* 错误码不同。
func classifySamplingPublicationContent(cause error) error {
	if cause == nil {
		return nil
	}
	if _, classified := fault.CodeOf(cause); classified {
		return cause
	}
	return wrapAutomationFault(cause, fault.InvalidArgument, CodeSamplingPublicationContentInvalid, "sampling publication content is invalid")
}

// capViolations 在聚合超过错误信封上限时保留确定性的前缀，避免不受信任输入导致校验 panic。
func capViolations(violations []fault.Violation) []fault.Violation {
	if len(violations) > fault.MaxViolations {
		return violations[:fault.MaxViolations]
	}
	return violations
}

// elementTargetInvalidError 构造元素目标内容的聚合校验错误，覆盖选择器、指纹、属性和版本来源。
// 一个顶层错误携带按顺序排列的字段违规，不为每个字段生成独立错误码，也不拼接公共文本。
func elementTargetInvalidError(violations ...fault.Violation) error {
	return mustAutomationFault(fault.InvalidArgument, CodeElementTargetInvalid, "element target content is invalid", fault.WithViolations(capViolations(violations)...))
}

// elementTargetHistoryInvalidError 校验版本历史的顺序、唯一性和所有权，而非单个版本的内容形状。
// 它使用 FAILED_PRECONDITION，因为修复动作是修复持久化历史，而不是修改本次输入字段。
func elementTargetHistoryInvalidError(violations ...fault.Violation) error {
	return mustAutomationFault(fault.FailedPrecondition, CodeElementTargetHistoryInvalid, "element target version history is invalid", fault.WithViolations(capViolations(violations)...))
}

// flowFragmentInvalidError 构造流程片段元数据、步骤树和参数定义的聚合校验错误。
// 参数定义属于流程片段内容，其结构失败降级到同一错误信封，不新增错误码。
func flowFragmentInvalidError(violations ...fault.Violation) error {
	return mustAutomationFault(fault.InvalidArgument, CodeFlowFragmentInvalid, "flow fragment content is invalid", fault.WithViolations(capViolations(violations)...))
}

// flowFragmentHistoryInvalidError 校验流程片段版本历史，语义与元素目标历史错误一致。
func flowFragmentHistoryInvalidError(violations ...fault.Violation) error {
	return mustAutomationFault(fault.FailedPrecondition, CodeFlowFragmentHistoryInvalid, "flow fragment version history is invalid", fault.WithViolations(capViolations(violations)...))
}

// environmentInvalidError 构造环境身份、基础 URL 和类型化变量的聚合校验错误。
func environmentInvalidError(violations ...fault.Violation) error {
	return mustAutomationFault(fault.InvalidArgument, CodeEnvironmentInvalid, "environment content is invalid", fault.WithViolations(capViolations(violations)...))
}

// aggregateTransitionInvalidError 构造所有自动化聚合共用的空转换和时间戳校验错误。
// 环境、元素目标、流程片段和执行流程均使用同一辅助函数；错误为 FAILED_PRECONDITION，
// 调用方应先达到有效的前置状态（新时间戳、非终止生命周期或新版本身份）再重试。
func aggregateTransitionInvalidError(violation fault.Violation) error {
	return mustAutomationFault(fault.FailedPrecondition, CodeAggregateTransitionInvalid, "automation aggregate transition is invalid", fault.WithViolations(violation))
}

// mustViolation 构造字段违规；底层构造失败表示代码契约错误，因此触发 panic。
func mustViolation(code fault.Code, field, message string) fault.Violation {
	violation, constructionErr := fault.NewViolation(code, field, message)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return violation
}

// wrapAutomationFault 将原因包装为自动化领域错误，并保留原因供 Unwrap 使用。
func wrapAutomationFault(cause error, kind fault.Kind, code fault.Code, message string, options ...fault.Option) error {
	err, constructionErr := fault.Wrap(cause, kind, code, message, options...)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}

// mustAutomationFault 构造自动化领域错误；底层构造失败表示代码契约错误，因此触发 panic。
func mustAutomationFault(kind fault.Kind, code fault.Code, message string, options ...fault.Option) error {
	err, constructionErr := fault.New(kind, code, message, options...)
	if constructionErr != nil {
		panic(constructionErr)
	}
	return err
}
