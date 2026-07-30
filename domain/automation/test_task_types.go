package automation

import "github.com/Capsule7446/healix-core/domain/parameter"

type FailurePolicy string

const (
	FailurePolicyStopOnFailure     FailurePolicy = "STOP_ON_FAILURE"
	FailurePolicyContinueOnFailure FailurePolicy = "CONTINUE_ON_FAILURE"
)

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

type ExecutionFlowItem struct {
	ID                string
	TestTaskVersionID string
	SequenceNumber    int
	FlowFragmentID    string
	VersionPolicy     FlowFragmentVersionPolicy
	WorkflowVersionID string
	Parameters        map[string]parameter.Value
}

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

type ExecutionFlowVersionPublication struct {
	ID                      string
	Items                   []ExecutionFlowItem
	FailurePolicy           FailurePolicy
	RequiredEnvironmentKeys []string
	CreatedAt               int64
}

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
	Nodes                         []NodeDependencySnapshot
	References                    []FlowFragmentReferenceResolution
}

type NodeDependencySnapshot struct {
	Node    Node
	Version NodeVersion
}
type FlowFragmentDependencySnapshot struct {
	FlowFragment       FlowFragment
	Version            FlowFragmentVersion
	ResolvedFromLatest bool
}
type FlowFragmentReferenceResolution struct {
	ParentFlowFragmentVersionID string
	StepID                      string
	FlowFragmentID              string
	WorkflowVersionID           string
	ResolvedFromLatest          bool
}
