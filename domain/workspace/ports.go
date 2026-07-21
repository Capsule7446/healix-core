package workspace

import (
	"context"
	"errors"
)

var (
	// ErrNotFound 表示目标不存在。适配器会用实体类型和 ID 包装此错误，
	// 以便调用方通过 errors.Is 区分“缺失”与存储故障，避免将两者归为同一业务结论。
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

type TestTaskReader interface {
	GetTestTaskVersion(ctx context.Context, versionID string) (TestTaskVersion, error)
	ListTestTaskRuns(ctx context.Context, testTaskID string) ([]TestTaskRun, error)
	ListAllTestTaskRuns(ctx context.Context) ([]TestTaskRun, error)
	TestTaskRunResourceURIs(ctx context.Context, runID string) ([]string, error)
}

type MaintenanceReader interface {
	ExpiredTestTaskRunIDs(ctx context.Context, cutoff int64) ([]string, error)
	StorageStats(ctx context.Context) (StorageStats, error)
	ListParameterSnapshots(ctx context.Context, includeDeleted bool) ([]ExecutionParameterSnapshot, error)
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

type MaintenanceWriter interface {
	CleanupStaleHealCandidates(ctx context.Context) error
	SetParameterSnapshotDeleted(ctx context.Context, snapshotID string, deleted bool, at int64) error
}

// WorkspaceWriter 是主机外观组合。持久性守卫仍然是一个安全网；转换策略保留在域行为中。
type WorkspaceWriter interface {
	FolderWriter
	EnvironmentWriter
	NodeWriter
	WorkflowWriter
	TestTaskWriter
	TestRunWriter
	MaintenanceWriter
}

// Store 是实现双方的适配器的组合根契约。读侧查询通过 application/readmodel 提供；执行事实通过 application/execution 提供。
type Store interface {
	WorkspaceWriter
}

// HealCandidateGovernance 公开了强大的 Candidate/NodeVersion 批准协议，而不授予不相关的工作空间突变。
type HealCandidateGovernance interface {
	ApproveHealCandidate(context.Context, HealCandidateReviewCommand) (string, error)
	RejectHealCandidate(context.Context, HealCandidateReviewCommand) error
}

// SamplingPublisher 在单个事务中保留一个临时工作流及其所有节点决策。它与 Store 是分开的，因此采样仍然是单元测试中的可选应用程序功能。
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
