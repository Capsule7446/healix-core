package execution

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

func TestWorkflowSnapshotValidateRejectsMalformedExecutionContracts(t *testing.T) {
	t.Parallel()
	valid := validWorkflowSnapshot()
	tests := []struct {
		name      string
		edit      func(*WorkflowSnapshot)
		wantField string
		wantCode  fault.Code
	}{
		{"identity", func(w *WorkflowSnapshot) { w.VersionID = "" }, "flowFragmentId", fault.CodeFieldInvalid},
		{"display name", func(w *WorkflowSnapshot) { w.DisplayName = " " }, "displayName", fault.CodeFieldRequired},
		{"version number", func(w *WorkflowSnapshot) { w.VersionNumber = 0 }, "versionNumber", fault.CodeFieldInvalid},
		{"duplicate parameters", func(w *WorkflowSnapshot) { w.Parameters = []Parameter{{Name: "query"}, {Name: "query"}} }, "parameters.1", fault.CodeFieldDuplicate},
		{"duplicate recursive ids", func(w *WorkflowSnapshot) {
			w.Steps = []Step{{ID: "repeat", DisplayName: "repeat", Kind: RepeatStep, RepeatCount: 1, Children: []Step{{ID: "repeat", DisplayName: "child", Kind: ActionStep, Action: "noop", ElementTargetID: "node", ElementTargetVersionID: "v1"}}}}
		}, "steps", fault.CodeFieldDuplicate},
		{"unsupported action", func(w *WorkflowSnapshot) { w.Steps[0].Action = "double_click" }, "steps", fault.CodeFieldInvalid},
		{"node version", func(w *WorkflowSnapshot) { w.Steps[0].ElementTargetVersionID = "" }, "steps", fault.CodeFieldRequired},
		{"wait kind", func(w *WorkflowSnapshot) {
			w.Steps[0] = Step{ID: "wait", DisplayName: "wait", Kind: WaitStep, WaitKind: "event"}
		}, "steps", fault.CodeFieldInvalid},
		{"repeat count", func(w *WorkflowSnapshot) {
			w.Steps[0] = Step{ID: "repeat", DisplayName: "repeat", Kind: RepeatStep, Children: []Step{{ID: "child", DisplayName: "child", Kind: ActionStep, Action: "noop", ElementTargetID: "node", ElementTargetVersionID: "v1"}}}
		}, "steps", fault.CodeFieldRequired},
		{"optional restriction", func(w *WorkflowSnapshot) {
			w.Steps[0] = Step{ID: "wait", DisplayName: "wait", Kind: WaitStep, WaitMS: 1, Optional: true}
		}, "steps", fault.CodeFieldInvalid},
		{"reference shape", func(w *WorkflowSnapshot) {
			w.Steps[0] = Step{ID: "ref", DisplayName: "ref", Kind: FlowFragmentReference, Reference: &Reference{}}
		}, "steps", fault.CodeFieldRequired},
		{"validation assertion", func(w *WorkflowSnapshot) { w.Steps[0] = standaloneValidation("unknown", 1000, 200) }, "steps", fault.CodeFieldInvalid},
		{"validation wait bound", func(w *WorkflowSnapshot) { w.Steps[0] = standaloneValidation("visible", 999, 200) }, "steps", fault.CodeFieldInvalid},
		{"group structure", func(w *WorkflowSnapshot) {
			w.Steps[0] = Step{ID: "group", DisplayName: "group", Kind: ValidationGroupStep, ValidationGroup: &ValidationGroup{MaxWaitMS: 1000, StabilityMS: 200}}
		}, "steps", fault.CodeFieldInvalid},
		{"group member wait", func(w *WorkflowSnapshot) {
			member := standaloneValidation("visible", 1000, 200)
			w.Steps[0] = Step{ID: "group", DisplayName: "group", Kind: ValidationGroupStep, ValidationGroup: &ValidationGroup{MaxWaitMS: 1000, StabilityMS: 200, Branches: []ValidationBranch{{ID: "branch", Name: "branch", Steps: []Step{member}}}}}
		}, "steps", fault.CodeFieldInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflow := valid
			workflow.Steps = append([]Step(nil), valid.Steps...)
			tt.edit(&workflow)
			requireStepShapeViolation(t, workflow.Validate(), tt.wantField, tt.wantCode)
		})
	}
}

