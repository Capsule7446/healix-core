package workspace

import (
	"strings"
	"testing"
)

func TestTestTaskVersionPlanValidatesCompleteDependencyGraphMatrix(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*TestTaskVersionPlan)
		want   string
	}{
		{name: "exact graph"},
		{name: "missing exact node version", mutate: func(plan *TestTaskVersionPlan) { plan.Nodes = plan.Nodes[1:] }, want: "no exact node dependency"},
		{name: "orphan node snapshot", mutate: func(plan *TestTaskVersionPlan) {
			plan.Nodes = append(plan.Nodes, graphNodeDependency("orphan", "orphan-v1", 1))
		}, want: "orphan snapshots"},
		{name: "missing reference resolution", mutate: func(plan *TestTaskVersionPlan) { plan.References = nil }, want: "no workflow reference resolution"},
		{name: "orphan reference resolution", mutate: func(plan *TestTaskVersionPlan) {
			plan.References = append(plan.References, WorkflowReferenceResolution{ParentWorkflowVersionID: "root-v1", StepID: "not-a-step", WorkflowID: "child", WorkflowVersionID: "child-v1"})
		}, want: "orphan snapshots"},
		{name: "reference target workflow changed", mutate: func(plan *TestTaskVersionPlan) { plan.References[0].WorkflowID = "other" }, want: "target is inconsistent"},
		{name: "fixed reference resolved as latest", mutate: func(plan *TestTaskVersionPlan) { plan.References[0].ResolvedFromLatest = true }, want: "fixed workflow reference changed version"},
		{name: "fixed reference changed version", mutate: func(plan *TestTaskVersionPlan) { plan.Workflows[1].Version.ID = "child-v2" }, want: "target is inconsistent"},
		{name: "orphan workflow snapshot", mutate: func(plan *TestTaskVersionPlan) {
			plan.Workflows = append(plan.Workflows, makeGraphWorkflowDependency("orphan", "orphan-v1", false,
				WorkflowStep{ID: "orphan-wait", DisplayName: "等待", Kind: StepWait, WaitMS: 1}))
		}, want: "orphan snapshots"},
		{name: "duplicate workflow version identity", mutate: func(plan *TestTaskVersionPlan) {
			duplicate := makeGraphWorkflowDependency("other", "child-v1", false, WorkflowStep{ID: "wait", DisplayName: "等待", Kind: StepWait, WaitMS: 1})
			plan.Workflows = append(plan.Workflows, duplicate)
		}, want: "duplicate workflow version dependency"},
		{name: "workflow cycle", mutate: func(plan *TestTaskVersionPlan) {
			plan.Workflows[1].Version.Definition.Steps = append(plan.Workflows[1].Version.Definition.Steps,
				WorkflowStep{ID: "back-root", DisplayName: "返回根流程", Kind: StepWorkflowRef,
					Reference: &WorkflowReference{WorkflowID: "root", WorkflowVersionID: "root-v1"}})
			plan.References = append(plan.References, WorkflowReferenceResolution{ParentWorkflowVersionID: "child-v1",
				StepID: "back-root", WorkflowID: "root", WorkflowVersionID: "root-v1"})
		}, want: "cycle includes"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := completeGraphVersionPlan()
			if test.mutate != nil {
				test.mutate(&plan)
			}
			err := plan.Validate()
			if test.want == "" && err != nil {
				t.Fatalf("valid dependency graph rejected: %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestTestTaskVersionPlanValidatesLatestNestedReferenceResolution(t *testing.T) {
	plan := completeGraphVersionPlan()
	reference := plan.Workflows[0].Version.Definition.Steps[1].Reference
	reference.WorkflowVersionID = ""
	reference.LatestPublished = true
	plan.References[0].ResolvedFromLatest = true
	if err := plan.Validate(); err != nil {
		t.Fatalf("latest nested reference rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*TestTaskVersionPlan)
	}{
		{name: "resolution does not record latest", mutate: func(plan *TestTaskVersionPlan) { plan.References[0].ResolvedFromLatest = false }},
		{name: "resolved version is no longer current", mutate: func(plan *TestTaskVersionPlan) { plan.Workflows[1].Workflow.CurrentVersionID = "child-v2" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invalid := completeGraphVersionPlan()
			invalid.Workflows[0].Version.Definition.Steps[1].Reference.WorkflowVersionID = ""
			invalid.Workflows[0].Version.Definition.Steps[1].Reference.LatestPublished = true
			invalid.References[0].ResolvedFromLatest = true
			test.mutate(&invalid)
			if err := invalid.Validate(); err == nil || !strings.Contains(err.Error(), "latest workflow reference") {
				t.Fatalf("invalid latest resolution error = %v", err)
			}
		})
	}
}

func TestTestTaskVersionPlanValidatesPublicationCandidateMatrix(t *testing.T) {
	t.Run("fixed top-level version", func(t *testing.T) {
		plan := completeGraphVersionPlan()
		plan.Version.Items[0].VersionPolicy = WorkflowVersionFixed
		plan.Version.Items[0].WorkflowVersionID = "root-v1"
		plan.Workflows[0].ResolvedFromLatest = false
		if err := plan.Validate(); err != nil {
			t.Fatalf("fixed top-level dependency rejected: %v", err)
		}
	})

	t.Run("subsequent version", func(t *testing.T) {
		plan := completeGraphVersionPlan()
		plan.Task.CurrentVersionID = "task-v2"
		plan.Task.UpdatedAt = 2
		plan.ExpectedTaskUpdatedAt = 1
		plan.Version.ID = "task-v2"
		plan.Version.VersionNumber = 2
		plan.Version.SourceVersionID = "task-v1"
		plan.Version.CreatedAt = 2
		plan.Version.Items[0].ID = "item-v2"
		plan.Version.Items[0].TestTaskVersionID = "task-v2"
		if err := plan.Validate(); err != nil {
			t.Fatalf("subsequent publication rejected: %v", err)
		}
	})

	tests := []struct {
		name   string
		mutate func(*TestTaskVersionPlan)
		want   string
	}{
		{name: "task owner mismatch", mutate: func(plan *TestTaskVersionPlan) { plan.Version.TestTaskID = "other" }, want: "candidate identity is inconsistent"},
		{name: "task current pointer mismatch", mutate: func(plan *TestTaskVersionPlan) { plan.Task.CurrentVersionID = "other" }, want: "candidate identity is inconsistent"},
		{name: "first version cannot carry expected timestamp", mutate: func(plan *TestTaskVersionPlan) { plan.ExpectedTaskUpdatedAt = 1 }, want: "version 1 without a source"},
		{name: "first version cannot carry source", mutate: func(plan *TestTaskVersionPlan) { plan.Version.SourceVersionID = "task-v0" }, want: "version 1 without a source"},
		{name: "subsequent version needs source", mutate: func(plan *TestTaskVersionPlan) {
			makeSubsequentVersionPlan(plan)
			plan.Version.SourceVersionID = ""
		}, want: "requires source"},
		{name: "subsequent version needs expected timestamp", mutate: func(plan *TestTaskVersionPlan) {
			makeSubsequentVersionPlan(plan)
			plan.ExpectedTaskUpdatedAt = 0
		}, want: "requires source"},
		{name: "fixed item needs matching dependency version", mutate: func(plan *TestTaskVersionPlan) {
			plan.Version.Items[0].VersionPolicy = WorkflowVersionFixed
			plan.Version.Items[0].WorkflowVersionID = "root-v2"
		}, want: "no matching workflow dependency"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := completeGraphVersionPlan()
			test.mutate(&plan)
			if err := plan.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestTestTaskVersionPlanRejectsDependencySnapshotIdentityMatrix(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*TestTaskVersionPlan)
		want   string
	}{
		{name: "workflow owner mismatch", mutate: func(plan *TestTaskVersionPlan) { plan.Workflows[0].Version.WorkflowID = "other" }, want: "workflow dependency snapshot identity"},
		{name: "workflow version number missing", mutate: func(plan *TestTaskVersionPlan) { plan.Workflows[0].Version.VersionNumber = 0 }, want: "workflow dependency snapshot identity"},
		{name: "duplicate workflow snapshot", mutate: func(plan *TestTaskVersionPlan) { plan.Workflows = append(plan.Workflows, plan.Workflows[0]) }, want: "duplicate workflow dependency snapshot"},
		{name: "node owner mismatch", mutate: func(plan *TestTaskVersionPlan) { plan.Nodes[0].Version.NodeID = "other" }, want: "node dependency snapshot identity"},
		{name: "node version number missing", mutate: func(plan *TestTaskVersionPlan) { plan.Nodes[0].Version.VersionNumber = 0 }, want: "node dependency snapshot identity"},
		{name: "duplicate node snapshot", mutate: func(plan *TestTaskVersionPlan) { plan.Nodes = append(plan.Nodes, plan.Nodes[0]) }, want: "duplicate node dependency snapshot"},
		{name: "incomplete reference identity", mutate: func(plan *TestTaskVersionPlan) { plan.References[0].StepID = "" }, want: "reference resolution identity is incomplete"},
		{name: "duplicate reference identity", mutate: func(plan *TestTaskVersionPlan) { plan.References = append(plan.References, plan.References[0]) }, want: "duplicate workflow reference resolution"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := completeGraphVersionPlan()
			test.mutate(&plan)
			if err := plan.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestTestTaskRunPlanValidatesFrozenShapeMatrix(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*TestTaskRunPlan)
		want   string
	}{
		{name: "complete frozen plan"},
		{name: "task identity mismatch", mutate: func(plan *TestTaskRunPlan) { plan.Run.TestTaskID = "other" }, want: "task and version identities"},
		{name: "environment identity mismatch", mutate: func(plan *TestTaskRunPlan) { plan.Run.EnvironmentID = "other" }, want: "environment identity"},
		{name: "missing execution", mutate: func(plan *TestTaskRunPlan) { plan.Executions = nil }, want: "exactly one execution"},
		{name: "workflow count mismatch", mutate: func(plan *TestTaskRunPlan) { plan.Run.WorkflowCount = 2 }, want: "workflow count"},
		{name: "duplicate execution id", mutate: func(plan *TestTaskRunPlan) {
			plan.Version.Items = append(plan.Version.Items, TestTaskItem{ID: "item-2", TestTaskVersionID: plan.Version.ID, SequenceNumber: 2, WorkflowID: "root", VersionPolicy: WorkflowVersionLatest})
			plan.Executions = append(plan.Executions, plan.Executions[0])
			plan.Executions[1].TestTaskItemID = "item-2"
			plan.Executions[1].SequenceNumber = 2
			plan.Run.WorkflowCount = 2
		}, want: "execution identities"},
		{name: "execution item mismatch", mutate: func(plan *TestTaskRunPlan) { plan.Executions[0].TestTaskItemID = "other" }, want: "does not match"},
		{name: "execution has no frozen workflow", mutate: func(plan *TestTaskRunPlan) { plan.Executions[0].WorkflowVersionID = "root-v2" }, want: "no frozen workflow dependency"},
		{name: "parameter scope item missing", mutate: func(plan *TestTaskRunPlan) { plan.Parameters[0].TestTaskItemID = "other" }, want: "scope identity"},
		{name: "parameter scope dependency missing", mutate: func(plan *TestTaskRunPlan) { plan.Parameters[0].WorkflowVersionID = "root-v2" }, want: "no frozen workflow dependency"},
		{name: "duplicate parameter occurrence scope", mutate: func(plan *TestTaskRunPlan) { plan.Parameters = append(plan.Parameters, plan.Parameters[0]) }, want: "scope identity"},
		{name: "new run must be queued", mutate: func(plan *TestTaskRunPlan) { plan.Run.Status = RunRunning }, want: "new run must be QUEUED"},
		{name: "new run cannot have start timestamp", mutate: func(plan *TestTaskRunPlan) { plan.Run.StartedAt = 3 }, want: "new run must be QUEUED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := completeGraphRunPlan()
			if test.mutate != nil {
				test.mutate(&plan)
			}
			err := plan.ValidateForCreation()
			if test.want == "" && err != nil {
				t.Fatalf("valid run plan rejected: %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("ValidateForCreation() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestTestTaskRunPlanAllowsRepeatedWorkflowOccurrences(t *testing.T) {
	publication := validTestTaskVersionPlan()
	publication.Version.Items = []TestTaskItem{
		{ID: "item-1", TestTaskVersionID: publication.Version.ID, SequenceNumber: 1, WorkflowID: "workflow", VersionPolicy: WorkflowVersionLatest},
		{ID: "item-2", TestTaskVersionID: publication.Version.ID, SequenceNumber: 2, WorkflowID: "workflow", VersionPolicy: WorkflowVersionLatest},
	}
	plan := TestTaskRunPlan{
		Run: TestTaskRun{ID: "run", TestTaskID: publication.Task.ID, TestTaskVersionID: publication.Version.ID,
			TestTaskVersionNumber: 1, Status: RunQueued, EnvironmentID: "environment", WorkflowCount: 2, CreatedAt: 1, QueuedAt: 1},
		Task: publication.Task, Version: publication.Version,
		Environment: EnvironmentSnapshot{ID: "environment"}, Workflows: publication.Workflows,
		Executions: []WorkflowExecutionPlan{
			{ID: "execution-1", TestTaskItemID: "item-1", SequenceNumber: 1, WorkflowID: "workflow", WorkflowVersionID: "workflow-v1", WorkflowVersionNumber: 1, VersionPolicy: WorkflowVersionLatest},
			{ID: "execution-2", TestTaskItemID: "item-2", SequenceNumber: 2, WorkflowID: "workflow", WorkflowVersionID: "workflow-v1", WorkflowVersionNumber: 1, VersionPolicy: WorkflowVersionLatest},
		},
	}
	if err := plan.ValidateForCreation(); err != nil {
		t.Fatalf("repeated workflow occurrences rejected: %v", err)
	}
}

func completeGraphVersionPlan() TestTaskVersionPlan {
	root := makeGraphWorkflowDependency("root", "root-v1", true,
		WorkflowStep{ID: "root-click", DisplayName: "根节点", Kind: StepAction, Action: "click", NodeID: "shared", NodeVersionID: "shared-v1"},
		WorkflowStep{ID: "call-child", DisplayName: "调用子流程", Kind: StepWorkflowRef,
			Reference: &WorkflowReference{WorkflowID: "child", WorkflowVersionID: "child-v1"}},
		WorkflowStep{ID: "repeat", DisplayName: "循环", Kind: StepRepeat, RepeatCount: 1,
			Children: []WorkflowStep{{ID: "nested-click", DisplayName: "新版节点", Kind: StepAction, Action: "click", NodeID: "shared", NodeVersionID: "shared-v2"}}},
	)
	child := makeGraphWorkflowDependency("child", "child-v1", false,
		WorkflowStep{ID: "child-click", DisplayName: "子节点", Kind: StepAction, Action: "click", NodeID: "child-node", NodeVersionID: "child-node-v1"})
	task := TestTask{ID: "task", DisplayName: "任务", CurrentVersionID: "task-v1", CreatedAt: 1, UpdatedAt: 1}
	version := TestTaskVersion{ID: "task-v1", TestTaskID: task.ID, VersionNumber: 1, CreatedAt: 1,
		Items: []TestTaskItem{{ID: "item", TestTaskVersionID: "task-v1", SequenceNumber: 1,
			WorkflowID: "root", VersionPolicy: WorkflowVersionLatest}}}
	return TestTaskVersionPlan{
		Task: task, Version: version,
		Workflows: []WorkflowDependencySnapshot{root, child},
		Nodes: []NodeDependencySnapshot{
			graphNodeDependency("shared", "shared-v1", 1),
			graphNodeDependency("shared", "shared-v2", 2),
			graphNodeDependency("child-node", "child-node-v1", 1),
		},
		References: []WorkflowReferenceResolution{{ParentWorkflowVersionID: "root-v1", StepID: "call-child",
			WorkflowID: "child", WorkflowVersionID: "child-v1"}},
	}
}

func completeGraphRunPlan() TestTaskRunPlan {
	publication := completeGraphVersionPlan()
	return TestTaskRunPlan{
		Run: TestTaskRun{ID: "run", TestTaskID: publication.Task.ID, TestTaskVersionID: publication.Version.ID,
			TestTaskVersionNumber: publication.Version.VersionNumber, Status: RunQueued, EnvironmentID: "environment",
			WorkflowCount: 1, CreatedAt: 1, QueuedAt: 1},
		Task: publication.Task, Version: publication.Version,
		Environment: EnvironmentSnapshot{ID: "environment"}, Workflows: publication.Workflows,
		Nodes: publication.Nodes, References: publication.References,
		Parameters: []WorkflowParameterScope{{TestTaskItemID: "item", Path: "root", WorkflowID: "root", WorkflowVersionID: "root-v1", Values: ParameterValues{}}},
		Executions: []WorkflowExecutionPlan{{ID: "execution", TestTaskItemID: "item", SequenceNumber: 1,
			WorkflowID: "root", WorkflowVersionID: "root-v1", WorkflowVersionNumber: 1, VersionPolicy: WorkflowVersionLatest}},
	}
}

func makeSubsequentVersionPlan(plan *TestTaskVersionPlan) {
	plan.Task.CurrentVersionID = "task-v2"
	plan.Task.UpdatedAt = 2
	plan.ExpectedTaskUpdatedAt = 1
	plan.Version.ID = "task-v2"
	plan.Version.VersionNumber = 2
	plan.Version.SourceVersionID = "task-v1"
	plan.Version.CreatedAt = 2
	plan.Version.Items[0].ID = "item-v2"
	plan.Version.Items[0].TestTaskVersionID = "task-v2"
}

func makeGraphWorkflowDependency(workflowID, versionID string, latest bool, steps ...WorkflowStep) WorkflowDependencySnapshot {
	return WorkflowDependencySnapshot{
		Workflow: Workflow{ID: workflowID, DisplayName: workflowID, Properties: Properties{}, CurrentVersionID: versionID, CreatedAt: 1, UpdatedAt: 1},
		Version: WorkflowVersion{ID: versionID, WorkflowID: workflowID, VersionNumber: 1,
			Definition: WorkflowDefinition{Steps: steps}, CreatedAt: 1},
		ResolvedFromLatest: latest,
	}
}

func graphNodeDependency(nodeID, versionID string, number int) NodeDependencySnapshot {
	return NodeDependencySnapshot{Node: Node{ID: nodeID, DisplayName: nodeID, CurrentVersionID: versionID},
		Version: NodeVersion{ID: versionID, NodeID: nodeID, VersionNumber: number}}
}
