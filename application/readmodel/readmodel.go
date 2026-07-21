package readmodel

import "context"

type NodeListItem struct {
	ID             string
	DisplayName    string
	CurrentVersion string
	VersionNumber  int
	RefCount       int
}

type WorkflowListItem struct {
	ID             string
	DisplayName    string
	CurrentVersion string
	VersionNumber  int
	LastRunStatus  string
	LastRunAt      int64
}

type TestTaskListItem struct {
	ID             string
	DisplayName    string
	CurrentVersion string
	VersionNumber  int
	LastRunStatus  string
	LastRunAt      int64
}

type RunListItem struct {
	ID                  string
	TestTaskID          string
	Status              string
	QueuePosition       int
	CurrentWorkflowName string
	CurrentStepName     string
	EnvironmentID       string
	EnvironmentName     string
	EnvironmentURL      string
	CreatedAt           int64
	QueuedAt            int64
	StartedAt           int64
	FinishedAt          int64
}

type ExecutionDetailView struct {
	ID          string
	RunID       string
	Status      string
	Steps       []StepView
	Requests    []RequestView
	Heals       []HealingReviewView
	Validations []ValidationView
}

type StepView struct {
	ID          string
	NodeID      string
	DisplayName string
	Phase       string
	Error       string
}

type RequestView struct {
	ID         string
	Method     string
	URL        string
	StatusCode int
}

type ValidationView struct {
	ID       string
	Passed   bool
	Expected string
	Actual   string
	Reason   string
}

type HealingCandidateView struct {
	CandidateHash   string
	FingerprintHash string
	Score           float64
	Rank            int
	Eligible        bool
	Selected        bool
	Status          string
}

type HealingReviewView struct {
	ObservationID string
	RunID         string
	NodeID        string
	SpecID        string
	OldSelector   string
	Disposition   string
	Explanation   string
	Candidates    []HealingCandidateView
}

type DashboardView struct {
	StatusCounts map[string]int
	Total30Days  int
	Running      *RunListItem
	Queue        []RunListItem
	Runs         []RunListItem
	TestTasks    []TestTaskListItem
}

type NodeReader interface {
	ListNodes(context.Context, bool) ([]NodeListItem, error)
	GetNode(context.Context, string) (NodeListItem, error)
}

type WorkflowReader interface {
	ListWorkflows(context.Context, bool) ([]WorkflowListItem, error)
	GetWorkflow(context.Context, string) (WorkflowListItem, error)
}

type TestTaskReader interface {
	ListTestTasks(context.Context, bool) ([]TestTaskListItem, error)
	GetTestTask(context.Context, string) (TestTaskListItem, error)
	ListTestTaskRuns(context.Context, string) ([]RunListItem, error)
	GetTestTaskRunDetail(context.Context, string) (ExecutionDetailView, error)
}

type DashboardReader interface {
	Dashboard(context.Context) (DashboardView, error)
}
