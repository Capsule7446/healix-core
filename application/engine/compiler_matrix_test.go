package engine

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/node"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

// requireEnginePlanRejection asserts execution's boundary-classification
// contract for EXECUTION_CREATE_INSTANCE_PLAN_INVALID: the bare internal
// detail is retained only on the private cause, never the public message.
func requireEnginePlanRejection(t *testing.T, err error, wantDetail string) {
	t.Helper()
	if !fault.IsCode(err, execution.CodeCreateInstancePlanInvalid) {
		t.Fatalf("error = %v, want code %s", err, execution.CodeCreateInstancePlanInvalid)
	}
	descriptor, ok := fault.Describe(err)
	if !ok {
		t.Fatalf("error is not a fault: %v", err)
	}
	if strings.Contains(descriptor.Message(), wantDetail) {
		t.Fatalf("public message %q carries the detail %q", descriptor.Message(), wantDetail)
	}
	cause := errors.Unwrap(err)
	if cause == nil || !strings.Contains(cause.Error(), wantDetail) {
		t.Fatalf("private cause = %v, want it to retain %q", cause, wantDetail)
	}
}

// requireEngineStepShapeViolation asserts that err is execution's step-shape
// envelope and carries a violation at wantField with wantCode.
func requireEngineStepShapeViolation(t *testing.T, err error, wantField string, wantCode fault.Code) {
	t.Helper()
	if !fault.IsCode(err, execution.CodeCreateInstanceStepShapeInvalid) {
		t.Fatalf("error = %v, want code %s", err, execution.CodeCreateInstanceStepShapeInvalid)
	}
	descriptor, ok := fault.Describe(err)
	if !ok {
		t.Fatalf("error is not a fault: %v", err)
	}
	for _, violation := range descriptor.Violations() {
		if violation.Field() == wantField && violation.Code() == wantCode {
			return
		}
	}
	t.Fatalf("violations = %#v, want %s at %q", descriptor.Violations(), wantCode, wantField)
}

func minimalCompilerPlan() execution.Draft {
	return execution.Draft{RunID: "execution", FailurePolicy: execution.FailurePolicyStopOnFailure, Entries: []execution.WorkflowEntry{{ExecutionID: "execution-entry", TestTaskItemID: "task-item", SequenceNumber: 1, FlowFragmentID: "root", WorkflowVersionID: "root-v1"}}, Workflows: []execution.WorkflowSnapshot{{ID: "root", FlowFragmentID: "root", VersionID: "root-v1", DisplayName: "根流程", VersionNumber: 1, Steps: []execution.Step{{ID: "wait", DisplayName: "等待", Kind: execution.WaitStep, WaitKind: "sleep", WaitMS: 1}}}}}
}

func TestCompilePlanRejectsSnapshotIdentityMatrix(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*execution.Draft)
		wantPlain string
		wantField string
		wantCode  fault.Code
	}{
		{name: "empty workflow version id", mutate: func(plan *execution.Draft) { plan.Workflows[0].VersionID = "" }, wantPlain: "empty version id"},
		{name: "duplicate workflow version", mutate: func(plan *execution.Draft) { plan.Workflows = append(plan.Workflows, plan.Workflows[0]) }, wantPlain: "duplicate workflow version"},
		{name: "duplicate reference resolution", mutate: func(plan *execution.Draft) {
			resolution := execution.ReferenceResolution{ParentVersionID: "root-v1", StepID: "call", FlowFragmentID: "child", WorkflowVersionID: "child-v1"}
			plan.References = []execution.ReferenceResolution{resolution, resolution}
		}, wantPlain: "duplicate workflow resolution"},
		{name: "duplicate node dependency", mutate: func(plan *execution.Draft) {
			dependency := compilerNodeSnapshot(compilerNodeV1, "submit")
			plan.Nodes = []execution.NodeSnapshot{dependency, dependency}
		}, wantPlain: "duplicate node dependency"},
		{name: "missing root version", mutate: func(plan *execution.Draft) { plan.Entries[0].WorkflowVersionID = "missing-v1" }, wantPlain: "entry workflow version"},
		// The workflow's own version-vs-flow-fragment mismatch is caught by
		// WorkflowSnapshot.Validate's step-shape envelope before Draft.Validate
		// ever reaches its own entry/workflow cross-check, so this and the next
		// case surface EXECUTION_CREATE_INSTANCE_STEP_SHAPE_INVALID, not the
		// plan-level code.
		{name: "workflow version has wrong owner", mutate: func(plan *execution.Draft) { plan.Workflows[0].FlowFragmentID = "other" }, wantField: "flowFragmentId", wantCode: fault.CodeFieldInvalid},
		{name: "invalid frozen workflow", mutate: func(plan *execution.Draft) { plan.Workflows[0].Steps = nil }, wantField: "steps", wantCode: fault.CodeFieldRequired},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := minimalCompilerPlan()
			test.mutate(&plan)
			_, err := compileDraft(plan)
			if test.wantField != "" {
				requireEngineStepShapeViolation(t, err, test.wantField, test.wantCode)
				return
			}
			requireEnginePlanRejection(t, err, test.wantPlain)
		})
	}
}

