package workspace

// TestTask is the stable, mutable-name asset. Executable configuration lives
// exclusively in immutable TestTaskVersion values.
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

type TestTaskQueryResult struct {
	TestTaskAggregate
	LastRunStatus TestTaskRunStatus
	LastRunAt     int64
}

// TestTaskVersionPlan carries a publication candidate plus the exact graph
// used for its environment-free validation. The adapter rechecks this graph
// inside the same transaction that publishes the version.
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

// WorkflowParameterScope is an occurrence-specific resolved parameter copy.
// Path distinguishes repeated/nested appearances of the same WorkflowVersion.
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
