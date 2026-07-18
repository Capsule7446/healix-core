package workspace

import (
	"context"
	"errors"
)

var (
	// ErrNotFound marks lookups whose subject does not exist. Adapters wrap it
	// with the entity kind and ID so callers can errors.Is-discriminate
	// "missing" from storage failures instead of collapsing both into one
	// business conclusion.
	ErrNotFound                    = errors.New("not found")
	ErrTestTaskRunNotFound         = errors.New("test task run not found")
	ErrTestTaskRunNotDeletable     = errors.New("test task run must be terminal before deletion")
	ErrHealCandidateNotReviewable  = errors.New("heal candidate is not awaiting approval")
	ErrHealCandidateBaseNotCurrent = errors.New("heal candidate base version is not current")
)

type FolderReader interface {
	ListFolders(ctx context.Context, kind FolderKind) ([]WorkspaceFolder, error)
	GetFolder(ctx context.Context, id string) (WorkspaceFolder, error)
}

type EnvironmentReader interface {
	ListEnvironments(ctx context.Context, includeDeleted bool) ([]Environment, error)
	GetEnvironment(ctx context.Context, id string) (Environment, error)
}

type NodeReader interface {
	ListNodes(ctx context.Context, includeDeleted bool) ([]NodeQueryResult, error)
	GetNode(ctx context.Context, id string) (NodeQueryResult, error)
}

type WorkflowReader interface {
	ListWorkflows(ctx context.Context, includeDeleted bool) ([]WorkflowQueryResult, error)
	GetWorkflow(ctx context.Context, id string) (WorkflowQueryResult, error)
}

type TestTaskReader interface {
	ListTestTasks(ctx context.Context, includeDeleted bool) ([]TestTaskQueryResult, error)
	GetTestTask(ctx context.Context, id string) (TestTaskQueryResult, error)
	GetTestTaskVersion(ctx context.Context, versionID string) (TestTaskVersion, error)
	ListTestTaskRuns(ctx context.Context, testTaskID string) ([]TestTaskRun, error)
	ListAllTestTaskRuns(ctx context.Context) ([]TestTaskRun, error)
	GetTestTaskRunDetail(ctx context.Context, runID string) (TestTaskRunDetail, error)
	TestTaskRunResourceURIs(ctx context.Context, runID string) ([]string, error)
}

type ExecutionEvidenceReader interface {
	GetExecutionDetail(ctx context.Context, executionID string) (ExecutionDetail, error)
	GetNetworkEvidence(ctx context.Context, requestID string) (NetworkEvidence, error)
}

type MaintenanceReader interface {
	ExpiredTestTaskRunIDs(ctx context.Context, cutoff int64) ([]string, error)
	StorageStats(ctx context.Context) (StorageStats, error)
	ListParameterSnapshots(ctx context.Context, includeDeleted bool) ([]ExecutionParameterSnapshot, error)
}

type HealCandidateReader interface {
	ListHealCandidates(ctx context.Context, includeStale bool) ([]HealCandidateRecord, error)
	ListHealObservations(ctx context.Context, nodeID, baseVersionID, candidateHash string) ([]HealObservationDetail, error)
}

type DashboardReader interface {
	Dashboard(ctx context.Context) (Dashboard, error)
}

// WorkspaceReader is the host façade composition. Individual use cases can
// depend on the aggregate/query-specific ports above.
type WorkspaceReader interface {
	FolderReader
	EnvironmentReader
	NodeReader
	WorkflowReader
	TestTaskReader
	ExecutionEvidenceReader
	MaintenanceReader
	HealCandidateReader
	DashboardReader
}

type FolderWriter interface {
	SaveFolder(ctx context.Context, folder WorkspaceFolder) error
	DeleteFolder(ctx context.Context, id string) error
	MoveFolderItem(ctx context.Context, kind FolderKind, itemID, folderID string) error
}

type EnvironmentWriter interface {
	SaveEnvironment(ctx context.Context, environment Environment) error
	SetEnvironmentDeleted(ctx context.Context, id string, deleted bool, at int64) error
}

type NodeWriter interface {
	SaveNode(ctx context.Context, aggregate NodeAggregate, publishVersion bool) error
	SetNodeArchived(ctx context.Context, id string, archived bool, at int64) error
}

