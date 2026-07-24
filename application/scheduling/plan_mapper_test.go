package scheduling

import (
	"github.com/Capsule7446/healix-core/domain/parameter"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

func TestBuildExecutionDraftMapsOrderedRepeatedEntriesAndSnapshots(t *testing.T) {
	source := validMapperSource()

	plan, err := buildExecutionDraft(source)
	if err != nil {
		t.Fatal(err)
	}
	if plan.FailurePolicy != execution.FailurePolicyContinueOnFailure {
		t.Fatalf("draft policy = %q", plan.FailurePolicy)
	}
	entries := plan.Entries
	if len(entries) != 2 || entries[0].ExecutionID != "execution-1" || entries[0].SequenceNumber != 1 || entries[1].ExecutionID != "execution-2" || entries[1].SequenceNumber != 2 {
		t.Fatalf("entries = %#v", entries)
	}
	if entries[0].WorkflowVersionID != "root-v1" || entries[1].WorkflowVersionID != "root-v1" {
		t.Fatalf("resolved versions not preserved: %#v", entries)
	}
	if len(plan.Workflows) != 2 || len(plan.Nodes) != 1 || len(plan.References) != 1 {
		t.Fatalf("snapshot sizes = workflows %d, nodes %d, references %d", len(plan.Workflows), len(plan.Nodes), len(plan.References))
	}
	if !plan.References[0].ResolvedFromLatest {
		t.Fatal("LATEST reference provenance was lost")
	}
	for _, entry := range entries {
		if entry.WorkflowID == "child" {
			t.Fatalf("referenced workflow became an entry: %#v", entries)
		}
	}
}

func TestBuildExecutionDraftDoesNotManufactureNestedDefaultBinding(t *testing.T) {
	source := validMapperSource()
	child := &source.Publication.Workflows[1].Version.Definition
	child.Parameters = []automation.ParameterDefinition{{Name: "region", DisplayName: "Region", Type: parameter.Text, Default: parameter.PresentValue(parameter.TextValue("east"))}}
	plan, err := buildExecutionDraft(source)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := plan.Workflows[0].Steps[0].Reference.ParameterBindings["region"]; exists {
		t.Fatal("mapper manufactured nested default binding")
	}
}

func TestBuildExecutionDraftMapsDifferentFixedVersionsOfSameWorkflow(t *testing.T) {
	source := validMapperSource()
	rootV2 := source.Publication.Workflows[0]
	rootV2.Workflow.CurrentVersionID = "root-v2"
	rootV2.Version.ID = "root-v2"
	rootV2.Version.VersionNumber = 2
	rootV2.ResolvedFromLatest = false
	rootV2.Version.Definition.Steps = append([]automation.WorkflowStep(nil), rootV2.Version.Definition.Steps...)
	reference := *rootV2.Version.Definition.Steps[0].Reference
	rootV2.Version.Definition.Steps[0].Reference = &reference
	rootV2.Version.Definition.Steps[0].Reference.LatestPublished = false
	rootV2.Version.Definition.Steps[0].Reference.WorkflowVersionID = "child-v1"
	source.Publication.Workflows[0].ResolvedFromLatest = false
	source.Publication.Workflows = append(source.Publication.Workflows, rootV2)
	source.Publication.References = append(source.Publication.References, automation.WorkflowReferenceResolution{
		ParentWorkflowVersionID: "root-v2", StepID: "call-child", WorkflowID: "child", WorkflowVersionID: "child-v1",
	})
	source.Publication.Version.Items[0].VersionPolicy = automation.WorkflowVersionFixed
	source.Publication.Version.Items[0].WorkflowVersionID = "root-v1"
	source.Publication.Version.Items[1].VersionPolicy = automation.WorkflowVersionFixed
	source.Publication.Version.Items[1].WorkflowVersionID = "root-v2"
	source.Entries[1].WorkflowVersionID = "root-v2"

	plan, err := buildExecutionDraft(source)
	if err != nil {
		t.Fatal(err)
	}
	entries := plan.Entries
	if entries[0].WorkflowVersionID != "root-v1" || entries[1].WorkflowVersionID != "root-v2" {
		t.Fatalf("entry versions = %#v", entries)
	}
}

func TestBuildExecutionDraftRejectsMismatchAndInvalidSource(t *testing.T) {
	tests := []struct {
		name string
		edit func(*buildExecutionPlanInput)
		want string
	}{
		{"execution mismatch", func(plan *buildExecutionPlanInput) { plan.Entries[0].WorkflowVersionID = "child-v1" }, "entry"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := validMapperSource()
			test.edit(&source)
			plan, err := buildExecutionDraft(source)
			if err == nil || !strings.Contains(err.Error(), test.want) || plan.RunID != "" {
				t.Fatalf("plan/error = %#v/%v", plan, err)
			}
		})
	}
}