func TestDraftValidateRejectsInvalidNodeSnapshots(t *testing.T) {
	validNode := validNodeSnapshot("00000000-0000-0000-0000-000000000001", "node-v1")
	tests := []struct {
		name      string
		edit      func(*NodeSnapshot)
		wantCode  fault.Code
		wantField string
	}{
		{"selector", func(node *NodeSnapshot) { node.Selectors[0].Value = " " }, fault.CodeFieldInvalid, "selectors.0"},
		{"fingerprint", func(node *NodeSnapshot) { node.Fingerprint.Tag = "" }, fault.CodeFieldRequired, "fingerprint.tag"},
		{"spec identity", func(node *NodeSnapshot) { node.ElementTargetID = "not-a-uuid" }, fault.CodeFieldInvalid, "uuid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node := validNode
			node.Selectors = append([]fingerprint.Selector(nil), validNode.Selectors...)
			test.edit(&node)
			draft := validDraftWithNodes(node)
			err := draft.Validate()
			if !fault.IsCode(err, fingerprint.CodeElementTargetSpecInvalid) {
				t.Fatalf("Validate() error = %v, want code %s", err, fingerprint.CodeElementTargetSpecInvalid)
			}
			descriptor, ok := fault.Describe(err)
			if !ok {
				t.Fatalf("Validate() error is not a fault: %v", err)
			}
			found := false
			for _, violation := range descriptor.Violations() {
				if violation.Code() == test.wantCode && violation.Field() == test.wantField {
					found = true
				}
			}
			if !found {
				t.Fatalf("Validate() violations = %#v, want %s at %q", descriptor.Violations(), test.wantCode, test.wantField)
			}
		})
	}
}

func TestDraftValidateRejectsNodeVersionOwnedByDifferentNodes(t *testing.T) {
	draft := validDraftWithNodes(
		validNodeSnapshot("00000000-0000-0000-0000-000000000001", "shared-v1"),
		validNodeSnapshot("00000000-0000-0000-0000-000000000002", "shared-v1"),
	)
	draft.Workflows[0].Steps[0].ElementTargetID = draft.Nodes[0].ElementTargetID
	draft.Workflows[0].Steps[0].ElementTargetVersionID = "shared-v1"
	requireCreateInstancePlanRejection(t, draft.Validate(), "owned by different nodes")
}

func TestDraftValidateRejectsReachableWorkflowReferenceCycles(t *testing.T) {
	tests := []struct {
		name  string
		edges map[string]string
	}{
		{"direct", map[string]string{"a": "a"}},
		{"indirect", map[string]string{"a": "b", "b": "c", "c": "a"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			draft := workflowReferenceDraft(test.edges)
			requireCreateInstancePlanRejection(t, draft.Validate(), "workflow reference cycle")
		})
	}
}