type WorkflowWriter interface {
	SaveWorkflow(ctx context.Context, aggregate WorkflowAggregate, publishVersion bool) error
	SetWorkflowArchived(ctx context.Context, id string, archived bool, at int64) error
}

type TestTaskWriter interface {
	UpdateTestTask(ctx context.Context, task TestTask) error
	PublishTestTaskVersion(ctx context.Context, plan TestTaskVersionPlan) (TestTaskAggregate, error)
	SetTestTaskDeleted(ctx context.Context, id string, deleted bool, at int64) error
}

type TestRunWriter interface {
	CreateTestTaskRun(ctx context.Context, plan TestTaskRunPlan) error
	CancelTestTaskRun(ctx context.Context, runID string, at int64) error
	ReorderTestTaskRunQueue(ctx context.Context, runIDs []string) error
	DeleteTestTaskRun(ctx context.Context, runID string) error
	RecoverInterruptedRuns(ctx context.Context, at int64) error
	ClaimNextTestTaskRun(ctx context.Context, at int64) (TestTaskRunPlan, bool, error)
	StartWorkflowExecution(ctx context.Context, executionID string, at int64) error
	FinishWorkflowExecution(ctx context.Context, executionID string, status ExecutionStatus, at int64) error
	FailTestTaskRun(ctx context.Context, runID, executionID string, at int64) error
	FinalizeTestTaskRun(ctx context.Context, runID string, status TestTaskRunStatus, at int64) error
}

type EvidenceWriter interface {
	SaveRecording(ctx context.Context, recording Recording) error
	DeleteRecording(ctx context.Context, executionID string) error
	SaveNetworkEvidence(ctx context.Context, evidence NetworkEvidence) error
}

type MaintenanceWriter interface {
	CleanupStaleHealCandidates(ctx context.Context) error
	SetParameterSnapshotDeleted(ctx context.Context, snapshotID string, deleted bool, at int64) error
}

// WorkspaceWriter is the host façade composition. Persistence guards remain a
// safety net; transition policy stays in domain behavior.
type WorkspaceWriter interface {
	FolderWriter
	EnvironmentWriter
	NodeWriter
	WorkflowWriter
	TestTaskWriter
	TestRunWriter
	EvidenceWriter
	MaintenanceWriter
}

// Store is the composition-root contract for adapters implementing both sides.
// Consumers should depend on WorkspaceReader, WorkspaceWriter or a narrower
// use-case-local interface instead of Store whenever possible.
type Store interface {
	WorkspaceReader
	WorkspaceWriter
}

// ExecutionFactCommitter owns the atomic Step/validation/healing fact
// boundary used by the application execution coordinator.
type ExecutionFactCommitter interface {
	CommitStepTransition(context.Context, StepTransitionCommit) (StepTransitionCommitResult, error)
}

// ExecutionProgressWriter owns non-terminal execution facts and the narrow
// post-commit error attachment operation. Terminal transitions and their
// final facts must go through ExecutionFactCommitter.
type ExecutionProgressWriter interface {
	RecordStepProgress(context.Context, StepPhaseEvent) error
	RecordValidationProgress(context.Context, ValidationObservation) error
	AttachTerminalStepError(context.Context, StepPhaseEvent) error
}

// HealCandidateGovernance exposes the strong Candidate/NodeVersion approval
// protocol without granting unrelated workspace mutations.
type HealCandidateGovernance interface {
	ApproveHealCandidate(context.Context, HealCandidateReviewCommand) (string, error)
	RejectHealCandidate(context.Context, HealCandidateReviewCommand) error
}

// SamplingPublisher persists one temporary Workflow and all of its Node
// decisions in a single transaction. It is separate from Store so sampling
// remains an optional application capability in unit tests.
type SamplingPublisher interface {
	PublishSamplingWorkflow(context.Context, SamplingPublication) (SamplingPublicationResult, error)
}

type ParameterSnapshotReader interface {
	GetParameterSnapshot(context.Context, string) (ExecutionParameterSnapshot, error)
}

type ParameterSnapshotUsageStore interface {
	ListParameterSnapshotUsages(context.Context, string) ([]ParameterSnapshotUsage, error)
}

type VersionStore interface {
	SetNodeVersionDeleted(context.Context, string, string, bool, int64) error
	SetWorkflowVersionDeleted(context.Context, string, string, bool, int64) error
}

type CleanupPreviewStore interface {
	CleanupPreview(context.Context, int64) (CleanupPreview, error)
}
