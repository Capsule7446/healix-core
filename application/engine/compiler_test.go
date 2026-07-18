package engine

import (
	"testing"
	"time"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/node"
	workspace "github.com/Capsule7446/healix-core/domain/workspace"
)

const (
	compilerNodeID = "00000000-0000-7000-8000-000000000101"
	compilerNodeV1 = "00000000-0000-7000-8000-000000000102"
	compilerNodeV2 = "00000000-0000-7000-8000-000000000103"
)

func TestCompileExecutionBuildsLockedWorkflowTreeAndIndexes(t *testing.T) {
	childVersion := workspace.WorkflowVersion{ID: "child-v1", WorkflowID: "child", VersionNumber: 1,
		Definition: workspace.WorkflowDefinition{Parameters: []workspace.ParameterDefinition{{Name: "region", DisplayName: "区域", Type: workspace.ParameterText, DefaultValue: "east"}},
			Steps: []workspace.WorkflowStep{{ID: "click", DisplayName: "点击", Kind: workspace.StepAction,
				Action: "click", NodeID: compilerNodeID, NodeVersionID: compilerNodeV1}}}}
	rootVersion := workspace.WorkflowVersion{ID: "root-v1", WorkflowID: "root", VersionNumber: 1,
		Definition: workspace.WorkflowDefinition{Steps: []workspace.WorkflowStep{{ID: "call", DisplayName: "调用子流程",
			Kind: workspace.StepWorkflowRef, Reference: &workspace.WorkflowReference{WorkflowID: "child",
				LatestPublished: true, ParameterBindings: map[string]string{}}}}}}
	plan := workspace.TestTaskRunPlan{
		Workflows: []workspace.WorkflowDependencySnapshot{
			{Workflow: workspace.Workflow{ID: "root", DisplayName: "根流程"}, Version: rootVersion},
			{Workflow: workspace.Workflow{ID: "child", DisplayName: "子流程"}, Version: childVersion},
		},
		Nodes: []workspace.NodeDependencySnapshot{compilerNodeSnapshot(compilerNodeV1, "submit-v1", 1)},
		References: []workspace.WorkflowReferenceResolution{{ParentWorkflowVersionID: rootVersion.ID,
			StepID: "call", WorkflowID: "child", WorkflowVersionID: childVersion.ID, ResolvedFromLatest: true}},
	}
	compiled, err := CompileExecution(plan, workspace.WorkflowExecutionPlan{ID: "exec", WorkflowVersionID: rootVersion.ID})
	if err != nil {
		t.Fatal(err)
	}
	root, ok := compiled.Program.Root.(*node.WorkflowNode)
	if !ok || root.ID() != rootVersion.ID || len(root.Children) != 1 {
		t.Fatalf("root = %#v", compiled.Program.Root)
	}
	call, ok := root.Children[0].(*node.WorkflowCallNode)
	if !ok || call.Target == nil || call.Target.ID() != childVersion.ID || call.Bindings["region"] != "east" {
		t.Fatalf("workflow call = %#v", root.Children[0])
	}
	if got := compiled.Metadata["child-v1::click"]; got.WorkflowStepID != "click" || got.NodeVersionID != compilerNodeV1 {
		t.Fatalf("metadata = %#v", got)
	}
	if got := compiled.RuntimeNodes[compilerNodeV1]; got.NodeID != compilerNodeID || got.NodeVersionID != compilerNodeV1 {
		t.Fatalf("runtime node identity = %#v", got)
	}
}

