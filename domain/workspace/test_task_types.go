package workspace

// TestTask 是稳定的、名称可变的资产。可执行配置仅存在于不可变的 TestTaskVersion 值中。
type TestTask struct {
	ID               string
	DisplayName      string
	FolderID         string
	CurrentVersionID string
	CreatedAt        int64
	UpdatedAt        int64
	DeletedAt        int64
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
	Task                  TestTask
	ExpectedTaskUpdatedAt int64
	Version               TestTaskVersion
	Workflows             []WorkflowDependencySnapshot
	Nodes                 []NodeDependencySnapshot
	References            []WorkflowReferenceResolution
}

type TestTaskRunStatus string

const (
	RunQueued    TestTaskRunStatus = "QUEUED"
	RunRunning   TestTaskRunStatus = "RUNNING"
	RunSucceeded TestTaskRunStatus = "SUCCEEDED"
	RunFailed    TestTaskRunStatus = "FAILED"
	RunCanceled  TestTaskRunStatus = "CANCELED"
	RunAborted   TestTaskRunStatus = "ABORTED"
)

type ExecutionStatus string

const (
	ExecutionPending   ExecutionStatus = "PENDING"
	ExecutionRunning   ExecutionStatus = "RUNNING"
	ExecutionSucceeded ExecutionStatus = "SUCCEEDED"
	ExecutionFailed    ExecutionStatus = "FAILED"
	ExecutionCanceled  ExecutionStatus = "CANCELED"
	ExecutionAborted   ExecutionStatus = "ABORTED"
)

type TestTaskRun struct {
	ID                         string
	TestTaskID                 string
	TestTaskVersionID          string
	TestTaskVersionNumber      int
	DisplayName                string
	FolderID                   string
	TriggerSource              string
	Status                     TestTaskRunStatus
	EnvironmentID              string
	EnvironmentName            string
	EnvironmentURL             string
	WorkflowCount              int
	SucceededCount             int
	FailedCount                int
	CanceledCount              int
	AbortedCount               int
	QueuePosition              int
	CreatedAt                  int64
	QueuedAt                   int64
	StartedAt                  int64
	FinishedAt                 int64
	ResourceBytes              int64
	CurrentWorkflowExecutionID string
	CurrentWorkflowName        string
	CurrentWorkflowVersionID   string
	CurrentWorkflowVersion     int
	CurrentStepExecutionID     string
	CurrentStepName            string
	ScreenshotPolicy           ScreenshotPolicy
	HealerPolicy               HealerPolicySnapshotV1
}

type EnvironmentSnapshot struct {
	ID          string
	DisplayName string
	BaseURL     string
	Username    string
	Password    string
	Variables   Properties
	Properties  Properties
	UpdatedAt   int64
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

type ExecutionParameterSnapshot struct {
	ID                    string
	DisplayName           string
	WorkflowID            string
	WorkflowVersionID     string
	WorkflowDisplayName   string
	WorkflowVersionNumber int
	Values                ParameterValues
	CreatedAt             int64
	LastUsedAt            int64
	DeletedAt             int64
	SourceRunID           string
	UsageCount            int
}

type ParameterSnapshotUsage struct {
	RunID          string
	RunDisplayName string
	ExecutionID    string
	SequenceNumber int
	Status         ExecutionStatus
	StartedAt      int64
	FinishedAt     int64
}

type WorkflowExecutionPlan struct {
	ID                    string
	TestTaskItemID        string
	SequenceNumber        int
	WorkflowID            string
	WorkflowVersionID     string
	WorkflowVersionNumber int
	VersionPolicy         WorkflowVersionPolicy
	WorkflowName          string
	StepCount             int
	ParameterSnapshot     ExecutionParameterSnapshot
}

// WorkflowParameterScope 是特定于事件的已解析参数副本。路径区分相同 WorkflowVersion 的重复/嵌套外观。
type WorkflowParameterScope struct {
	TestTaskItemID    string
	Path              string
	WorkflowID        string
	WorkflowVersionID string
	Values            ParameterValues
}

type StepPhaseEvent struct {
	ID             string
	ExecutionID    string
	WorkflowStepID string
	DisplayName    string
	Kind           string
	Phase          string
	Occurrence     int
	HierarchyPath  string
	Timestamp      int64
	ErrorMessage   string
}