func TestPlanSealState(t *testing.T) {
	validDraft := validDraftWithNodes(validNodeSnapshot("00000000-0000-0000-0000-000000000001", "v1"))
	tests := []struct {
		name       string
		build      func() (Plan, error)
		wantSealed bool
		wantErr    bool
	}{
		{name: "zero plan", build: func() (Plan, error) { return Plan{}, nil }},
		{name: "successful copied plan", build: func() (Plan, error) {
			plan, err := Seal(validDraft)
			copy := plan
			return copy, err
		}, wantSealed: true},
		{name: "failed seal", build: func() (Plan, error) { return Seal(PlanSnapshot{}) }, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan, err := test.build()
			if (err != nil) != test.wantErr {
				t.Fatalf("Seal() error = %v, wantErr %v", err, test.wantErr)
			}
			if got := plan.IsSealed(); got != test.wantSealed {
				t.Fatalf("IsSealed() = %v, want %v", got, test.wantSealed)
			}
			if err := plan.Validate(); !test.wantSealed && !fault.IsCode(err, CodePlanUnsealed) {
				t.Fatalf("Validate() error = %v, want %v", err, CodePlanUnsealed)
			} else if test.wantSealed && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestSealRejectsBlankAndMismatchedFixedWorkflowVersions(t *testing.T) {
	child := validWorkflowSnapshot()
	child.FlowFragmentID, child.ID, child.VersionID = "child", "child", "child-v1"
	for _, test := range []struct{ name, fixed, resolved, want string }{
		{"blank", "", "child-v1", "exact workflow version"},
		{"mismatch", "child-v2", "child-v1", "disagrees"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := validWorkflowSnapshot()
			root.Steps = []Step{{ID: "call", DisplayName: "Call", Kind: FlowFragmentReference, Reference: &Reference{FlowFragmentID: "child", WorkflowVersionID: test.fixed}}}
			_, err := Seal(PlanSnapshot{InstanceID: mustInstanceID("run"), FailurePolicy: FailurePolicyStopOnFailure, Entries: []Entry{{ID: mustEntryID("execution-entry"), TestTaskItemID: "task-item", SequenceNumber: 1, FlowFragmentID: root.FlowFragmentID, WorkflowVersionID: root.VersionID}}, Workflows: []WorkflowSnapshot{root, child}, References: []ReferenceResolution{{ParentVersionID: root.VersionID, StepID: "call", FlowFragmentID: "child", WorkflowVersionID: test.resolved}}})
			requireCreateInstancePlanRejection(t, err, test.want)
		})
	}
}

func TestSealOwnsNestedMutableValues(t *testing.T) {
	draft := validDraftWithNodes(validNodeSnapshot("00000000-0000-0000-0000-000000000001", "v1"))
	draft.Workflows[0].Steps[0].Values = []string{"original"}
	plan, err := Seal(draft)
	if err != nil {
		t.Fatal(err)
	}
	draft.Workflows[0].Steps[0].Values[0] = "mutated"
	snapshot := plan.Snapshot()
	if snapshot.Workflows[0].Steps[0].Values[0] != "original" {
		t.Fatal("sealed plan retained caller-owned nested slice")
	}
	snapshot.Workflows[0].Steps[0].Values[0] = "accessor mutation"
	if plan.Snapshot().Workflows[0].Steps[0].Values[0] != "original" {
		t.Fatal("snapshot exposed plan internals")
	}
}

func TestWorkflowSnapshotValidateBoundsNestingWithoutOverflow(t *testing.T) {
	step := Step{ID: "leaf", DisplayName: "leaf", Kind: ActionStep, Action: "noop", ElementTargetID: "node", ElementTargetVersionID: "v1"}
	for depth := 0; depth < MaxStepNestingDepth; depth++ {
		step = Step{ID: fmt.Sprintf("repeat-%d", depth), DisplayName: "repeat", Kind: RepeatStep, RepeatCount: 1, Children: []Step{step}}
	}
	workflow := validWorkflowSnapshot()
	workflow.Steps = []Step{step}
	requireStepShapeViolation(t, workflow.Validate(), "steps", fault.CodeFieldInvalid)
}

func TestParameterValidateRequiresCompleteDefinition(t *testing.T) {
	t.Parallel()
	if err := (Parameter{Name: "query", DisplayName: "Query", Type: parameter.Text, Required: true}).Validate(); err != nil {
		t.Fatalf("execution parameter rejected: %v", err)
	}
	if err := (Parameter{}).Validate(); err == nil {
		t.Fatal("empty execution parameter accepted")
	}
}

func TestSealedPlanAccessorsAndNestedCopies(t *testing.T) {
	node := validNodeSnapshot("00000000-0000-0000-0000-000000000001", "v1")
	member := standaloneValidation("visible", 0, 0)
	member.ElementTargetID, member.ElementTargetVersionID = node.ElementTargetID, node.VersionID
	workflow := validWorkflowSnapshot()
	workflow.Steps = []Step{
		{ID: "repeat", DisplayName: "Repeat", Kind: RepeatStep, RepeatCount: 1, Children: []Step{{ID: "wait", DisplayName: "Wait", Kind: WaitStep, WaitMS: 1}}, Values: nil},
		{ID: "group", DisplayName: "Group", Kind: ValidationGroupStep, ValidationGroup: &ValidationGroup{MaxWaitMS: 1000, StabilityMS: 200, Branches: []ValidationBranch{{ID: "ok", Name: "OK", Steps: []Step{member}}}}},
	}
	draft := PlanSnapshot{InstanceID: mustInstanceID("run"), FailurePolicy: FailurePolicyStopOnFailure, Entries: []Entry{{ID: mustEntryID("execution-entry"), TestTaskItemID: "task-item", SequenceNumber: 1, FlowFragmentID: workflow.FlowFragmentID, WorkflowVersionID: workflow.VersionID}}, Workflows: []WorkflowSnapshot{workflow}, Nodes: []NodeSnapshot{node}}
	plan, err := Seal(draft)
	if err != nil {
		t.Fatal(err)
	}
	if plan.InstanceID() != mustInstanceID("run") || plan.Entries()[0].WorkflowVersionID != workflow.VersionID || len(plan.Workflows()) != 1 || len(plan.Nodes()) != 1 || len(plan.References()) != 0 {
		t.Fatal("sealed plan accessors returned unexpected values")
	}
	workflows := plan.Workflows()
	workflows[0].Steps[0].Children[0].DisplayName = "changed"
	workflows[0].Steps[1].ValidationGroup.Branches[0].Steps[0].DisplayName = "changed"
	if plan.Workflows()[0].Steps[0].Children[0].DisplayName == "changed" || plan.Workflows()[0].Steps[1].ValidationGroup.Branches[0].Steps[0].DisplayName == "changed" {
		t.Fatal("accessors exposed nested plan state")
	}
}

func TestValidationValidateBranches(t *testing.T) {
	tests := []struct {
		name    string
		value   Validation
		wait    bool
		wantErr bool
	}{
		{"boolean options", Validation{Kind: "visible", Expected: "x"}, false, true},
		{"scalar options", Validation{Kind: "text_equals", ExpectedValues: []string{"x"}}, false, true},
		{"invalid regexp", Validation{Kind: "text_matches", Expected: "["}, false, true},
		{"variable regexp", Validation{Kind: "text_matches", Expected: "${pattern}"}, false, false},
		{"set options", Validation{Kind: "selected_set_equals", Expected: "x"}, false, true},
		{"attribute missing", Validation{Kind: "attribute_equals"}, false, true},
		{"attribute variable", Validation{Kind: "attribute_contains", Attribute: "${name}"}, false, true},
		{"valid wait", Validation{Kind: "visible", MaxWaitMS: 1000, StabilityMS: 200}, true, false},
		{"stability too long", Validation{Kind: "visible", MaxWaitMS: 1000, StabilityMS: 1000}, true, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.value.Validate(test.wait)
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestDraftValidateAggregatesReferencedWorkflowBudgetAcrossCallsAndRepeats(t *testing.T) {
	child := WorkflowSnapshot{ID: "child", FlowFragmentID: "child", VersionID: "child-v1", DisplayName: "Child", VersionNumber: 1,
		Steps: []Step{{ID: "wait", DisplayName: "Wait", Kind: WaitStep, WaitKind: "sleep", WaitMS: MaxWaitMS}}}
	ref := func(id string) Step {
		return Step{ID: id, DisplayName: id, Kind: FlowFragmentReference, Reference: &Reference{FlowFragmentID: "child", WorkflowVersionID: "child-v1"}}
	}
	root := WorkflowSnapshot{ID: "root", FlowFragmentID: "root", VersionID: "root-v1", DisplayName: "Root", VersionNumber: 1,
		Steps: []Step{{ID: "outer", DisplayName: "Outer", Kind: RepeatStep, RepeatCount: 1000, Children: []Step{
			{ID: "inner", DisplayName: "Inner", Kind: RepeatStep, RepeatCount: 2, Children: []Step{ref("first"), ref("second")}},
		}}}}
	draft := PlanSnapshot{InstanceID: mustInstanceID("run"), FailurePolicy: FailurePolicyStopOnFailure, Entries: []Entry{{ID: mustEntryID("execution-entry"), TestTaskItemID: "task-item", SequenceNumber: 1, FlowFragmentID: root.FlowFragmentID, WorkflowVersionID: root.VersionID}}, Workflows: []WorkflowSnapshot{root, child}, References: []ReferenceResolution{
		{ParentVersionID: root.VersionID, StepID: "first", FlowFragmentID: "child", WorkflowVersionID: child.VersionID},
		{ParentVersionID: root.VersionID, StepID: "second", FlowFragmentID: "child", WorkflowVersionID: child.VersionID},
	}}
	requireCreateInstancePlanRejection(t, draft.Validate(), "cumulative wait")
}

func TestExecutionBudgetCountsValidationBranchWork(t *testing.T) {
	member := func(id string) Step {
		return Step{ID: id, DisplayName: id, Kind: ValidationStep, Validation: &Validation{Kind: "visible"}}
	}
	branches := []ValidationBranch{{ID: "a", Name: "A", Steps: []Step{member("a")}}, {ID: "b", Name: "B", Steps: []Step{member("b")}}}
	group := Step{ID: "group", DisplayName: "Group", Kind: ValidationGroupStep, ValidationGroup: &ValidationGroup{MaxWaitMS: MaxWaitMS, StabilityMS: 200, Branches: branches}}

	cost, err := executionStepsCost("root-v1", []Step{group}, 1, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cost.executions != 3 {
		t.Fatalf("executions = %d, want group plus both branch members", cost.executions)
	}
}

func TestExecutionBudgetMemoizesSharedDiamondAndChargesEveryCallSite(t *testing.T) {
	ref := func(id, workflowID, versionID string) Step {
		return Step{ID: id, DisplayName: id, Kind: FlowFragmentReference, Reference: &Reference{FlowFragmentID: workflowID, WorkflowVersionID: versionID}}
	}
	leaf := WorkflowSnapshot{FlowFragmentID: "leaf", VersionID: "leaf-v1", Steps: []Step{{ID: "wait", Kind: WaitStep, WaitMS: 10}}}
	left := WorkflowSnapshot{FlowFragmentID: "left", VersionID: "left-v1", Steps: []Step{ref("left-leaf", "leaf", leaf.VersionID)}}
	right := WorkflowSnapshot{FlowFragmentID: "right", VersionID: "right-v1", Steps: []Step{ref("right-leaf", "leaf", leaf.VersionID)}}
	root := WorkflowSnapshot{FlowFragmentID: "root", VersionID: "root-v1", Steps: []Step{ref("root-left", "left", left.VersionID), ref("root-right", "right", right.VersionID)}}
	workflows := map[string]WorkflowSnapshot{root.VersionID: root, left.VersionID: left, right.VersionID: right, leaf.VersionID: leaf}
	resolutions := map[WorkflowReferenceKey]ReferenceResolution{
		{ParentVersionID: root.VersionID, StepID: "root-left"}:   {WorkflowVersionID: left.VersionID},
		{ParentVersionID: root.VersionID, StepID: "root-right"}:  {WorkflowVersionID: right.VersionID},
		{ParentVersionID: left.VersionID, StepID: "left-leaf"}:   {WorkflowVersionID: leaf.VersionID},
		{ParentVersionID: right.VersionID, StepID: "right-leaf"}: {WorkflowVersionID: leaf.VersionID},
	}

	evaluator := newExecutionBudgetEvaluator(workflows, resolutions)
	cost, err := evaluator.workflowCost(root.VersionID)
	if err != nil {
		t.Fatal(err)
	}
	if cost.executions != 6 || cost.waitMS != 20 {
		t.Fatalf("cost = %+v, want 6 executions and 20ms (shared leaf charged twice)", cost)
	}
	for versionID, visits := range evaluator.visits {
		if visits != 1 {
			t.Fatalf("workflow %s visits = %d, want 1", versionID, visits)
		}
	}
}

func TestExecutionBudgetSharedDiamondGrowthIsLinear(t *testing.T) {
	const levels = 30
	workflows := make(map[string]WorkflowSnapshot, levels+1)
	resolutions := make(map[WorkflowReferenceKey]ReferenceResolution, levels*2)
	leafVersion := "v30"
	workflows[leafVersion] = WorkflowSnapshot{FlowFragmentID: "leaf", VersionID: leafVersion, Steps: []Step{{ID: "leaf", Kind: ActionStep}}}
	for level := levels - 1; level >= 0; level-- {
		versionID := fmt.Sprintf("v%d", level)
		childVersionID := fmt.Sprintf("v%d", level+1)
		firstID, secondID := fmt.Sprintf("a%d", level), fmt.Sprintf("b%d", level)
		workflows[versionID] = WorkflowSnapshot{FlowFragmentID: versionID, VersionID: versionID, Steps: []Step{{ID: firstID, Kind: FlowFragmentReference}, {ID: secondID, Kind: FlowFragmentReference}}}
		resolutions[WorkflowReferenceKey{ParentVersionID: versionID, StepID: firstID}] = ReferenceResolution{WorkflowVersionID: childVersionID}
		resolutions[WorkflowReferenceKey{ParentVersionID: versionID, StepID: secondID}] = ReferenceResolution{WorkflowVersionID: childVersionID}
	}

	evaluator := newExecutionBudgetEvaluator(workflows, resolutions)
	cost, err := evaluator.workflowCost("v0")
	if err != nil {
		t.Fatal(err)
	}
	if cost.executions != MaxExpandedExecutions+1 {
		t.Fatalf("executions = %d, want capped rejection value", cost.executions)
	}
	if len(evaluator.visits) != levels+1 {
		t.Fatalf("visited workflows = %d, want %d", len(evaluator.visits), levels+1)
	}
	for versionID, visits := range evaluator.visits {
		if visits != 1 {
			t.Fatalf("workflow %s visits = %d, want 1", versionID, visits)
		}
	}
}

func TestDraftValidateReportsOrphanResolutionDeterministically(t *testing.T) {
	workflow := validWorkflowSnapshot()
	workflow.Steps[0].ElementTargetID = "00000000-0000-7000-8000-000000000001"
	workflow.Steps[0].ElementTargetVersionID = "00000000-0000-7000-8000-000000000002"
	draft := PlanSnapshot{InstanceID: mustInstanceID("run"), FailurePolicy: FailurePolicyStopOnFailure, Entries: []Entry{{ID: mustEntryID("execution-entry"), TestTaskItemID: "task-item", SequenceNumber: 1, FlowFragmentID: workflow.FlowFragmentID, WorkflowVersionID: workflow.VersionID}}, Workflows: []WorkflowSnapshot{workflow}, Nodes: []NodeSnapshot{validNodeSnapshot(workflow.Steps[0].ElementTargetID, workflow.Steps[0].ElementTargetVersionID)}, References: []ReferenceResolution{
		{ParentVersionID: "z", StepID: "z", FlowFragmentID: "unused", WorkflowVersionID: "unused-v1"},
		{ParentVersionID: "a", StepID: "a", FlowFragmentID: "unused", WorkflowVersionID: "unused-v1"},
	}}
	for i := 0; i < 20; i++ {
		requireCreateInstancePlanRejection(t, draft.Validate(), `{"a" "a"}`)
	}
}

func TestNavigateSealValidation(t *testing.T) {
	tests := []struct {
		value string
		valid bool
	}{
		{"https://example.com/path", true},
		{"http://example.com", true},
		{"${base}/path", false},
		{"https://${host}/path", false},
		{"/relative", false},
		{"ftp://example.com/${path}", false},
		{"https://${}/path", false},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			workflow := validWorkflowSnapshot()
			workflow.Steps[0] = Step{ID: "navigate", DisplayName: "Navigate", Kind: ActionStep, Action: "navigate", Value: test.value}
			draft := PlanSnapshot{InstanceID: mustInstanceID("run"), FailurePolicy: FailurePolicyStopOnFailure, Entries: []Entry{{ID: mustEntryID("execution-entry"), TestTaskItemID: "task-item", SequenceNumber: 1, FlowFragmentID: workflow.FlowFragmentID, WorkflowVersionID: workflow.VersionID}}, Workflows: []WorkflowSnapshot{workflow}}
			err := draft.Validate()
			if (err == nil) != test.valid {
				t.Fatalf("Validate() error = %v, valid = %v", err, test.valid)
			}
		})
	}
}

func TestAggregateInputLimitsRejectTopLevelEntriesBeforeSealClone(t *testing.T) {
	draft := PlanSnapshot{Entries: make([]Entry, MaxAggregateCollectionElements+1)}
	if err := validateAggregateInputBounds(draft); err == nil || !strings.Contains(err.Error(), "aggregate collection elements") {
		t.Fatalf("aggregate preflight error = %v", err)
	}
}

func TestAggregateInputLimitsBeforeSealClone(t *testing.T) {
	draft := validDraftWithNodes(validNodeSnapshot("00000000-0000-0000-0000-000000000001", "v1"))
	draft.Workflows[0].Steps[0].DisplayName = strings.Repeat("x", MaxStringBytes+1)
	_, err := Seal(draft)
	requireCreateInstancePlanRejection(t, err, "string exceeds")

	draft = validDraftWithNodes(validNodeSnapshot("00000000-0000-0000-0000-000000000001", "v1"))
	draft.Workflows = make([]WorkflowSnapshot, MaxDraftWorkflows+1)
	requireCreateInstancePlanRejection(t, draft.Validate(), "collection limit")
}

func TestExecutionBudgetAggregatesAllEntryOccurrences(t *testing.T) {
	workflow := WorkflowSnapshot{VersionID: "root", Steps: []Step{{ID: "repeat", Kind: RepeatStep, RepeatCount: 500_000, Children: []Step{{ID: "child", Kind: ActionStep}}}}}
	workflows := map[string]WorkflowSnapshot{"root": workflow}

	if err := validateExecutionBudget([]string{"root"}, workflows, nil); err != nil {
		t.Fatalf("single entry should fit: %v", err)
	}
	if err := validateExecutionBudget([]string{"root", "root"}, workflows, nil); err == nil || !strings.Contains(err.Error(), "expanded executions") {
		t.Fatalf("two entry occurrences should exceed aggregate budget: %v", err)
	}
}

func TestExecutionBudgetChargesSharedTargetPerEntryButEvaluatesOnce(t *testing.T) {
	workflows := map[string]WorkflowSnapshot{"shared": {VersionID: "shared", Steps: []Step{{ID: "wait", Kind: WaitStep, WaitMS: 7}}}}
	evaluator := newExecutionBudgetEvaluator(workflows, nil)
	total := executionCost{}
	for range 2 {
		cost, err := evaluator.workflowCost("shared")
		if err != nil {
			t.Fatal(err)
		}
		total = addCost(total, cost)
	}
	if evaluator.visits["shared"] != 1 {
		t.Fatalf("shared workflow evaluations = %d, want 1", evaluator.visits["shared"])
	}
	if total != (executionCost{executions: 2, waitMS: 14}) {
		t.Fatalf("aggregate cost = %+v", total)
	}
}

func TestExecutionBudgetAcceptsExactAggregateLimits(t *testing.T) {
	workflow := WorkflowSnapshot{VersionID: "root", Steps: []Step{{ID: "repeat", Kind: RepeatStep, RepeatCount: MaxExpandedExecutions - 1, Children: []Step{{ID: "wait", Kind: WaitStep, WaitMS: MaxCumulativeWaitMS / (MaxExpandedExecutions - 1)}}}}}
	if err := validateExecutionBudget([]string{"root"}, map[string]WorkflowSnapshot{"root": workflow}, nil); err != nil {
		t.Fatalf("exact execution aggregate should fit: %v", err)
	}

	waitWorkflow := WorkflowSnapshot{VersionID: "wait", Steps: []Step{{ID: "wait", Kind: WaitStep, WaitMS: MaxCumulativeWaitMS / 2}}}
	if err := validateExecutionBudget([]string{"wait", "wait"}, map[string]WorkflowSnapshot{"wait": waitWorkflow}, nil); err != nil {
		t.Fatalf("exact wait aggregate should fit: %v", err)
	}
}

func TestAggregateCollectionLimitsRejectEmptyEntriesBeforeSealClone(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*PlanSnapshot)
	}{
		{name: "step values", mutate: func(draft *PlanSnapshot) {
			draft.Workflows[0].Steps[0].Values = make([]string, MaxAggregateCollectionElements+1)
		}},
		{name: "validation expected values", mutate: func(draft *PlanSnapshot) {
			draft.Workflows[0].Steps[0].Validation = &Validation{ExpectedValues: make([]string, MaxAggregateCollectionElements+1)}
		}},
		{name: "validation group branches", mutate: func(draft *PlanSnapshot) {
			draft.Workflows[0].Steps[0].ValidationGroup = &ValidationGroup{Branches: make([]ValidationBranch, MaxAggregateCollectionElements+1)}
		}},
		{name: "fingerprint frameworks", mutate: func(draft *PlanSnapshot) {
			draft.Nodes[0].Fingerprint.Framework = make(fingerprint.FrameworkStack, MaxAggregateCollectionElements+1)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			draft := validDraftWithNodes(validNodeSnapshot("00000000-0000-0000-0000-000000000001", "v1"))
			test.mutate(&draft)
			_, err := Seal(draft)
			requireCreateInstancePlanRejection(t, err, "aggregate collection elements")
		})
	}
}

func TestExecutionStatusTransitionMatrix(t *testing.T) {
	statuses := []EntryStatus{EntryPending, EntryRunning, EntrySucceeded, EntryFailed, EntryCanceled, EntryAborted}
	allowed := map[[2]EntryStatus]bool{
		{EntryPending, EntryRunning}: true, {EntryPending, EntryFailed}: true, {EntryPending, EntryCanceled}: true,
		{EntryRunning, EntrySucceeded}: true, {EntryRunning, EntryFailed}: true, {EntryRunning, EntryCanceled}: true, {EntryRunning, EntryAborted}: true,
	}
	for _, from := range statuses {
		for _, to := range statuses {
			err := ValidateEntryStatusTransition(from, to)
			if (err == nil) != allowed[[2]EntryStatus{from, to}] {
				t.Fatalf("transition %s -> %s error = %v", from, to, err)
			}
		}
	}
}

func TestExecutionStatusRejectsUnknownAndTerminalTransitions(t *testing.T) {
	tests := []struct {
		name string
		from EntryStatus
		to   EntryStatus
	}{
		{"unknown source", EntryStatus("UNKNOWN"), EntryRunning},
		{"unknown target", EntryRunning, EntryStatus("UNKNOWN")},
		{"succeeded is terminal", EntrySucceeded, EntryRunning},
		{"failed is terminal", EntryFailed, EntryRunning},
		{"canceled is terminal", EntryCanceled, EntryRunning},
		{"aborted is terminal", EntryAborted, EntryRunning},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateEntryStatusTransition(test.from, test.to); !fault.IsCode(err, CodeStatusTransitionInvalid) {
				t.Fatalf("transition %s -> %s error = %v, want ErrInvalidExecutionStatusTransition", test.from, test.to, err)
			}
		})
	}
}

