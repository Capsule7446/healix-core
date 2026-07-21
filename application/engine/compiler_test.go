package engine

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/node"
)

const (
	compilerNodeID = "00000000-0000-7000-8000-000000000101"
	compilerNodeV1 = "00000000-0000-7000-8000-000000000102"
	compilerNodeV2 = "00000000-0000-7000-8000-000000000103"
)

func TestCompilePlanRejectsUnsealedPlanWithDomainError(t *testing.T) {
	_, err := CompilePlan(execution.Plan{})
	if !errors.Is(err, execution.ErrUnsealedPlan) {
		t.Fatalf("CompilePlan() error = %v, want ErrUnsealedPlan", err)
	}
}

func TestRuntimeWorkflowStepIDIsCollisionFree(t *testing.T) {
	first := runtimeWorkflowStepID("a:b", "c")
	second := runtimeWorkflowStepID("a", "b:c")
	if first == second {
		t.Fatalf("runtime ids collided: %q", first)
	}
	if first != runtimeWorkflowStepID("a:b", "c") {
		t.Fatal("runtime id is not deterministic")
	}
}

func TestCompilePlanBuildsLockedWorkflowTreeAndBindsChildDefaults(t *testing.T) {

	plan := execution.Draft{
		RunID: "exec", Entries: []execution.WorkflowEntry{{ExecutionID: "execution-entry", TestTaskItemID: "task-item", WorkflowID: "root", WorkflowVersionID: "root-v1"}},
		Workflows: []execution.WorkflowSnapshot{
			{WorkflowID: "root", VersionID: "root-v1", DisplayName: "根流程", VersionNumber: 1, Steps: []execution.Step{{ID: "call", DisplayName: "调用子流程", Kind: execution.WorkflowReference, Reference: &execution.Reference{WorkflowID: "child", WorkflowVersionID: "child-v1"}}}},
			{WorkflowID: "child", VersionID: "child-v1", DisplayName: "子流程", VersionNumber: 1,
				Parameters: []execution.Parameter{{Name: "region", DefaultValue: "east"}},
				Steps:      []execution.Step{{ID: "click", DisplayName: "点击", Kind: execution.ActionStep, Action: "click", NodeID: compilerNodeID, NodeVersionID: compilerNodeV1}}},
		},
		Nodes:      []execution.NodeSnapshot{compilerNodeSnapshot(compilerNodeV1, "submit-v1")},
		References: []execution.ReferenceResolution{{ParentVersionID: "root-v1", StepID: "call", WorkflowID: "child", WorkflowVersionID: "child-v1"}},
	}

	compiled, err := compileDraft(plan)
	if err != nil {
		t.Fatal(err)
	}
	root, ok := compiled.Program.Root.(*node.WorkflowNode)
	if !ok || root.ID() != "workflow|15:execution-entry" || len(root.Children) != 1 {
		t.Fatalf("root = %#v", compiled.Program.Root)
	}
	call, ok := root.Children[0].(*node.WorkflowCallNode)
	if !ok || call.Target == nil || call.Target.ID() != "workflow|15:execution-entry4:call8:child-v1" || call.Bindings["region"] != "east" {
		t.Fatalf("workflow call = %#v", root.Children[0])
	}
	childStepID := runtimeInvocationStepID("15:execution-entry4:call8:child-v1", "click")
	if got := compiled.Metadata[childStepID]; got.WorkflowStepID != "click" || got.NodeVersionID != compilerNodeV1 {
		t.Fatalf("metadata = %#v", got)
	}
}

func TestCompilePlanCreatesDistinctRuntimeIdentitiesForSharedChildInvocations(t *testing.T) {
	child := execution.WorkflowSnapshot{WorkflowID: "child", VersionID: "child-v1", DisplayName: "Child", VersionNumber: 1,
		Steps: []execution.Step{{ID: "click", DisplayName: "Click", Kind: execution.ActionStep, Action: "click", NodeID: compilerNodeID, NodeVersionID: compilerNodeV1}}}
	call := func(id string) execution.Step {
		return execution.Step{ID: id, DisplayName: id, Kind: execution.WorkflowReference, Reference: &execution.Reference{WorkflowID: "child", WorkflowVersionID: "child-v1"}}
	}
	plan := execution.Draft{RunID: "exec", FailurePolicy: execution.FailurePolicyStopOnFailure, Entries: []execution.WorkflowEntry{{ExecutionID: "execution-entry", TestTaskItemID: "task-item", SequenceNumber: 1, WorkflowID: "root", WorkflowVersionID: "root-v1"}}, Workflows: []execution.WorkflowSnapshot{
		{WorkflowID: "root", VersionID: "root-v1", DisplayName: "Root", VersionNumber: 1, Steps: []execution.Step{call("first"), call("second")}}, child},
		Nodes: []execution.NodeSnapshot{compilerNodeSnapshot(compilerNodeV1, "submit")}, References: []execution.ReferenceResolution{
			{ParentVersionID: "root-v1", StepID: "first", WorkflowID: "child", WorkflowVersionID: "child-v1"},
			{ParentVersionID: "root-v1", StepID: "second", WorkflowID: "child", WorkflowVersionID: "child-v1"},
		}}

	compiled, err := compileDraft(plan)
	if err != nil {
		t.Fatal(err)
	}
	children := compiled.Program.Root.(*node.WorkflowNode).Children
	first := children[0].(*node.WorkflowCallNode).Target
	second := children[1].(*node.WorkflowCallNode).Target
	if first.ID() == second.ID() || first.Children[0].ID() == second.Children[0].ID() {
		t.Fatalf("shared child runtime IDs collided: workflows %q, steps %q", first.ID(), first.Children[0].ID())
	}
	if compiled.Metadata[first.Children[0].ID()].WorkflowStepID != "click" || compiled.Metadata[second.Children[0].ID()].WorkflowStepID != "click" {
		t.Fatalf("invocation metadata missing: %#v", compiled.Metadata)
	}
}

