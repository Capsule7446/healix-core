package node

import "github.com/Capsule7446/healix-core/domain/fault"

const (
	// CodeElementNotFound 表示目标规格中的全部选择器均未定位到元素。
	CodeElementNotFound fault.Code = "EXECUTION_ELEMENT_NOT_FOUND"
	// CodeTimeout 表示节点操作超过执行期限。
	CodeTimeout fault.Code = "EXECUTION_OPERATION_TIMEOUT"
	// CodeCanceled 表示节点操作被上下文取消。
	CodeCanceled fault.Code = "EXECUTION_OPERATION_CANCELED"
	// CodeTransientDriver 表示 Driver 明确报告可重试的瞬态故障。
	CodeTransientDriver fault.Code = "EXECUTION_TRANSIENT_DRIVER"
	// CodeOperationFailed 表示未分类的节点操作失败。
	CodeOperationFailed fault.Code = "EXECUTION_OPERATION_FAILED"
	// CodeStepTimelineStartFailed 表示步骤开始时间线事件记录失败。
	CodeStepTimelineStartFailed fault.Code = "EXECUTION_STEP_TIMELINE_START_FAILED"
	// CodeStepTimelineFinishFailed 表示步骤完成时间线事件记录失败。
	CodeStepTimelineFinishFailed fault.Code = "EXECUTION_STEP_TIMELINE_FINISH_FAILED"
	// CodeNodeCompletionObservation 表示节点完成观测记录失败。
	CodeNodeCompletionObservation fault.Code = "EXECUTION_NODE_COMPLETION_OBSERVATION_FAILED"
	// CodeLeafCompletionFailed 表示叶节点执行及其完成副作用存在聚合失败。
	CodeLeafCompletionFailed fault.Code = "EXECUTION_LEAF_COMPLETION_FAILED"

	// CodeStepConfigurationInvalid 表示动作、等待、重复、断言或工作流调用等步骤定义形状无效；
	// 具体失败位置由 violation 字段路径区分。
	CodeStepConfigurationInvalid fault.Code = "EXECUTION_STEP_CONFIGURATION_INVALID"
	// CodeStepPhaseTransitionInvalid 表示 StepExecution 状态机或叶生命周期开始边界拒绝阶段迁移。
	CodeStepPhaseTransitionInvalid fault.Code = "EXECUTION_STEP_PHASE_TRANSITION_INVALID"
	// CodeHealingRefused 表示自愈策略因安全限制或候选未达到审查阈值而拒绝决策。
	CodeHealingRefused fault.Code = "EXECUTION_HEALING_REFUSED"
	// CodeEvidenceRecordFailed 表示进度事件、自愈决策或候选样本写入 Facts 端口失败。
	CodeEvidenceRecordFailed fault.Code = "EXECUTION_EVIDENCE_RECORD_FAILED"
)
