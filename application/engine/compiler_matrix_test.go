package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/node"
	workspace "github.com/Capsule7446/healix-core/domain/workspace"
)

func minimalCompilerPlan() (workspace.TestTaskRunPlan, workspace.WorkflowExecutionPlan) {
	version := workspace.WorkflowVersion{ID: "root-v1", WorkflowID: "root", VersionNumber: 1,
		Definition: workspace.WorkflowDefinition{Steps: []workspace.WorkflowStep{{
			ID: "wait", DisplayName: "等待", Kind: workspace.StepWait, WaitKind: "sleep", WaitMS: 1,
		}}}}
	return workspace.TestTaskRunPlan{Workflows: []workspace.WorkflowDependencySnapshot{{
		Workflow: workspace.Workflow{ID: "root", DisplayName: "根流程"}, Version: version,
	}}}, workspace.WorkflowExecutionPlan{ID: "execution", WorkflowVersionID: version.ID}
}

func TestCompileExecutionRejectsSnapshotIdentityMatrix(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*workspace.TestTaskRunPlan, *workspace.WorkflowExecutionPlan)
		want   string
	}{
		{name: "empty workflow version id", mutate: func(plan *workspace.TestTaskRunPlan, execution *workspace.WorkflowExecutionPlan) {
			plan.Workflows[0].Version.ID = ""
			execution.WorkflowVersionID = ""
		}, want: "empty version id"},
		{name: "duplicate workflow version", mutate: func(plan *workspace.TestTaskRunPlan, _ *workspace.WorkflowExecutionPlan) {
			plan.Workflows = append(plan.Workflows, plan.Workflows[0])
		}, want: "duplicate workflow version"},
		{name: "duplicate reference resolution", mutate: func(plan *workspace.TestTaskRunPlan, _ *workspace.WorkflowExecutionPlan) {
			resolution := workspace.WorkflowReferenceResolution{ParentWorkflowVersionID: "root-v1", StepID: "call", WorkflowID: "child", WorkflowVersionID: "child-v1"}
			plan.References = []workspace.WorkflowReferenceResolution{resolution, resolution}
		}, want: "duplicate workflow resolution"},
		{name: "duplicate node dependency", mutate: func(plan *workspace.TestTaskRunPlan, _ *workspace.WorkflowExecutionPlan) {
			dependency := compilerNodeSnapshot(compilerNodeV1, "submit", 1)
			plan.Nodes = []workspace.NodeDependencySnapshot{dependency, dependency}
		}, want: "duplicate node dependency"},
		{name: "missing root version", mutate: func(_ *workspace.TestTaskRunPlan, execution *workspace.WorkflowExecutionPlan) {
			execution.WorkflowVersionID = "missing-v1"
		}, want: "missing from the run snapshot"},
		{name: "workflow version has wrong owner", mutate: func(plan *workspace.TestTaskRunPlan, _ *workspace.WorkflowExecutionPlan) {
			plan.Workflows[0].Version.WorkflowID = "other"
		}, want: "does not belong"},
		{name: "invalid frozen workflow", mutate: func(plan *workspace.TestTaskRunPlan, _ *workspace.WorkflowExecutionPlan) {
			plan.Workflows[0].Version.Definition.Steps = nil
		}, want: "failed execution preflight"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan, execution := minimalCompilerPlan()
			test.mutate(&plan, &execution)
			_, err := CompileExecution(plan, execution)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestCompileExecutionRejectsWorkflowReferenceResolutionMatrix(t *testing.T) {
	rootStep := workspace.WorkflowStep{ID: "call", DisplayName: "调用", Kind: workspace.StepWorkflowRef,
		Reference: &workspace.WorkflowReference{WorkflowID: "child", WorkflowVersionID: "child-v1"}}
	child := workspace.WorkflowDependencySnapshot{
		Workflow: workspace.Workflow{ID: "child", DisplayName: "子流程"},
		Version: workspace.WorkflowVersion{ID: "child-v1", WorkflowID: "child", VersionNumber: 1,
			Definition: workspace.WorkflowDefinition{Steps: []workspace.WorkflowStep{{ID: "wait", DisplayName: "等待", Kind: workspace.StepWait, WaitKind: "sleep", WaitMS: 1}}}},
	}
	tests := []struct {
		name   string
		mutate func(*workspace.TestTaskRunPlan)
		want   string
	}{
		{name: "missing resolution", want: "no locked version resolution"},
		{name: "resolved version missing", mutate: func(plan *workspace.TestTaskRunPlan) {
			plan.References = []workspace.WorkflowReferenceResolution{{ParentWorkflowVersionID: "root-v1", StepID: "call", WorkflowID: "child", WorkflowVersionID: "child-v1"}}
		}, want: "resolved version child-v1 is missing"},
		{name: "resolution workflow mismatch", mutate: func(plan *workspace.TestTaskRunPlan) {
			plan.Workflows = append(plan.Workflows, child)
			plan.References = []workspace.WorkflowReferenceResolution{{ParentWorkflowVersionID: "root-v1", StepID: "call", WorkflowID: "other", WorkflowVersionID: "child-v1"}}
		}, want: "does not match workflow child"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan, execution := minimalCompilerPlan()
			plan.Workflows[0].Version.Definition.Steps = []workspace.WorkflowStep{rootStep}
			if test.mutate != nil {
				test.mutate(&plan)
			}
			_, err := CompileExecution(plan, execution)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestCompileExecutionRejectsNodeSnapshotMatrix(t *testing.T) {
	plan, execution := minimalCompilerPlan()
	plan.Workflows[0].Version.Definition.Steps = []workspace.WorkflowStep{{
		ID: "click", DisplayName: "点击", Kind: workspace.StepAction, Action: "click",
		NodeID: compilerNodeID, NodeVersionID: compilerNodeV1,
	}}

	t.Run("invalid node spec", func(t *testing.T) {
		invalid := plan
		invalid.Nodes = []workspace.NodeDependencySnapshot{compilerNodeSnapshot(compilerNodeV1, "submit", 1)}
		invalid.Nodes[0].Version.Fingerprint.Tag = ""
		_, err := CompileExecution(invalid, execution)
		if err == nil || !strings.Contains(err.Error(), "fingerprint.tag") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("node version has wrong stable owner", func(t *testing.T) {
		inconsistent := plan
		inconsistent.Nodes = []workspace.NodeDependencySnapshot{compilerNodeSnapshot(compilerNodeV1, "submit", 1)}
		inconsistent.Nodes[0].Version.NodeID = "00000000-0000-7000-8000-000000000999"
		_, err := CompileExecution(inconsistent, execution)
		if err == nil || !strings.Contains(err.Error(), "does not belong to node") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("one version cannot belong to two stable nodes", func(t *testing.T) {
		secondNodeID := "00000000-0000-7000-8000-000000000201"
		sharedVersionID := compilerNodeV1
		conflict := plan
		conflict.Workflows[0].Version.Definition.Steps = append(conflict.Workflows[0].Version.Definition.Steps,
			workspace.WorkflowStep{ID: "second", DisplayName: "第二节点", Kind: workspace.StepAction, Action: "click", NodeID: secondNodeID, NodeVersionID: sharedVersionID})
		first := compilerNodeSnapshot(sharedVersionID, "first", 1)
		second := compilerNodeSnapshot(sharedVersionID, "second", 1)
		second.Node.ID = secondNodeID
		second.Version.NodeID = secondNodeID
		conflict.Nodes = []workspace.NodeDependencySnapshot{first, second}
		_, err := CompileExecution(conflict, execution)
		if err == nil || !strings.Contains(err.Error(), "shared by different stable nodes") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestCompileExecutionBuildsCompleteStepTreeWithoutAliasingSnapshot(t *testing.T) {
	child := workspace.WorkflowDependencySnapshot{
		Workflow: workspace.Workflow{ID: "child", DisplayName: "子流程"},
		Version: workspace.WorkflowVersion{ID: "child-v1", WorkflowID: "child", VersionNumber: 1,
			Definition: workspace.WorkflowDefinition{
				Parameters: []workspace.ParameterDefinition{{Name: "region", DisplayName: "区域", Type: workspace.ParameterText, DefaultValue: "east"}},
				Steps:      []workspace.WorkflowStep{{ID: "child-wait", DisplayName: "子等待", Kind: workspace.StepWait, WaitKind: "sleep", WaitMS: 1}},
			}},
	}
	root := workspace.WorkflowVersion{ID: "root-v1", WorkflowID: "root", VersionNumber: 1,
		Definition: workspace.WorkflowDefinition{Steps: []workspace.WorkflowStep{
			{ID: "select", DisplayName: "选择", Kind: workspace.StepAction, Action: "select", NodeID: compilerNodeID, NodeVersionID: compilerNodeV1, Values: []string{"east", "west"}, Optional: true, CaptureScreenshot: true},
			{ID: "validate", DisplayName: "验证", Kind: workspace.StepValidation, NodeID: compilerNodeID, NodeVersionID: compilerNodeV1,
				Validation: &workspace.ValidationConfig{Assertion: workspace.ValidationAssertion{Kind: workspace.ValidationTextContains, Expected: "ready", IgnoreCase: true}, Wait: workspace.ValidationWait{MaxWaitMS: 2_000, StabilityMS: 200}}},
			{ID: "repeat", DisplayName: "重复", Kind: workspace.StepRepeat, RepeatCount: 2, Children: []workspace.WorkflowStep{{ID: "network", DisplayName: "网络", Kind: workspace.StepWait, WaitKind: "network_idle", WaitMS: 300}}},
			{ID: "call", DisplayName: "调用", Kind: workspace.StepWorkflowRef, Reference: &workspace.WorkflowReference{WorkflowID: "child", WorkflowVersionID: "child-v1", ParameterBindings: map[string]string{"region": "north"}}},
		}}}
	plan := workspace.TestTaskRunPlan{
		Workflows:  []workspace.WorkflowDependencySnapshot{{Workflow: workspace.Workflow{ID: "root", DisplayName: "根流程"}, Version: root}, child},
		Nodes:      []workspace.NodeDependencySnapshot{compilerNodeSnapshot(compilerNodeV1, "region", 1)},
		References: []workspace.WorkflowReferenceResolution{{ParentWorkflowVersionID: "root-v1", StepID: "call", WorkflowID: "child", WorkflowVersionID: "child-v1"}},
	}
	compiled, err := CompileExecution(plan, workspace.WorkflowExecutionPlan{ID: "execution", WorkflowVersionID: "root-v1"})
	if err != nil {
		t.Fatal(err)
	}
	children := compiled.Program.Root.(*node.WorkflowNode).Children
	if len(children) != 4 {
		t.Fatalf("children = %#v", children)
	}
	selectStep := children[0].(*node.StepNode)
	validation := children[1].(*node.ValidationNode)
	repeat := children[2].(*node.RepeatNode)
	call := children[3].(*node.WorkflowCallNode)
	if selectStep.Action.Kind != node.ActionSelect || !selectStep.Optional || len(selectStep.Action.Values) != 2 ||
		validation.Assertion.Kind != string(workspace.ValidationTextContains) || validation.MaxWait != 2*time.Second ||
		repeat.Times != 2 || repeat.Children[0].(*node.WaitNode).Kind != node.WaitNetworkIdle ||
		call.Bindings["region"] != "north" {
		t.Fatalf("compiled tree = %#v %#v %#v %#v", selectStep, validation, repeat, call)
	}
	if metadata := compiled.Metadata["root-v1::select"]; !metadata.CaptureScreenshot || metadata.HierarchyPath != "根流程 / 选择" {
		t.Fatalf("metadata = %#v", metadata)
	}

	plan.Workflows[0].Version.Definition.Steps[0].Values[0] = "mutated"
	plan.Workflows[0].Version.Definition.Steps[3].Reference.ParameterBindings["region"] = "mutated"
	plan.Nodes[0].Version.Selectors[0].Value = "mutated"
	plan.Nodes[0].Version.Fingerprint.Attributes["name"] = "mutated"
	if selectStep.Action.Values[0] != "east" || call.Bindings["region"] != "north" ||
		compiled.Program.Specs[compilerNodeV1].Selectors[0].Value != "region" ||
		compiled.Program.Specs[compilerNodeV1].Fingerprint.Attributes["name"] == "mutated" {
		t.Fatalf("compiled execution aliases frozen snapshot: %#v", compiled)
	}
}

func TestCompileExecutionPreservesCompleteFingerprint(t *testing.T) {
	plan, execution := minimalCompilerPlan()
	plan.Workflows[0].Version.Definition.Steps = []workspace.WorkflowStep{{ID: "click", DisplayName: "点击", Kind: workspace.StepAction,
		Action: "click", NodeID: compilerNodeID, NodeVersionID: compilerNodeV1}}
	dependency := compilerNodeSnapshot(compilerNodeV1, "submit", 1)
	dependency.Version.PageURL = "/checkout"
	dependency.Version.Origin = "https://example.test"
	dependency.Version.Fingerprint = fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{"name": "submit"}, Text: "提交",
		ARIA: fingerprint.ARIA{Role: "button", Name: "提交"}, Path: []string{"html", "body"}, SiblingIndex: 2,
		Neighbors: fingerprint.Neighbors{Prev: "input", Next: "a", ParentTag: "form"}, LabelText: "提交订单", FormID: "checkout"}
	plan.Nodes = []workspace.NodeDependencySnapshot{dependency}
	compiled, err := CompileExecution(plan, execution)
	if err != nil {
		t.Fatal(err)
	}
	got := compiled.Program.Specs[compilerNodeV1]
	if got.PageURL != dependency.Version.PageURL || got.Origin != dependency.Version.Origin ||
		got.Fingerprint.Text != "提交" || got.Fingerprint.ARIA.Name != "提交" || got.Fingerprint.SiblingIndex != 2 ||
		got.Fingerprint.Neighbors.ParentTag != "form" || got.Fingerprint.LabelText != "提交订单" || got.Fingerprint.FormID != "checkout" {
		t.Fatalf("fingerprint fields were lost: %#v", got)
	}
}
