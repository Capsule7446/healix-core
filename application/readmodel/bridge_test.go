package readmodel

import (
	"context"
	"testing"

	"github.com/Capsule7446/healix-core/domain/workspace"
)

type fakeNodeReader struct{ items []workspace.NodeQueryResult }

func (f fakeNodeReader) ListNodes(context.Context, bool) ([]workspace.NodeQueryResult, error) {
	return f.items, nil
}
func (f fakeNodeReader) GetNode(context.Context, string) (workspace.NodeQueryResult, error) {
	return f.items[0], nil
}

type fakeWorkflowReader struct {
	items []workspace.WorkflowQueryResult
}

func (f fakeWorkflowReader) ListWorkflows(context.Context, bool) ([]workspace.WorkflowQueryResult, error) {
	return f.items, nil
}
func (f fakeWorkflowReader) GetWorkflow(context.Context, string) (workspace.WorkflowQueryResult, error) {
	return f.items[0], nil
}

type fakeTaskReader struct {
	items []workspace.TestTaskQueryResult
}

func (f fakeTaskReader) ListTestTasks(context.Context, bool) ([]workspace.TestTaskQueryResult, error) {
	return f.items, nil
}
func (f fakeTaskReader) GetTestTask(context.Context, string) (workspace.TestTaskQueryResult, error) {
	return f.items[0], nil
}
func (f fakeTaskReader) GetTestTaskVersion(context.Context, string) (workspace.TestTaskVersion, error) {
	return workspace.TestTaskVersion{}, nil
}
func (f fakeTaskReader) ListTestTaskRuns(context.Context, string) ([]workspace.TestTaskRun, error) {
	return nil, nil
}
func (f fakeTaskReader) ListAllTestTaskRuns(context.Context) ([]workspace.TestTaskRun, error) {
	return nil, nil
}
func (f fakeTaskReader) GetTestTaskRunDetail(context.Context, string) (workspace.TestTaskRunDetail, error) {
	return workspace.TestTaskRunDetail{}, nil
}
func (f fakeTaskReader) TestTaskRunResourceURIs(context.Context, string) ([]string, error) {
	return nil, nil
}

type fakeDashboardReader struct{}

func (fakeDashboardReader) Dashboard(context.Context) (workspace.Dashboard, error) {
	return workspace.Dashboard{StatusCounts: map[workspace.TestTaskRunStatus]int{workspace.RunSucceeded: 2}}, nil
}

func TestLegacyReaderBridgeMapsQueryResultsWithoutAggregates(t *testing.T) {
	bridge := NewLegacyReaderBridge(
		fakeNodeReader{items: []workspace.NodeQueryResult{{NodeAggregate: workspace.NodeAggregate{Node: workspace.Node{ID: "node", DisplayName: "按钮"}, Current: workspace.NodeVersion{ID: "node-v2", VersionNumber: 2}}, RefCount: 4}}},
		fakeWorkflowReader{items: []workspace.WorkflowQueryResult{{WorkflowAggregate: workspace.WorkflowAggregate{Workflow: workspace.Workflow{ID: "workflow", DisplayName: "登录"}, Current: workspace.WorkflowVersion{ID: "workflow-v3", VersionNumber: 3}}, LastRunStatus: "SUCCEEDED", LastRunAt: 10}}},
		fakeTaskReader{items: []workspace.TestTaskQueryResult{{TestTaskAggregate: workspace.TestTaskAggregate{Task: workspace.TestTask{ID: "task", DisplayName: "回归"}, Current: workspace.TestTaskVersion{ID: "task-v4", VersionNumber: 4}}, LastRunStatus: workspace.RunFailed, LastRunAt: 11}}},
		fakeDashboardReader{},
	)
	nodes, err := bridge.ListNodes(context.Background(), false)
	if err != nil || len(nodes) != 1 || nodes[0].CurrentVersion != "node-v2" || nodes[0].RefCount != 4 {
		t.Fatalf("unexpected node view: %+v, err=%v", nodes, err)
	}
	tasks, err := bridge.ListTestTasks(context.Background(), false)
	if err != nil || len(tasks) != 1 || tasks[0].LastRunStatus != "FAILED" {
		t.Fatalf("unexpected task view: %+v, err=%v", tasks, err)
	}
	dashboard, err := bridge.Dashboard(context.Background())
	if err != nil || dashboard.StatusCounts["SUCCEEDED"] != 2 {
		t.Fatalf("unexpected dashboard: %+v, err=%v", dashboard, err)
	}
}