func validNodeSnapshot(nodeID, versionID string) NodeSnapshot {
	return NodeSnapshot{
		ElementTargetID: nodeID, VersionID: versionID, DisplayName: "ElementTarget",
		Selectors:   []fingerprint.Selector{{Type: fingerprint.SelectorTestID, Value: "submit"}},
		Fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{}},
	}
}

func validDraftWithNodes(nodes ...NodeSnapshot) PlanSnapshot {
	workflow := validWorkflowSnapshot()
	workflow.Steps[0].ElementTargetID = nodes[0].ElementTargetID
	workflow.Steps[0].ElementTargetVersionID = nodes[0].VersionID
	return PlanSnapshot{InstanceID: mustInstanceID("run"), FailurePolicy: FailurePolicyStopOnFailure, Entries: []Entry{{ID: mustEntryID("execution-entry"), TestTaskItemID: "task-item", SequenceNumber: 1, FlowFragmentID: workflow.FlowFragmentID, WorkflowVersionID: workflow.VersionID}}, Workflows: []WorkflowSnapshot{workflow}, Nodes: nodes}
}

func workflowReferenceDraft(edges map[string]string) PlanSnapshot {
	workflows := make([]WorkflowSnapshot, 0, len(edges))
	resolutions := make([]ReferenceResolution, 0, len(edges))
	for parent, child := range edges {
		parentVersion, childVersion := parent+"-v1", child+"-v1"
		stepID := "to-" + child
		workflows = append(workflows, WorkflowSnapshot{ID: parent, FlowFragmentID: parent, VersionID: parentVersion, DisplayName: parent, VersionNumber: 1, Steps: []Step{{ID: stepID, DisplayName: stepID, Kind: FlowFragmentReference, Reference: &Reference{FlowFragmentID: child, WorkflowVersionID: childVersion}}}})
		resolutions = append(resolutions, ReferenceResolution{ParentVersionID: parentVersion, StepID: stepID, FlowFragmentID: child, WorkflowVersionID: childVersion})
	}
	return PlanSnapshot{InstanceID: mustInstanceID("run"), FailurePolicy: FailurePolicyStopOnFailure, Entries: []Entry{{ID: mustEntryID("execution-entry"), TestTaskItemID: "task-item", SequenceNumber: 1, FlowFragmentID: "a", WorkflowVersionID: "a-v1"}}, Workflows: workflows, References: resolutions}
}

func validWorkflowSnapshot() WorkflowSnapshot {
	return WorkflowSnapshot{ID: "workflow", FlowFragmentID: "workflow", VersionID: "workflow-v1", DisplayName: "FlowFragment", VersionNumber: 1, Steps: []Step{{ID: "click", DisplayName: "Click", Kind: ActionStep, Action: "click", ElementTargetID: "node", ElementTargetVersionID: "v1"}}}
}

func standaloneValidation(kind string, maxWait, stability int) Step {
	return Step{ID: "validation", DisplayName: "Validation", Kind: ValidationStep, ElementTargetID: "node", ElementTargetVersionID: "v1", Validation: &Validation{Kind: kind, MaxWaitMS: maxWait, StabilityMS: stability}}
}
