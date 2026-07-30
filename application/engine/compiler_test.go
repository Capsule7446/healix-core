package engine

import (
	"errors"
	"fmt"
	"github.com/Capsule7446/healix-core/domain/parameter"
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

func TestCompilePlanIndexesInvocationsOnceAcrossEntries(t *testing.T) {
	allocationsForEntries := func(entryCount int) float64 {
		draft := minimalCompilerPlan()
		draft.Workflows[0].Parameters = []execution.Parameter{{Name: "payload", DisplayName: "Payload", Type: parameter.Text, Required: true}}
		draft.Entries = make([]execution.WorkflowEntry, entryCount)
		for index := range draft.Entries {
			draft.Entries[index] = execution.WorkflowEntry{
				ExecutionID:       fmt.Sprintf("execution-%03d", index),
				TestTaskItemID:    fmt.Sprintf("item-%03d", index),
				SequenceNumber:    index + 1,
				FlowFragmentID:    "root",
				WorkflowVersionID: "root-v1",
				Parameters: execution.ParameterSnapshot{ID: fmt.Sprintf("parameters-%03d", index), SchemaVersion: 1, WorkflowVersionID: "root-v1", Values: map[string]parameter.Value{
					"payload": parameter.TextValue("value"),
				}},
			}
		}
		snapshot, err := runSnapshotForCompilerTest(draft, map[string]string{})
		if err != nil {
			t.Fatalf("seal %d-entry snapshot: %v", entryCount, err)
		}
		compiled, err := CompilePlan(snapshot)
		if err != nil {
			t.Fatalf("compile %d-entry snapshot: %v", entryCount, err)
		}
		if len(compiled.Entries()) != entryCount {
			t.Fatalf("compiled entries = %d, want %d", len(compiled.Entries()), entryCount)
		}
		return testing.AllocsPerRun(20, func() {
			measured, compileErr := CompilePlan(snapshot)
			if compileErr != nil {
				panic(compileErr)
			}
			if len(measured.Entries()) != entryCount {
				panic("compiled entry count mismatch")
			}
		})
	}

	const smallEntryCount = 32
	small := allocationsForEntries(smallEntryCount)
	medium := allocationsForEntries(2 * smallEntryCount)
	large := allocationsForEntries(4 * smallEntryCount)
	if !(small < medium && medium < large) {
		t.Fatalf("compile allocations are not monotonic: 32=%.0f, 64=%.0f, 128=%.0f", small, medium, large)
	}
	firstGrowth := medium - small
	secondGrowth := large - medium
	// Moving snapshot.Invocations or invocationIndex into the entry loop changes
	// incremental growth from approximately 2x (linear) to approximately 4x.
	if firstGrowth <= 0 || secondGrowth >= 3*firstGrowth {
		t.Fatalf("compile allocation growth is not near-linear: 32=%.0f, 64=%.0f, 128=%.0f; growths=%.0f, %.0f (ratio %.2f); invocations must be cloned and indexed once", small, medium, large, firstGrowth, secondGrowth, secondGrowth/firstGrowth)
	}
}

func TestCompilePlanInjectsTypedEnvironmentValues(t *testing.T) {
	draft := minimalCompilerPlan()
	draft.Entries[0].Parameters.Values = nil
	number, err := parameter.NewNumberValue("1.25")
	if err != nil {
		t.Fatal(err)
	}
	variables := map[string]parameter.Value{
		"text":    parameter.TextValue("east"),
		"number":  number,
		"boolean": parameter.BooleanValue(true),
		"single":  parameter.SingleSelectValue("primary"),
		"multi":   parameter.MultiSelectValue([]string{"east", "west"}),
	}
	snapshot, err := runSnapshotForCompilerTypedEnvironmentTest(draft, variables)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := CompilePlan(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	root := compiled.Entries()[0].program.Root.(*node.WorkflowNode)
	for name, want := range variables {
		got, exists := root.Parameters["env."+name]
		if !exists || !got.Equal(want) {
			t.Fatalf("env.%s = %#v, want %#v", name, got, want)
		}
	}
	root.Parameters["env.multi"] = parameter.MultiSelectValue([]string{"mutated"})
	if got := snapshot.Environment().Variables["multi"].MultiSelect(); len(got) != 2 || got[0] != "east" {
		t.Fatalf("compiled scope aliases snapshot: %v", got)
	}
}

func TestCompilePlanInjectsEnvironmentIntoParameterlessRoot(t *testing.T) {
	draft := minimalCompilerPlan()
	draft.Entries[0].Parameters.Values = nil
	snapshot, err := runSnapshotForCompilerTest(draft, map[string]string{"Region": "east"})
	if err != nil {
		t.Fatal(err)
	}

	compiled, err := CompilePlan(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	root := compiled.Entries()[0].program.Root.(*node.WorkflowNode)
	value, exists := root.Parameters["env.Region"]
	if !exists || !value.Equal(parameter.TextValue("east")) {
		t.Fatalf("env.Region = %#v, exists = %t", value, exists)
	}
}

func TestCompilePlanRejectsUnsealedZeroValue(t *testing.T) {
	_, err := CompilePlan(execution.RunSnapshot{})
	if err == nil {
		t.Fatal("unsealed zero-value run snapshot was accepted")
	}
}

func TestCompilerRequiresConcreteRootAndNestedInvocations(t *testing.T) {
	draft := minimalCompilerPlan()
	plan, err := execution.Seal(draft)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compileSnapshotDraft(plan.Snapshot(), execution.RunSnapshot{}); err == nil {
		t.Fatal("missing root invocation was accepted")
	}
	root := execution.WorkflowSnapshot{FlowFragmentID: "root", VersionID: "root-v1", DisplayName: "Root", Steps: []execution.Step{{ID: "call", DisplayName: "Call", Kind: execution.FlowFragmentReference, Reference: &execution.Reference{FlowFragmentID: "child", WorkflowVersionID: "child-v1"}}}}
	child := execution.WorkflowSnapshot{FlowFragmentID: "child", VersionID: "child-v1", DisplayName: "Child"}
	compiler := executionCompiler{
		versions:    map[string]execution.WorkflowSnapshot{"root-v1": root, "child-v1": child},
		resolutions: map[execution.WorkflowReferenceKey]execution.ReferenceResolution{{ParentVersionID: "root-v1", StepID: "call"}: {ParentVersionID: "root-v1", StepID: "call", FlowFragmentID: "child", WorkflowVersionID: "child-v1"}},
		invocations: map[execution.InvocationEdgeKey]execution.InvocationScopeSnapshot{}, metadata: map[string]StepMetadata{}, programSpecs: map[string]fingerprint.ElementTargetSpec{}, runtimeNodes: map[string]RuntimeNodeIdentity{},
	}
	count := 0
	compiler.compiledNodes = &count
	if _, err := compiler.compileWorkflow("root-v1", "root", "root", 1); err == nil {
		t.Fatal("missing concrete child invocation was accepted")
	}
}

func TestCompilePlanRejectsUnsealedPlanWithDomainError(t *testing.T) {
	_, err := compilePlanForTest(execution.Plan{})
	if !errors.Is(err, execution.ErrUnsealedPlan) {
		t.Fatalf("compilePlanForTest() error = %v, want ErrUnsealedPlan", err)
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
		RunID: "exec", Entries: []execution.WorkflowEntry{{ExecutionID: "execution-entry", TestTaskItemID: "task-item", FlowFragmentID: "root", WorkflowVersionID: "root-v1"}},
		Workflows: []execution.WorkflowSnapshot{
			{FlowFragmentID: "root", VersionID: "root-v1", DisplayName: "根流程", VersionNumber: 1, Steps: []execution.Step{{ID: "call", DisplayName: "调用子流程", Kind: execution.FlowFragmentReference, Reference: &execution.Reference{FlowFragmentID: "child", WorkflowVersionID: "child-v1", ParameterBindings: map[string]parameter.Binding{"region": parameter.LiteralBinding(parameter.TextValue("east"))}}}}},
			{FlowFragmentID: "child", VersionID: "child-v1", DisplayName: "子流程", VersionNumber: 1,
				Parameters: []execution.Parameter{{Name: "region", DisplayName: "Region", Type: parameter.Text, Default: parameter.PresentValue(parameter.TextValue("east"))}},
				Steps:      []execution.Step{{ID: "click", DisplayName: "点击", Kind: execution.ActionStep, Action: "click", NodeID: compilerNodeID, NodeVersionID: compilerNodeV1}}},
		},
		Nodes:      []execution.NodeSnapshot{compilerNodeSnapshot(compilerNodeV1, "submit-v1")},
		References: []execution.ReferenceResolution{{ParentVersionID: "root-v1", StepID: "call", FlowFragmentID: "child", WorkflowVersionID: "child-v1"}},
	}

	compiled, err := compileDraft(plan)
	if err != nil {
		t.Fatal(err)
	}
	root, ok := compiled.program.Root.(*node.WorkflowNode)
	if !ok || root.ID() != "workflow|15:execution-entry" || len(root.Children) != 1 {
		t.Fatalf("root = %#v", compiled.program.Root)
	}
	call, ok := root.Children[0].(*node.WorkflowCallNode)
	if !ok || call.Target == nil || call.Target.ID() != "workflow|15:execution-entry4:call8:child-v1" || !literalBindingEqual(call.Bindings["region"], parameter.TextValue("east")) {
		t.Fatalf("workflow call = %#v", root.Children[0])
	}
	childStepID := runtimeInvocationStepID("15:execution-entry4:call8:child-v1", "click")
	if got := compiled.Metadata[childStepID]; got.WorkflowStepID != "click" || got.NodeVersionID != compilerNodeV1 {
		t.Fatalf("metadata = %#v", got)
	}
}

func TestCompilePlanCreatesDistinctRuntimeIdentitiesForSharedChildInvocations(t *testing.T) {
	child := execution.WorkflowSnapshot{FlowFragmentID: "child", VersionID: "child-v1", DisplayName: "Child", VersionNumber: 1,
		Parameters: []execution.Parameter{{Name: "region", DisplayName: "Region", Type: parameter.Text, Required: true}},
		Steps:      []execution.Step{{ID: "click", DisplayName: "Click", Kind: execution.ActionStep, Action: "click", NodeID: compilerNodeID, NodeVersionID: compilerNodeV1}}}
	call := func(id string) execution.Step {
		return execution.Step{ID: id, DisplayName: id, Kind: execution.FlowFragmentReference, Reference: &execution.Reference{FlowFragmentID: "child", WorkflowVersionID: "child-v1", ParameterBindings: map[string]parameter.Binding{"region": parameter.LiteralBinding(parameter.TextValue("east"))}}}
	}
	plan := execution.Draft{RunID: "exec", FailurePolicy: execution.FailurePolicyStopOnFailure, Entries: []execution.WorkflowEntry{{ExecutionID: "execution-entry", TestTaskItemID: "task-item", SequenceNumber: 1, FlowFragmentID: "root", WorkflowVersionID: "root-v1"}}, Workflows: []execution.WorkflowSnapshot{
		{FlowFragmentID: "root", VersionID: "root-v1", DisplayName: "Root", VersionNumber: 1, Steps: []execution.Step{call("first"), call("second")}}, child},
		Nodes: []execution.NodeSnapshot{compilerNodeSnapshot(compilerNodeV1, "submit")}, References: []execution.ReferenceResolution{
			{ParentVersionID: "root-v1", StepID: "first", FlowFragmentID: "child", WorkflowVersionID: "child-v1"},
			{ParentVersionID: "root-v1", StepID: "second", FlowFragmentID: "child", WorkflowVersionID: "child-v1"},
		}}

	compiled, err := compileDraft(plan)
	if err != nil {
		t.Fatal(err)
	}
	children := compiled.program.Root.(*node.WorkflowNode).Children
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
	plan := execution.Draft{RunID: "exec", FailurePolicy: execution.FailurePolicyStopOnFailure, Entries: []execution.WorkflowEntry{{ExecutionID: "execution-entry", TestTaskItemID: "task-item", SequenceNumber: 1, FlowFragmentID: "checkout", WorkflowVersionID: "checkout-v1"}},
		Workflows: []execution.WorkflowSnapshot{{FlowFragmentID: "checkout", VersionID: "checkout-v1", DisplayName: "结账", VersionNumber: 1, Steps: []execution.Step{
			{ID: "old", DisplayName: "旧版", Kind: execution.ActionStep, Action: "click", NodeID: compilerNodeID, NodeVersionID: compilerNodeV1},
			{ID: "new", DisplayName: "新版", Kind: execution.ActionStep, Action: "click", NodeID: compilerNodeID, NodeVersionID: compilerNodeV2},
		}}}, Nodes: []execution.NodeSnapshot{compilerNodeSnapshot(compilerNodeV1, "checkout-old"), compilerNodeSnapshot(compilerNodeV2, "checkout-new")}}

	compiled, err := compileDraft(plan)
	if err != nil {
		t.Fatal(err)
	}
	children := compiled.program.Root.(*node.WorkflowNode).Children
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
		RunID: "exec", Entries: []execution.WorkflowEntry{{ExecutionID: "execution-entry", TestTaskItemID: "task-item", FlowFragmentID: "validation", WorkflowVersionID: "validation-v1"}},
		Workflows: []execution.WorkflowSnapshot{{
			FlowFragmentID: "validation", VersionID: "validation-v1", DisplayName: "验证", VersionNumber: 1,
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
	group := compiled.program.Root.(*node.WorkflowNode).Children[0].(*node.ValidationGroupNode)
	validation := group.Branches[0].Nodes[0]
	if validation.GroupID != runtimeWorkflowStepID("execution-entry", "group") || validation.BranchID != "success" || validation.Assertion.Expected != "成功" || validation.MaxWait != 2*time.Second {
		t.Fatalf("validation member = %#v", validation)
	}
	if !compiled.Metadata[runtimeWorkflowStepID("execution-entry", "group")].CaptureScreenshot || compiled.Metadata[runtimeWorkflowStepID("execution-entry", "member")].HierarchyPath != "验证 / 结果 / 成功 / 状态成功" {
		t.Fatalf("validation metadata = %#v", compiled.Metadata)
	}
}

func TestCompilePlanRejectsMissingSnapshotAndCycles(t *testing.T) {
	missing := execution.Draft{RunID: "exec", FailurePolicy: execution.FailurePolicyStopOnFailure, Entries: []execution.WorkflowEntry{{ExecutionID: "execution-entry", TestTaskItemID: "task-item", SequenceNumber: 1, FlowFragmentID: "root", WorkflowVersionID: "root-v1"}}, Workflows: []execution.WorkflowSnapshot{{FlowFragmentID: "root", VersionID: "root-v1", DisplayName: "root", VersionNumber: 1, Steps: []execution.Step{{ID: "click", DisplayName: "click", Kind: execution.ActionStep, Action: "click", NodeID: compilerNodeID, NodeVersionID: compilerNodeV1}}}}}
	if _, err := compileDraft(missing); err == nil {
		t.Fatal("missing node snapshot was accepted")
	}
	cycle := execution.Draft{RunID: "exec", FailurePolicy: execution.FailurePolicyStopOnFailure, Entries: []execution.WorkflowEntry{{ExecutionID: "execution-entry", TestTaskItemID: "task-item", SequenceNumber: 1, FlowFragmentID: "a", WorkflowVersionID: "a-v1"}}, Workflows: []execution.WorkflowSnapshot{
		{FlowFragmentID: "a", VersionID: "a-v1", DisplayName: "a", VersionNumber: 1, Steps: []execution.Step{{ID: "to-b", DisplayName: "b", Kind: execution.FlowFragmentReference, Reference: &execution.Reference{FlowFragmentID: "b"}}}},
		{FlowFragmentID: "b", VersionID: "b-v1", DisplayName: "b", VersionNumber: 1, Steps: []execution.Step{{ID: "to-a", DisplayName: "a", Kind: execution.FlowFragmentReference, Reference: &execution.Reference{FlowFragmentID: "a"}}}},
	}, References: []execution.ReferenceResolution{{ParentVersionID: "a-v1", StepID: "to-b", FlowFragmentID: "b", WorkflowVersionID: "b-v1"}, {ParentVersionID: "b-v1", StepID: "to-a", FlowFragmentID: "a", WorkflowVersionID: "a-v1"}}}
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
	compiled, err := compilePlanForTest(plan)
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
		{ExecutionID: "execution-a", TestTaskItemID: "item-a", SequenceNumber: 1, FlowFragmentID: "root", WorkflowVersionID: "root-v1"},
		{ExecutionID: "execution-b", TestTaskItemID: "item-b", SequenceNumber: 2, FlowFragmentID: "root", WorkflowVersionID: "root-v1"},
	}
	plan, err := execution.Seal(draft)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := compilePlanForTest(plan)
	if err != nil {
		t.Fatal(err)
	}
	entries := compiled.Entries()
	if len(entries) != 2 {
		t.Fatalf("entries = %d", len(entries))
	}
	if entries[0].ExecutionID != "execution-a" || entries[1].ExecutionID != "execution-b" {
		t.Fatalf("entry declaration order not preserved: %q, %q", entries[0].ExecutionID, entries[1].ExecutionID)
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
	if a.program.Root.ID() == b.program.Root.ID() {
		t.Fatalf("runtime roots collide: %q", a.program.Root.ID())
	}
	if &a.Metadata == &b.Metadata || a.program.Root.ID() != "workflow|11:execution-a" {
		t.Fatalf("entries not occurrence-specific")
	}
}
