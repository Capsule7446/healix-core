package automation

import "github.com/Capsule7446/healix-core/domain/parameter"

// FailurePolicy 标识执行流程遇到失败后的处理策略。
type FailurePolicy string

const (
	// FailurePolicyStopOnFailure 表示首个失败后停止流程。
	FailurePolicyStopOnFailure FailurePolicy = "STOP_ON_FAILURE"
	// FailurePolicyContinueOnFailure 表示失败后继续流程。
	FailurePolicyContinueOnFailure FailurePolicy = "CONTINUE_ON_FAILURE"
)

// IsValid 判断失败策略是否属于支持的枚举值。
func (p FailurePolicy) IsValid() bool {
	return p == FailurePolicyStopOnFailure || p == FailurePolicyContinueOnFailure
}

// ExecutionFlow 是稳定的、名称可变的资产。可执行配置仅存在于不可变的 ExecutionFlowVersion 值中。
type ExecutionFlow struct {
	ID               string
	DisplayName      string
	FolderID         string
	CurrentVersionID string
	CreatedAt        int64
	UpdatedAt        int64
	DeletedAt        int64
	Revision         Revision
}

// ExecutionFlowItem 描述执行流程中引用流程片段的一个条目及参数。
type ExecutionFlowItem struct {
	ID                string
	TestTaskVersionID string
	SequenceNumber    int
	FlowFragmentID    string
	VersionPolicy     FlowFragmentVersionPolicy
	WorkflowVersionID string
	Parameters        map[string]parameter.Value
}

// ExecutionFlowVersion 表示执行流程的一份不可变版本内容。
type ExecutionFlowVersion struct {
	ID                      string
	ExecutionFlowID         string
	VersionNumber           int
	SourceVersionID         string
	Items                   []ExecutionFlowItem
	FailurePolicy           FailurePolicy
	RequiredEnvironmentKeys []string
	CreatedAt               int64
}

// ExecutionFlowVersionPublication 携带发布新执行流程版本所需的内容。
type ExecutionFlowVersionPublication struct {
	ID                      string
	Items                   []ExecutionFlowItem
	FailurePolicy           FailurePolicy
	RequiredEnvironmentKeys []string
	CreatedAt               int64
}

// ExecutionFlowAggregate 持有执行流程元数据、当前版本和完整版本历史。
type ExecutionFlowAggregate struct {
	Task     ExecutionFlow
	Current  ExecutionFlowVersion
	Versions []ExecutionFlowVersion
}

// ResolvedExecutionFlow 带有候选出版物以及用于其无环境验证的精确图表。适配器在发布版本的同一事务内重新检查该图。
type ResolvedExecutionFlow struct {
	Task                          ExecutionFlow
	ExpectedExecutionFlowRevision Revision
	Version                       ExecutionFlowVersion
	Workflows                     []FlowFragmentDependencySnapshot
	Nodes                         []ElementTargetDependencySnapshot
	References                    []FlowFragmentReferenceResolution
}

// ElementTargetDependencySnapshot 保存解析后的元素目标及其选定版本。
type ElementTargetDependencySnapshot struct {
	ElementTarget ElementTarget
	Version       ElementTargetVersion
}

// FlowFragmentDependencySnapshot 保存解析后的流程片段及其选定版本。
type FlowFragmentDependencySnapshot struct {
	FlowFragment       FlowFragment
	Version            FlowFragmentVersion
	ResolvedFromLatest bool
}

// FlowFragmentReferenceResolution 保存流程片段引用解析后的目标和来源策略。
type FlowFragmentReferenceResolution struct {
	ParentFlowFragmentVersionID string
	StepID                      string
	FlowFragmentID              string
	WorkflowVersionID           string
	ResolvedFromLatest          bool
}
