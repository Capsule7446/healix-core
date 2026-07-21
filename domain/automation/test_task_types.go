package automation

type FailurePolicy string

const (
	FailurePolicyStopOnFailure     FailurePolicy = "STOP_ON_FAILURE"
	FailurePolicyContinueOnFailure FailurePolicy = "CONTINUE_ON_FAILURE"
)

func (p FailurePolicy) IsValid() bool {
	return p == FailurePolicyStopOnFailure || p == FailurePolicyContinueOnFailure
}

// TestTask 是稳定的、名称可变的资产。可执行配置仅存在于不可变的 TestTaskVersion 值中。
type TestTask struct {
	ID               string
	DisplayName      string
	FolderID         string
	CurrentVersionID string
	CreatedAt        int64
	UpdatedAt        int64
	DeletedAt        int64
	Revision         Revision
}

type TestTaskItem struct {
	ID                string
	TestTaskVersionID string
	SequenceNumber    int
	WorkflowID        string
	VersionPolicy     WorkflowVersionPolicy
	WorkflowVersionID string
	Parameters        ParameterValues
}

type TestTaskVersion struct {
	ID                      string
	TestTaskID              string
	VersionNumber           int
	SourceVersionID         string
	Items                   []TestTaskItem
	FailurePolicy           FailurePolicy
	RequiredEnvironmentKeys []string
	CreatedAt               int64
}

type TestTaskAggregate struct {
	Task     TestTask
	Current  TestTaskVersion
	Versions []TestTaskVersion
}

// TestTaskVersionPlan 带有候选出版物以及用于其无环境验证的精确图表。适配器在发布版本的同一事务内重新检查该图。
type TestTaskVersionPlan struct {
	Task                 TestTask
	ExpectedTaskRevision Revision
	Version              TestTaskVersion
	Workflows            []WorkflowDependencySnapshot
	Nodes                []NodeDependencySnapshot
	References           []WorkflowReferenceResolution
}

type NodeDependencySnapshot struct {
	Node    Node
	Version NodeVersion
}
type WorkflowDependencySnapshot struct {
	Workflow           Workflow
	Version            WorkflowVersion
	ResolvedFromLatest bool
}
type WorkflowReferenceResolution struct {
	ParentWorkflowVersionID string
	StepID                  string
	WorkflowID              string
	WorkflowVersionID       string
	ResolvedFromLatest      bool
}
type ParameterValues map[string]any