func TestBuildExecutionDraftMapsTypedParameterSnapshot(t *testing.T) {
	source := validMapperSource()
	number, err := parameter.NewNumberValue("01.00")
	if err != nil {
		t.Fatal(err)
	}
	source.Publication.Workflows[0].Version.Definition.Parameters = []automation.ParameterDefinition{{Name: "count", DisplayName: "Count", Description: "Run count", Type: parameter.Number, Required: true}}
	source.Publication.Version.Items[0].Parameters = map[string]parameter.Value{"count": number}
	source.Publication.Version.Items[1].Parameters = map[string]parameter.Value{"count": number}
	for i := range source.Entries {
		source.Entries[i].ParameterSnapshot = parameterSnapshotInput{IsPresent: true, ID: "snapshot-" + source.Entries[i].ExecutionID, SchemaVersion: 1, Values: map[string]parameter.Value{"count": number}}
	}

	plan, err := buildExecutionDraft(source)
	if err != nil {
		t.Fatal(err)
	}
	entries := plan.Entries
	if entries[0].Parameters.Values["count"].Number() != "1" {
		t.Fatalf("snapshot = %#v", entries[0].Parameters)
	}
	source.Entries[0].ParameterSnapshot.Values["count"] = parameter.TextValue("mutated")
	if plan.Entries[0].Parameters.Values["count"].Number() != "1" {
		t.Fatal("sealed snapshot was mutated")
	}
}

func TestBuildExecutionDraftRejectsMissingTypedParameterSnapshot(t *testing.T) {
	source := validMapperSource()
	source.Publication.Workflows[0].Version.Definition.Parameters = []automation.ParameterDefinition{{Name: "enabled", DisplayName: "Enabled", Type: parameter.Boolean, Required: true}}
	source.Publication.Version.Items[0].Parameters = map[string]parameter.Value{"enabled": parameter.BooleanValue(true)}
	source.Publication.Version.Items[1].Parameters = map[string]parameter.Value{"enabled": parameter.BooleanValue(true)}
	if _, err := buildExecutionDraft(source); err == nil || !strings.Contains(err.Error(), "requires a parameter snapshot") {
		t.Fatalf("error = %v", err)
	}
}

func validMapperSource() buildExecutionPlanInput {
	rootStep := automation.WorkflowStep{ID: "call-child", DisplayName: "Call child", Kind: automation.StepWorkflowRef, Reference: &automation.WorkflowReference{WorkflowID: "child", LatestPublished: true}}
	root := automation.WorkflowDependencySnapshot{Workflow: automation.Workflow{ID: "root", DisplayName: "Root", Properties: automation.Properties{}, CurrentVersionID: "root-v1", CreatedAt: 1, UpdatedAt: 1}, Version: automation.WorkflowVersion{ID: "root-v1", WorkflowID: "root", VersionNumber: 1, Definition: automation.WorkflowDefinition{Steps: []automation.WorkflowStep{rootStep}}, CreatedAt: 1}, ResolvedFromLatest: true}
	child := automation.WorkflowDependencySnapshot{Workflow: automation.Workflow{ID: "child", DisplayName: "Child", Properties: automation.Properties{}, CurrentVersionID: "child-v1", CreatedAt: 1, UpdatedAt: 1}, Version: automation.WorkflowVersion{ID: "child-v1", WorkflowID: "child", VersionNumber: 1, Definition: automation.WorkflowDefinition{Steps: []automation.WorkflowStep{{ID: "click", DisplayName: "Click", Kind: automation.StepAction, Action: "click", NodeID: "660e8400-e29b-41d4-a716-446655440000", NodeVersionID: "550e8400-e29b-41d4-a716-446655440000"}}}, CreatedAt: 1}}
	task := automation.TestTask{ID: "task", DisplayName: "Task", CurrentVersionID: "task-v1", CreatedAt: 1, UpdatedAt: 1}
	version := automation.TestTaskVersion{ID: "task-v1", TestTaskID: "task", VersionNumber: 1, FailurePolicy: automation.FailurePolicyContinueOnFailure, CreatedAt: 1, Items: []automation.TestTaskItem{
		{ID: "item-1", TestTaskVersionID: "task-v1", SequenceNumber: 1, WorkflowID: "root", VersionPolicy: automation.WorkflowVersionLatest},
		{ID: "item-2", TestTaskVersionID: "task-v1", SequenceNumber: 2, WorkflowID: "root", VersionPolicy: automation.WorkflowVersionLatest},
	}}
	return buildExecutionPlanInput{
		RunID: "run",
		Publication: automation.TestTaskVersionPlan{
			Task: task, Version: version,
			Workflows:  []automation.WorkflowDependencySnapshot{root, child},
			Nodes:      []automation.NodeDependencySnapshot{{Node: automation.Node{ID: "660e8400-e29b-41d4-a716-446655440000", DisplayName: "Node", CurrentVersionID: "550e8400-e29b-41d4-a716-446655440000"}, Version: automation.NodeVersion{ID: "550e8400-e29b-41d4-a716-446655440000", NodeID: "660e8400-e29b-41d4-a716-446655440000", VersionNumber: 1, Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorTestID, Value: "submit"}}, Fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{}}}}},
			References: []automation.WorkflowReferenceResolution{{ParentWorkflowVersionID: "root-v1", StepID: "call-child", WorkflowID: "child", WorkflowVersionID: "child-v1", ResolvedFromLatest: true}},
		},
		Entries: []executionEntryInput{
			{ExecutionID: "execution-1", TestTaskItemID: "item-1", SequenceNumber: 1, WorkflowID: "root", WorkflowVersionID: "root-v1"},
			{ExecutionID: "execution-2", TestTaskItemID: "item-2", SequenceNumber: 2, WorkflowID: "root", WorkflowVersionID: "root-v1"},
		},
	}
}