func TestCompilePlanRejectsDuplicateNestedStepIDs(t *testing.T) {
	plan := minimalCompilerPlan()
	plan.Workflows[0].Steps = []execution.Step{
		{ID: "repeat", DisplayName: "重复", Kind: execution.RepeatStep, RepeatCount: 1, Children: []execution.Step{{ID: "duplicate", DisplayName: "一", Kind: execution.WaitStep, WaitKind: "sleep", WaitMS: 1}}},
		{ID: "duplicate", DisplayName: "二", Kind: execution.WaitStep, WaitKind: "sleep", WaitMS: 1},
	}
	_, err := compileDraft(plan)
	requireEngineStepShapeViolation(t, err, "steps", fault.CodeFieldDuplicate)
}

func TestCompilePlanBuildsWaitKinds(t *testing.T) {
	tests := []struct {
		kind         string
		want         node.WaitKind
		requiresNode bool
	}{
		{"element", node.WaitElement, true},
		{"element_visible", node.WaitElementVisible, true},
		{"element_invisible", node.WaitElementInvisible, true},
		{"network_idle", node.WaitNetworkIdle, false},
	}
	for _, test := range tests {
		t.Run(test.kind, func(t *testing.T) {
			plan := minimalCompilerPlan()
			step := execution.Step{ID: "wait", DisplayName: "Wait", Kind: execution.WaitStep, WaitKind: test.kind, WaitMS: 100}
			if test.requiresNode {
				step.ElementTargetID, step.ElementTargetVersionID = compilerNodeID, compilerNodeV1
				plan.Nodes = []execution.NodeSnapshot{compilerNodeSnapshot(compilerNodeV1, "target")}
			}
			plan.Workflows[0].Steps = []execution.Step{step}
			compiled, err := compileDraft(plan)
			if err != nil {
				t.Fatal(err)
			}
			wait := compiled.program.Root.(*node.WorkflowNode).Children[0].(*node.WaitNode)
			if wait.Kind != test.want {
				t.Fatalf("wait kind = %q, want %q", wait.Kind, test.want)
			}
		})
	}
}