func TestCompilePlanKeepsTwoVersionsOfSameNodeExact(t *testing.T) {
	plan := execution.Draft{RunID: "exec", FailurePolicy: execution.FailurePolicyStopOnFailure, Entries: []execution.WorkflowEntry{{ExecutionID: "execution-entry", TestTaskItemID: "task-item", SequenceNumber: 1, WorkflowID: "checkout", WorkflowVersionID: "checkout-v1"}},
		Workflows: []execution.WorkflowSnapshot{{WorkflowID: "checkout", VersionID: "checkout-v1", DisplayName: "结账", VersionNumber: 1, Steps: []execution.Step{
			{ID: "old", DisplayName: "旧版", Kind: execution.ActionStep, Action: "click", NodeID: compilerNodeID, NodeVersionID: compilerNodeV1},
			{ID: "new", DisplayName: "新版", Kind: execution.ActionStep, Action: "click", NodeID: compilerNodeID, NodeVersionID: compilerNodeV2},
		}}}, Nodes: []execution.NodeSnapshot{compilerNodeSnapshot(compilerNodeV1, "checkout-old"), compilerNodeSnapshot(compilerNodeV2, "checkout-new")}}

	compiled, err := compileDraft(plan)
	if err != nil {
		t.Fatal(err)
	}
	children := compiled.Program.Root.(*node.WorkflowNode).Children
	oldStep, newStep := children[0].(*node.StepNode), children[1].(*node.StepNode)
	if oldStep.Target.ID != compilerNodeV1 || oldStep.Target.Selectors[0].Value != "checkout-old" {
		t.Fatalf("old target = %#v", oldStep.Target)
	}
	if newStep.Target.ID != compilerNodeV2 || newStep.Target.Selectors[0].Value != "checkout-new" {
		t.Fatalf("new target = %#v", newStep.Target)
	}
}

func TestCompilePlanBuildsValidationGroup(t *testing.T) {
	member := execution.Step{ID: "member", DisplayName: "状态成功", Kind: execution.ValidationStep, NodeID: compilerNodeID, NodeVersionID: compilerNodeV1, Validation: &execution.Validation{Kind: "text_equals", Expected: "成功"}}
	plan := execution.Draft{
		RunID: "exec", Entries: []execution.WorkflowEntry{{ExecutionID: "execution-entry", TestTaskItemID: "task-item", WorkflowID: "validation", WorkflowVersionID: "validation-v1"}},
		Workflows: []execution.WorkflowSnapshot{{
			WorkflowID: "validation", VersionID: "validation-v1", DisplayName: "验证", VersionNumber: 1,
			Steps: []execution.Step{{
				ID: "group", DisplayName: "结果", Kind: execution.ValidationGroupStep, CaptureScreenshot: true,
				ValidationGroup: &execution.ValidationGroup{
					MaxWaitMS: 2_000, StabilityMS: 300,
					Branches: []execution.ValidationBranch{{ID: "success", Name: "成功", Steps: []execution.Step{member}}},
				},
			}},
		}},
		Nodes: []execution.NodeSnapshot{compilerNodeSnapshot(compilerNodeV1, "status")},
	}

	compiled, err := compileDraft(plan)
	if err != nil {
		t.Fatal(err)
	}
	group := compiled.Program.Root.(*node.WorkflowNode).Children[0].(*node.ValidationGroupNode)
	validation := group.Branches[0].Nodes[0]
	if validation.GroupID != runtimeWorkflowStepID("execution-entry", "group") || validation.BranchID != "success" || validation.Assertion.Expected != "成功" || validation.MaxWait != 2*time.Second {
		t.Fatalf("validation member = %#v", validation)
	}
	if !compiled.Metadata[runtimeWorkflowStepID("execution-entry", "group")].CaptureScreenshot || compiled.Metadata[runtimeWorkflowStepID("execution-entry", "member")].HierarchyPath != "验证 / 结果 / 成功 / 状态成功" {
		t.Fatalf("validation metadata = %#v", compiled.Metadata)
	}
}

