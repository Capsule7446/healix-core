package workspace

import (
	"strings"
	"testing"
)

func validTestTaskVersionPlan() TestTaskVersionPlan {
	workflow := Workflow{ID: "workflow", DisplayName: "流程", Properties: Properties{},
		CurrentVersionID: "workflow-v1", CreatedAt: 1, UpdatedAt: 1}
	workflowVersion := WorkflowVersion{ID: "workflow-v1", WorkflowID: workflow.ID, VersionNumber: 1,
		Definition: WorkflowDefinition{Steps: []WorkflowStep{{ID: "wait", DisplayName: "等待",
			Kind: StepWait, WaitKind: "sleep", WaitMS: 1}}}, CreatedAt: 1}
	task := TestTask{ID: "task", DisplayName: "任务", CurrentVersionID: "task-v1", CreatedAt: 1, UpdatedAt: 1}
	version := TestTaskVersion{ID: "task-v1", TestTaskID: task.ID, VersionNumber: 1, CreatedAt: 1,
		Items: []TestTaskItem{{ID: "item", TestTaskVersionID: "task-v1", SequenceNumber: 1,
			WorkflowID: workflow.ID, VersionPolicy: WorkflowVersionLatest, Parameters: ParameterValues{}}}}
	return TestTaskVersionPlan{Task: task, Version: version, Workflows: []WorkflowDependencySnapshot{{
		Workflow: workflow, Version: workflowVersion, ResolvedFromLatest: true}}}
}

func TestTestTaskAggregateOwnsCurrentAndHistoryConsistency(t *testing.T) {
	plan := validTestTaskVersionPlan()
	v1 := plan.Version
	v2 := v1
	v2.ID = "task-v2"
	v2.VersionNumber = 2
	v2.SourceVersionID = v1.ID
	v2.CreatedAt = 2
	v2.Items = append([]TestTaskItem(nil), v1.Items...)
	v2.Items[0].ID = "item-v2"
	v2.Items[0].TestTaskVersionID = v2.ID
	task := plan.Task
	task.CurrentVersionID = v2.ID
	task.UpdatedAt = 2
	aggregate := TestTaskAggregate{Task: task, Current: v2, Versions: []TestTaskVersion{v2, v1}}
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

func TestTestTaskRunPlanOwnsFrozenExecutionShape(t *testing.T) {
	publication := validTestTaskVersionPlan()
	plan := TestTaskRunPlan{
		Run: TestTaskRun{ID: "run", TestTaskID: publication.Task.ID, TestTaskVersionID: publication.Version.ID,
			TestTaskVersionNumber: 1, DisplayName: "任务", TriggerSource: "MANUAL", Status: RunQueued,
			EnvironmentID: "environment", WorkflowCount: 1, CreatedAt: 1, QueuedAt: 1},
		Task: publication.Task, Version: publication.Version,
		Environment: EnvironmentSnapshot{ID: "environment", DisplayName: "环境", UpdatedAt: 1},
		Workflows:   publication.Workflows,
		Executions: []WorkflowExecutionPlan{{ID: "execution", TestTaskItemID: "item", SequenceNumber: 1,
			WorkflowID: "workflow", WorkflowVersionID: "workflow-v1", WorkflowVersionNumber: 1,
			VersionPolicy: WorkflowVersionLatest, WorkflowName: "流程", StepCount: 1}},
	}
	if err := plan.ValidateForCreation(); err != nil {
		t.Fatalf("valid run plan: %v", err)
	}
	plan.Executions[0].SequenceNumber = 2
	if err := plan.ValidateForCreation(); err == nil || !strings.Contains(err.Error(), "sequence numbers") {
		t.Fatalf("invalid sequence error = %v", err)
	}
}