func TestCompileExecutionKeepsTwoVersionsOfSameNodeExact(t *testing.T) {
	workflow := workspace.WorkflowVersion{ID: "checkout-v1", WorkflowID: "checkout", VersionNumber: 1,
		Definition: workspace.WorkflowDefinition{Steps: []workspace.WorkflowStep{
			{ID: "old", DisplayName: "旧版", Kind: workspace.StepAction, Action: "click", NodeID: compilerNodeID, NodeVersionID: compilerNodeV1},
			{ID: "new", DisplayName: "新版", Kind: workspace.StepAction, Action: "click", NodeID: compilerNodeID, NodeVersionID: compilerNodeV2},
		}}}
	plan := workspace.TestTaskRunPlan{Workflows: []workspace.WorkflowDependencySnapshot{{
		Workflow: workspace.Workflow{ID: "checkout", DisplayName: "结账"}, Version: workflow}},
		Nodes: []workspace.NodeDependencySnapshot{
			compilerNodeSnapshot(compilerNodeV1, "checkout-old", 1),
			compilerNodeSnapshot(compilerNodeV2, "checkout-new", 2),
		}}
	compiled, err := CompileExecution(plan, workspace.WorkflowExecutionPlan{ID: "exec", WorkflowVersionID: workflow.ID})
	if err != nil {
		t.Fatal(err)
	}
	root := compiled.Program.Root.(*node.WorkflowNode)
	oldStep := root.Children[0].(*node.StepNode)
	newStep := root.Children[1].(*node.StepNode)
	if oldStep.Target.ID != compilerNodeV1 || oldStep.Target.Selectors[0].Value != "checkout-old" {
		t.Fatalf("old target = %#v", oldStep.Target)
	}
	if newStep.Target.ID != compilerNodeV2 || newStep.Target.Selectors[0].Value != "checkout-new" {
		t.Fatalf("new target = %#v", newStep.Target)
	}
	if len(compiled.Program.Specs) != 2 || len(compiled.RuntimeNodes) != 2 {
		t.Fatalf("compiled indexes = specs:%d nodes:%d", len(compiled.Program.Specs), len(compiled.RuntimeNodes))
	}
}

func TestCompileExecutionBuildsValidationGroup(t *testing.T) {
	member := workspace.WorkflowStep{
		ID: "member", DisplayName: "状态成功", Kind: workspace.StepValidation,
		NodeID: compilerNodeID, NodeVersionID: compilerNodeV1,
		Validation: &workspace.ValidationConfig{Assertion: workspace.ValidationAssertion{
			Kind: workspace.ValidationTextEquals, Expected: "成功"}},
	}
	workflow := workspace.WorkflowVersion{
		ID: "validation-v1", WorkflowID: "validation", VersionNumber: 1,
		Definition: workspace.WorkflowDefinition{Steps: []workspace.WorkflowStep{{
			ID: "group", DisplayName: "结果", Kind: workspace.StepValidationGroup, CaptureScreenshot: true,
			ValidationGroup: &workspace.ValidationGroup{
				Wait: workspace.ValidationWait{MaxWaitMS: 2_000, StabilityMS: 300},
				Branches: []workspace.ValidationBranch{{
					ID: "success", Name: "成功", Steps: []workspace.WorkflowStep{member},
				}},
			},
		}}},
	}
	plan := workspace.TestTaskRunPlan{Workflows: []workspace.WorkflowDependencySnapshot{{
		Workflow: workspace.Workflow{ID: "validation", DisplayName: "验证"}, Version: workflow}},
		Nodes: []workspace.NodeDependencySnapshot{compilerNodeSnapshot(compilerNodeV1, "status", 1)}}
	compiled, err := CompileExecution(plan, workspace.WorkflowExecutionPlan{ID: "exec", WorkflowVersionID: workflow.ID})
	if err != nil {
		t.Fatal(err)
	}
	group, ok := compiled.Program.Root.(*node.WorkflowNode).Children[0].(*node.ValidationGroupNode)
	if !ok || len(group.Branches) != 1 || len(group.Branches[0].Nodes) != 1 {
		t.Fatalf("validation group = %#v", compiled.Program.Root)
	}
	validation := group.Branches[0].Nodes[0]
	if validation.GroupID != "validation-v1::group" || validation.BranchID != "success" ||
		validation.Assertion.Expected != "成功" || validation.MaxWait.Milliseconds() != 2_000 {
		t.Fatalf("validation member = %#v", validation)
	}
	if !compiled.Metadata["validation-v1::group"].CaptureScreenshot ||
		compiled.Metadata["validation-v1::member"].HierarchyPath != "验证 / 结果 / 成功 / 状态成功" {
		t.Fatalf("validation metadata = %#v", compiled.Metadata)
	}
}

