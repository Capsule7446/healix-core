package readmodel

import (
	"context"

	"github.com/Capsule7446/healix-core/domain/workspace"
)

// LegacyReaderBridge adapts the legacy workspace query ports to UI-safe read models.
// It keeps the old storage contract available while consumers migrate to readmodel.
type LegacyReaderBridge struct {
	nodes     workspace.NodeReader
	workflows workspace.WorkflowReader
	testTasks workspace.TestTaskReader
	dashboard workspace.DashboardReader
}

func NewLegacyReaderBridge(nodes workspace.NodeReader, workflows workspace.WorkflowReader, testTasks workspace.TestTaskReader, dashboard workspace.DashboardReader) LegacyReaderBridge {
	return LegacyReaderBridge{nodes: nodes, workflows: workflows, testTasks: testTasks, dashboard: dashboard}
}

func (b LegacyReaderBridge) ListNodes(ctx context.Context, includeDeleted bool) ([]NodeListItem, error) {
	items, err := b.nodes.ListNodes(ctx, includeDeleted)
	if err != nil {
		return nil, err
	}
	result := make([]NodeListItem, 0, len(items))
	for _, item := range items {
		result = append(result, nodeListItem(item))
	}
	return result, nil
}

func (b LegacyReaderBridge) GetNode(ctx context.Context, id string) (NodeListItem, error) {
	item, err := b.nodes.GetNode(ctx, id)
	if err != nil {
		return NodeListItem{}, err
	}
	return nodeListItem(item), nil
}

func (b LegacyReaderBridge) ListWorkflows(ctx context.Context, includeDeleted bool) ([]WorkflowListItem, error) {
	items, err := b.workflows.ListWorkflows(ctx, includeDeleted)
	if err != nil {
		return nil, err
	}
	result := make([]WorkflowListItem, 0, len(items))
	for _, item := range items {
		result = append(result, workflowListItem(item))
	}
	return result, nil
}

func (b LegacyReaderBridge) GetWorkflow(ctx context.Context, id string) (WorkflowListItem, error) {
	item, err := b.workflows.GetWorkflow(ctx, id)
	if err != nil {
		return WorkflowListItem{}, err
	}
	return workflowListItem(item), nil
}

func (b LegacyReaderBridge) ListTestTasks(ctx context.Context, includeDeleted bool) ([]TestTaskListItem, error) {
	items, err := b.testTasks.ListTestTasks(ctx, includeDeleted)
	if err != nil {
		return nil, err
	}
	result := make([]TestTaskListItem, 0, len(items))
	for _, item := range items {
		result = append(result, testTaskListItem(item))
	}
	return result, nil
}

func (b LegacyReaderBridge) GetTestTask(ctx context.Context, id string) (TestTaskListItem, error) {
	item, err := b.testTasks.GetTestTask(ctx, id)
	if err != nil {
		return TestTaskListItem{}, err
	}
	return testTaskListItem(item), nil
}

func (b LegacyReaderBridge) ListTestTaskRuns(ctx context.Context, id string) ([]RunListItem, error) {
	items, err := b.testTasks.ListTestTaskRuns(ctx, id)
	if err != nil {
		return nil, err
	}
	result := make([]RunListItem, 0, len(items))
	for _, item := range items {
		result = append(result, runListItem(item))
	}
	return result, nil
}

func (b LegacyReaderBridge) GetTestTaskRunDetail(ctx context.Context, id string) (ExecutionDetailView, error) {
	item, err := b.testTasks.GetTestTaskRunDetail(ctx, id)
	if err != nil {
		return ExecutionDetailView{}, err
	}
	return executionDetailView(item), nil
}

func (b LegacyReaderBridge) Dashboard(ctx context.Context) (DashboardView, error) {
	item, err := b.dashboard.Dashboard(ctx)
	if err != nil {
		return DashboardView{}, err
	}
	statusCounts := make(map[string]int, len(item.StatusCounts))
	for status, count := range item.StatusCounts {
		statusCounts[string(status)] = count
	}
	view := DashboardView{StatusCounts: statusCounts, Total30Days: item.Total30Days}
	view.Running = optionalRun(item.Running)
	view.Queue = runs(item.Queue)
	view.Runs = runs(item.Runs)
	view.TestTasks = make([]TestTaskListItem, 0, len(item.TestTasks))
	for _, task := range item.TestTasks {
		view.TestTasks = append(view.TestTasks, testTaskListItem(task))
	}
	return view, nil
}

func nodeListItem(item workspace.NodeQueryResult) NodeListItem {
	return NodeListItem{ID: item.Node.ID, DisplayName: item.Node.DisplayName, CurrentVersion: item.Current.ID, VersionNumber: item.Current.VersionNumber, RefCount: item.RefCount}
}

func workflowListItem(item workspace.WorkflowQueryResult) WorkflowListItem {
	return WorkflowListItem{ID: item.Workflow.ID, DisplayName: item.Workflow.DisplayName, CurrentVersion: item.Current.ID, VersionNumber: item.Current.VersionNumber, LastRunStatus: item.LastRunStatus, LastRunAt: item.LastRunAt}
}

func testTaskListItem(item workspace.TestTaskQueryResult) TestTaskListItem {
	return TestTaskListItem{ID: item.Task.ID, DisplayName: item.Task.DisplayName, CurrentVersion: item.Current.ID, VersionNumber: item.Current.VersionNumber, LastRunStatus: string(item.LastRunStatus), LastRunAt: item.LastRunAt}
}

func runListItem(item workspace.TestTaskRun) RunListItem {
	return RunListItem{ID: item.ID, TestTaskID: item.TestTaskID, Status: string(item.Status), QueuePosition: item.QueuePosition, CurrentWorkflowName: item.CurrentWorkflowName, CurrentStepName: item.CurrentStepName, EnvironmentID: item.EnvironmentID, EnvironmentName: item.EnvironmentName, EnvironmentURL: item.EnvironmentURL, CreatedAt: item.CreatedAt, QueuedAt: item.QueuedAt, StartedAt: item.StartedAt, FinishedAt: item.FinishedAt}
}

func runs(items []workspace.TestTaskRun) []RunListItem {
	result := make([]RunListItem, 0, len(items))
	for _, item := range items {
		result = append(result, runListItem(item))
	}
	return result
}

func optionalRun(item *workspace.TestTaskRun) *RunListItem {
	if item == nil {
		return nil
	}
	view := runListItem(*item)
	return &view
}

func executionDetailView(item workspace.TestTaskRunDetail) ExecutionDetailView {
	view := ExecutionDetailView{ID: item.Plan.Run.ID, RunID: item.Plan.Run.ID, Status: string(item.Plan.Run.Status)}
	view.Steps = make([]StepView, 0, len(item.Executions))
	for _, execution := range item.Executions {
		view.Steps = append(view.Steps, StepView{
			ID:          execution.Plan.ID,
			DisplayName: execution.Plan.WorkflowName,
			Phase:       string(execution.Status),
		})
	}
	return view
}
