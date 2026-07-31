package automation

import (
	"github.com/Capsule7446/healix-core/domain/parameter"
	"strings"
	"testing"
)

func validTestTaskVersionPlan() ResolvedExecutionFlow {
	workflow := FlowFragment{ID: "workflow", DisplayName: "流程", Properties: Properties{},
		CurrentVersionID: "workflow-v1", CreatedAt: 1, UpdatedAt: 1}
	workflowVersion := FlowFragmentVersion{ID: "workflow-v1", FlowFragmentID: workflow.ID, VersionNumber: 1,
		Definition: FlowFragmentContent{Steps: []FlowFragmentStep{{ID: "wait", DisplayName: "等待",
			Kind: StepWait, WaitKind: "sleep", WaitMS: 1}}}, CreatedAt: 1}
	task := ExecutionFlow{ID: "task", DisplayName: "任务", CurrentVersionID: "task-v1", CreatedAt: 1, UpdatedAt: 1}
	version := ExecutionFlowVersion{ID: "task-v1", ExecutionFlowID: task.ID, VersionNumber: 1, CreatedAt: 1, FailurePolicy: FailurePolicyStopOnFailure,
		Items: []ExecutionFlowItem{{ID: "item", TestTaskVersionID: "task-v1", SequenceNumber: 1,
			FlowFragmentID: workflow.ID, VersionPolicy: FlowFragmentVersionLatest, Parameters: map[string]parameter.Value{}}}}
	return ResolvedExecutionFlow{Task: task, Version: version, Workflows: []FlowFragmentDependencySnapshot{{
		FlowFragment: workflow, Version: workflowVersion, ResolvedFromLatest: true}}}
}

func TestTestTaskAggregateOwnsCurrentAndHistoryConsistency(t *testing.T) {
	plan := validTestTaskVersionPlan()
	v1 := plan.Version
	v2 := v1
	v2.ID = "task-v2"
	v2.VersionNumber = 2
	v2.SourceVersionID = v1.ID
	v2.CreatedAt = 2
	v2.Items = append([]ExecutionFlowItem(nil), v1.Items...)
	v2.Items[0].ID = "item-v2"
	v2.Items[0].TestTaskVersionID = v2.ID
	task := plan.Task
	task.CurrentVersionID = v2.ID
	task.UpdatedAt = 2
	aggregate := ExecutionFlowAggregate{Task: task, Current: v2, Versions: []ExecutionFlowVersion{v2, v1}}
	if err := aggregate.Validate(); err != nil {
		t.Fatalf("valid aggregate: %v", err)
	}
	aggregate.Task.CurrentVersionID = v1.ID
	if err := aggregate.Validate(); err == nil || !strings.Contains(err.Error(), "current version") {
		t.Fatalf("inconsistent current error = %v", err)
	}
}

func TestTestTaskVersionPlanValidatesResolvedTopLevelDependency(t *testing.T) {
	plan := validTestTaskVersionPlan()
	if err := plan.Validate(); err != nil {
		t.Fatalf("valid plan: %v", err)
	}
	plan.Workflows[0].ResolvedFromLatest = false
	if err := plan.Validate(); err == nil || !strings.Contains(err.Error(), "matching workflow dependency") {
		t.Fatalf("unresolved latest error = %v", err)
	}
}

func TestResolvedExecutionFlowKeepsExactVersionsAfterCurrentPointersAdvance(t *testing.T) {
	plan := validTestTaskVersionPlan()
	plan.Task.CurrentVersionID = "task-v2"
	plan.Workflows[0].FlowFragment.CurrentVersionID = "workflow-v2"

	if err := plan.Validate(); err != nil {
		t.Fatalf("resolved exact plan rejected after current pointers advanced: %v", err)
	}
}