func TestCompilePlanBuildsCompleteStepTreeWithoutAliasingPlan(t *testing.T) {
	number, err := parameter.NewNumberValue("01.20")
	if err != nil {
		t.Fatal(err)
	}
	plan := execution.Draft{RunID: "execution", FailurePolicy: execution.FailurePolicyStopOnFailure, Entries: []execution.WorkflowEntry{{ExecutionID: "execution-entry", TestTaskItemID: "task-item", SequenceNumber: 1, FlowFragmentID: "root", WorkflowVersionID: "root-v1"}},
		Workflows: []execution.WorkflowSnapshot{
			{FlowFragmentID: "root", VersionID: "root-v1", DisplayName: "根流程", VersionNumber: 1, Steps: []execution.Step{
				{ID: "select", DisplayName: "选择", Kind: execution.ActionStep, Action: "select", ElementTargetID: compilerNodeID, ElementTargetVersionID: compilerNodeV1, Values: []string{"east", "west"}, Optional: true, CaptureScreenshot: true},
				{ID: "validate", DisplayName: "验证", Kind: execution.ValidationStep, ElementTargetID: compilerNodeID, ElementTargetVersionID: compilerNodeV1, Validation: &execution.Validation{Kind: "text_contains", Expected: "ready", IgnoreCase: true, MaxWaitMS: 2_000, StabilityMS: 200}},
				{ID: "repeat", DisplayName: "重复", Kind: execution.RepeatStep, RepeatCount: 2, Children: []execution.Step{{ID: "network", DisplayName: "网络", Kind: execution.WaitStep, WaitKind: "network_idle", WaitMS: 300}}},
				{ID: "call", DisplayName: "调用", Kind: execution.FlowFragmentReference, Reference: &execution.Reference{FlowFragmentID: "child", WorkflowVersionID: "child-v1", ParameterBindings: map[string]parameter.Binding{"region": parameter.LiteralBinding(parameter.TextValue("north")), "enabled": parameter.LiteralBinding(parameter.BooleanValue(true)), "count": parameter.LiteralBinding(number), "regions": parameter.LiteralBinding(parameter.MultiSelectValue([]string{"north,east", "south"}))}}},
			}},
			{FlowFragmentID: "child", VersionID: "child-v1", DisplayName: "子流程", VersionNumber: 1, Parameters: []execution.Parameter{{Name: "region", DisplayName: "Region", Type: parameter.Text, Required: true}, {Name: "enabled", DisplayName: "Enabled", Type: parameter.Boolean, Required: true}, {Name: "count", DisplayName: "Count", Type: parameter.Number, Required: true}, {Name: "regions", DisplayName: "Regions", Type: parameter.MultiSelect, Required: true, Options: []string{"north,east", "south"}}}, Steps: []execution.Step{{ID: "child-wait", DisplayName: "子等待", Kind: execution.WaitStep, WaitKind: "sleep", WaitMS: 1}}},
		},
		Nodes:      []execution.NodeSnapshot{compilerNodeSnapshot(compilerNodeV1, "region")},
		References: []execution.ReferenceResolution{{ParentVersionID: "root-v1", StepID: "call", FlowFragmentID: "child", WorkflowVersionID: "child-v1"}},
	}
	compiled, err := compileDraft(plan)
	if err != nil {
		t.Fatal(err)
	}
	children := compiled.program.Root.(*node.WorkflowNode).Children
	selectStep, validation := children[0].(*node.StepNode), children[1].(*node.ValidationNode)
	repeat, call := children[2].(*node.RepeatNode), children[3].(*node.WorkflowCallNode)
	regions := literalMultiSelect(call.Bindings["regions"])
	if selectStep.Action.Kind != node.ActionSelect || !selectStep.Optional || len(selectStep.Action.Values) != 2 || validation.MaxWait != 2*time.Second || repeat.Times != 2 || !literalBindingEqual(call.Bindings["region"], parameter.TextValue("north")) || !literalBindingEqual(call.Bindings["enabled"], parameter.BooleanValue(true)) || literalNumber(call.Bindings["count"]) != "1.2" || len(regions) != 2 || regions[0] != "north,east" || regions[1] != "south" {
		t.Fatalf("compiled tree = %#v %#v %#v %#v", selectStep, validation, repeat, call)
	}
	plan.Workflows[0].Steps[0].Values[0] = "mutated"
	plan.Workflows[0].Steps[3].Reference.ParameterBindings["region"] = parameter.LiteralBinding(parameter.TextValue("mutated"))
	plan.Nodes[0].Selectors[0].Value = "mutated"
	if selectStep.Action.Values[0] != "east" || !literalBindingEqual(call.Bindings["region"], parameter.TextValue("north")) || compiled.program.Specs[compilerNodeV1].Selectors[0].Value != "region" {
		t.Fatalf("compiled execution aliases plan: %#v", compiled)
	}
}

func TestCompilePlanPreservesCompleteFingerprint(t *testing.T) {
	plan := minimalCompilerPlan()
	plan.Workflows[0].Steps = []execution.Step{{ID: "click", DisplayName: "点击", Kind: execution.ActionStep, Action: "click", ElementTargetID: compilerNodeID, ElementTargetVersionID: compilerNodeV1}}
	dependency := compilerNodeSnapshot(compilerNodeV1, "submit")
	dependency.PageURL, dependency.Origin = "/checkout", "https://example.test"
	dependency.Fingerprint = fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{"name": "submit"}, Text: "提交", ARIA: fingerprint.ARIA{Role: "button", Name: "提交"}, Path: []string{"html", "body"}, SiblingIndex: 2, Neighbors: fingerprint.Neighbors{Prev: "input", Next: "a", ParentTag: "form"}, LabelText: "提交订单", FormID: "checkout"}
	plan.Nodes = []execution.NodeSnapshot{dependency}
	compiled, err := compileDraft(plan)
	if err != nil {
		t.Fatal(err)
	}
	got := compiled.program.Specs[compilerNodeV1]
	if got.PageURL != dependency.PageURL || got.Origin != dependency.Origin || got.Fingerprint.Text != "提交" || got.Fingerprint.ARIA.Name != "提交" || got.Fingerprint.SiblingIndex != 2 || got.Fingerprint.Neighbors.ParentTag != "form" || got.Fingerprint.LabelText != "提交订单" || got.Fingerprint.FormID != "checkout" {
		t.Fatalf("fingerprint fields were lost: %#v", got)
	}
}