func TestCompilePlanRejectsMissingSnapshotAndCycles(t *testing.T) {
	missing := execution.Draft{RunID: "exec", FailurePolicy: execution.FailurePolicyStopOnFailure, Entries: []execution.WorkflowEntry{{ExecutionID: "execution-entry", TestTaskItemID: "task-item", SequenceNumber: 1, WorkflowID: "root", WorkflowVersionID: "root-v1"}}, Workflows: []execution.WorkflowSnapshot{{WorkflowID: "root", VersionID: "root-v1", DisplayName: "root", VersionNumber: 1, Steps: []execution.Step{{ID: "click", DisplayName: "click", Kind: execution.ActionStep, Action: "click", NodeID: compilerNodeID, NodeVersionID: compilerNodeV1}}}}}
	if _, err := compileDraft(missing); err == nil {
		t.Fatal("missing node snapshot was accepted")
	}
	cycle := execution.Draft{RunID: "exec", FailurePolicy: execution.FailurePolicyStopOnFailure, Entries: []execution.WorkflowEntry{{ExecutionID: "execution-entry", TestTaskItemID: "task-item", SequenceNumber: 1, WorkflowID: "a", WorkflowVersionID: "a-v1"}}, Workflows: []execution.WorkflowSnapshot{
		{WorkflowID: "a", VersionID: "a-v1", DisplayName: "a", VersionNumber: 1, Steps: []execution.Step{{ID: "to-b", DisplayName: "b", Kind: execution.WorkflowReference, Reference: &execution.Reference{WorkflowID: "b"}}}},
		{WorkflowID: "b", VersionID: "b-v1", DisplayName: "b", VersionNumber: 1, Steps: []execution.Step{{ID: "to-a", DisplayName: "a", Kind: execution.WorkflowReference, Reference: &execution.Reference{WorkflowID: "a"}}}},
	}, References: []execution.ReferenceResolution{{ParentVersionID: "a-v1", StepID: "to-b", WorkflowID: "b", WorkflowVersionID: "b-v1"}, {ParentVersionID: "b-v1", StepID: "to-a", WorkflowID: "a", WorkflowVersionID: "a-v1"}}}
	if _, err := compileDraft(cycle); err == nil {
		t.Fatal("workflow reference cycle was accepted")
	}
}

func compilerNodeSnapshot(versionID, selector string) execution.NodeSnapshot {
	return execution.NodeSnapshot{NodeID: compilerNodeID, VersionID: versionID, DisplayName: "节点", Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorTestID, Value: selector}}, Fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{}}}
}

func compileDraft(draft execution.Draft) (CompiledEntry, error) {
	if draft.FailurePolicy == "" {
		draft.FailurePolicy = execution.FailurePolicyStopOnFailure
	}
	for index := range draft.Entries {
		if draft.Entries[index].SequenceNumber == 0 {
			draft.Entries[index].SequenceNumber = index + 1
		}
	}
	plan, err := execution.Seal(draft)
	if err != nil {
		return CompiledEntry{}, err
	}
	compiled, err := CompilePlan(plan)
	if err != nil {
		return CompiledEntry{}, err
	}
	entry, ok := compiled.Entry(draft.Entries[0].ExecutionID)
	if !ok {
		return CompiledEntry{}, fmt.Errorf("compiled entry is missing")
	}
	return entry, nil
}

func TestCompilePlanKeepsRepeatedEntryOccurrencesIndependent(t *testing.T) {
	draft := minimalCompilerPlan()
	draft.Entries = []execution.WorkflowEntry{
		{ExecutionID: "execution-a", TestTaskItemID: "item-a", SequenceNumber: 1, WorkflowID: "root", WorkflowVersionID: "root-v1"},
		{ExecutionID: "execution-b", TestTaskItemID: "item-b", SequenceNumber: 2, WorkflowID: "root", WorkflowVersionID: "root-v1"},
	}
	plan, err := execution.Seal(draft)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := CompilePlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Entries) != 2 {
		t.Fatalf("entries = %d", len(compiled.Entries))
	}
	if compiled.Entries[0].ExecutionID != "execution-a" || compiled.Entries[1].ExecutionID != "execution-b" {
		t.Fatalf("entry declaration order not preserved: %q, %q", compiled.Entries[0].ExecutionID, compiled.Entries[1].ExecutionID)
	}
	a, ok := compiled.Entry("execution-a")
	if !ok {
		t.Fatal("execution-a is missing")
	}
	b, ok := compiled.Entry("execution-b")
	if !ok {
		t.Fatal("execution-b is missing")
	}
	if _, ok := compiled.Entry("missing"); ok {
		t.Fatal("missing execution unexpectedly found")
	}
	lookupCopy := a
	lookupCopy.ExecutionID = "mutated"
	again, ok := compiled.Entry("execution-a")
	if !ok || again.ExecutionID != "execution-a" {
		t.Fatal("lookup exposed mutable index state")
	}
	if a.Program.Root.ID() == b.Program.Root.ID() {
		t.Fatalf("runtime roots collide: %q", a.Program.Root.ID())
	}
	if &a.Metadata == &b.Metadata || a.Program.Root.ID() != "workflow|11:execution-a" {
		t.Fatalf("entries not occurrence-specific")
	}
}