func TestCompileExecutionRejectsMissingSnapshotAndCycles(t *testing.T) {
	missing := workspace.TestTaskRunPlan{Workflows: []workspace.WorkflowDependencySnapshot{{
		Workflow: workspace.Workflow{ID: "root"}, Version: workspace.WorkflowVersion{ID: "root-v1", WorkflowID: "root",
			Definition: workspace.WorkflowDefinition{Steps: []workspace.WorkflowStep{{ID: "click", Kind: workspace.StepAction,
				Action: "click", NodeID: compilerNodeID, NodeVersionID: compilerNodeV1}}}}}}}
	if _, err := CompileExecution(missing, workspace.WorkflowExecutionPlan{ID: "exec", WorkflowVersionID: "root-v1"}); err == nil {
		t.Fatal("missing node snapshot was accepted")
	}

	a := workspace.WorkflowVersion{ID: "a-v1", WorkflowID: "a", Definition: workspace.WorkflowDefinition{Steps: []workspace.WorkflowStep{{
		ID: "to-b", Kind: workspace.StepWorkflowRef, Reference: &workspace.WorkflowReference{WorkflowID: "b"}}}}}
	b := workspace.WorkflowVersion{ID: "b-v1", WorkflowID: "b", Definition: workspace.WorkflowDefinition{Steps: []workspace.WorkflowStep{{
		ID: "to-a", Kind: workspace.StepWorkflowRef, Reference: &workspace.WorkflowReference{WorkflowID: "a"}}}}}
	cycle := workspace.TestTaskRunPlan{Workflows: []workspace.WorkflowDependencySnapshot{
		{Workflow: workspace.Workflow{ID: "a"}, Version: a}, {Workflow: workspace.Workflow{ID: "b"}, Version: b}},
		References: []workspace.WorkflowReferenceResolution{
			{ParentWorkflowVersionID: "a-v1", StepID: "to-b", WorkflowID: "b", WorkflowVersionID: "b-v1"},
			{ParentWorkflowVersionID: "b-v1", StepID: "to-a", WorkflowID: "a", WorkflowVersionID: "a-v1"},
		}}
	if _, err := CompileExecution(cycle, workspace.WorkflowExecutionPlan{ID: "exec", WorkflowVersionID: "a-v1"}); err == nil {
		t.Fatal("workflow reference cycle was accepted")
	}
}

func TestCompileExecutionBuildsAllWaitKindsAndRejectsUnknownKind(t *testing.T) {
	workflow := workspace.WorkflowVersion{ID: "wait-v1", WorkflowID: "wait", VersionNumber: 1,
		Definition: workspace.WorkflowDefinition{Steps: []workspace.WorkflowStep{
			{ID: "sleep", DisplayName: "固定等待", Kind: workspace.StepWait, WaitKind: "sleep", WaitMS: 25},
			{ID: "element", DisplayName: "元素等待", Kind: workspace.StepWait, WaitKind: "element", WaitMS: 500,
				NodeID: compilerNodeID, NodeVersionID: compilerNodeV1},
			{ID: "network", DisplayName: "网络等待", Kind: workspace.StepWait, WaitKind: "network_idle", WaitMS: 750},
		}}}
	plan := workspace.TestTaskRunPlan{Workflows: []workspace.WorkflowDependencySnapshot{{
		Workflow: workspace.Workflow{ID: "wait", DisplayName: "等待"}, Version: workflow}},
		Nodes: []workspace.NodeDependencySnapshot{compilerNodeSnapshot(compilerNodeV1, "ready", 1)}}
	compiled, err := CompileExecution(plan, workspace.WorkflowExecutionPlan{ID: "execution", WorkflowVersionID: workflow.ID})
	if err != nil {
		t.Fatal(err)
	}
	children := compiled.Program.Root.(*node.WorkflowNode).Children
	sleep := children[0].(*node.WaitNode)
	element := children[1].(*node.WaitNode)
	network := children[2].(*node.WaitNode)
	if sleep.Kind != node.WaitSleep || sleep.Duration != 25*time.Millisecond ||
		element.Kind != node.WaitElement || element.Target.ID != compilerNodeV1 || element.Timeout != 500*time.Millisecond ||
		network.Kind != node.WaitNetworkIdle || network.Timeout != 750*time.Millisecond {
		t.Fatalf("compiled waits = %#v %#v %#v", sleep, element, network)
	}
	workflow.Definition.Steps = []workspace.WorkflowStep{{ID: "unknown", DisplayName: "未知", Kind: workspace.StepWait, WaitKind: "eventual"}}
	plan.Workflows[0].Version = workflow
	if _, err := CompileExecution(plan, workspace.WorkflowExecutionPlan{ID: "execution", WorkflowVersionID: workflow.ID}); err == nil {
		t.Fatal("unknown wait kind was accepted")
	}
}

func compilerNodeSnapshot(versionID, selector string, number int) workspace.NodeDependencySnapshot {
	return workspace.NodeDependencySnapshot{Node: workspace.Node{ID: compilerNodeID, DisplayName: "节点"},
		Version: workspace.NodeVersion{ID: versionID, NodeID: compilerNodeID, VersionNumber: number,
			Selectors:   []fingerprint.Selector{{Type: fingerprint.SelectorTestID, Value: selector}},
			Fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{}}}}
}
