package execution

import (
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/interpolation"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

func TestPlanFailurePolicyReturnsSealedPolicy(t *testing.T) {
	for _, policy := range []FailurePolicy{FailurePolicyStopOnFailure, FailurePolicyContinueOnFailure} {
		t.Run(string(policy), func(t *testing.T) {
			draft := validDraftWithNodes(validNodeSnapshot("00000000-0000-0000-0000-000000000001", "v1"))
			draft.FailurePolicy = policy
			plan, err := Seal(draft)
			if err != nil {
				t.Fatal(err)
			}
			if got := plan.FailurePolicy(); got != policy {
				t.Fatalf("FailurePolicy() = %q, want %q", got, policy)
			}
		})
	}

	if got := (Plan{}).FailurePolicy(); got != "" {
		t.Fatalf("zero Plan FailurePolicy() = %q, want empty", got)
	}
}

func TestRunSnapshotInvocationFindsAndOwnsValues(t *testing.T) {
	snapshot, err := SealInstanceSnapshot(validRunSnapshotInput(t))
	if err != nil {
		t.Fatal(err)
	}

	invocation, found := snapshot.Invocation("entry-1")
	if !found || invocation.Path != "entry-1" {
		t.Fatalf("Invocation() = %#v, %v", invocation, found)
	}
	invocation.Values["count"] = invocation.Values["regions"]
	again, found := snapshot.Invocation("entry-1")
	if !found || again.Values["count"].Number() != "1.2" {
		t.Fatalf("Invocation() aliases snapshot storage: %#v", again)
	}
	if missing, found := snapshot.Invocation("missing"); found || missing.Path != "" {
		t.Fatalf("missing Invocation() = %#v, %v", missing, found)
	}
}

// TestReferenceValidateConfigurationMatrix exercises WORKFLOW_REF step shape
// checking through the public WorkflowSnapshot.Validate() boundary rather
// than the internal validateReferenceInto directly: the step-shape envelope
// is the only surface a host actually observes.
func TestReferenceValidateConfigurationMatrix(t *testing.T) {
	literal := parameter.LiteralBinding(parameter.TextValue("value"))
	tests := []struct {
		name      string
		reference *Reference
		step      Step
		wantOK    bool
		wantField string
		wantCode  fault.Code
		wantErr   fault.Code
	}{
		{name: "minimal reference", reference: &Reference{FlowFragmentID: "workflow"}, wantOK: true},
		{name: "parent binding is resolved later", reference: &Reference{FlowFragmentID: "workflow", ParameterBindings: map[string]parameter.Binding{"value": parameter.ParentReferenceBinding("parent")}}, wantOK: true},
		{name: "nil reference", reference: nil, wantField: "steps", wantCode: fault.CodeFieldRequired},
		{name: "blank workflow", reference: &Reference{}, wantField: "steps", wantCode: fault.CodeFieldRequired},
		{name: "unsupported residual configuration", reference: &Reference{FlowFragmentID: "workflow"}, step: Step{Action: "click"}, wantField: "steps", wantCode: fault.CodeFieldInvalid},
		{name: "blank binding name", reference: &Reference{FlowFragmentID: "workflow", ParameterBindings: map[string]parameter.Binding{" ": literal}}, wantField: "steps", wantCode: fault.CodeFieldInvalid},
		{name: "invalid binding kind", reference: &Reference{FlowFragmentID: "workflow", ParameterBindings: map[string]parameter.Binding{"value": {}}}, wantErr: parameter.CodeBindingInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workflow := validWorkflowSnapshot()
			workflow.Steps = []Step{mergeReferenceStep(test.step, "call", test.reference)}
			err := workflow.Validate()
			switch {
			case test.wantOK:
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
			case test.wantErr != "":
				if !fault.IsCode(err, test.wantErr) {
					t.Fatalf("Validate() error = %v, want code %s", err, test.wantErr)
				}
			default:
				requireStepShapeViolation(t, err, test.wantField, test.wantCode)
			}
		})
	}
}

func mergeReferenceStep(base Step, id string, reference *Reference) Step {
	base.ID, base.DisplayName, base.Kind = id, id, FlowFragmentReference
	base.Reference = reference
	return base
}

// TestValidationValidateBoundaryAndKindMatrix exercises Validation.Validate
// directly: it is an internal shape check reached either through the
// step-shape envelope (which discards its detail into a generic violation)
// or, for the two interpolation cases, an already-classified fault that a
// caller can match by code. It never echoes Kind, Expected, or Attribute, so
// wantError checks static wording only, never an interpolated value.
func TestValidationValidateBoundaryAndKindMatrix(t *testing.T) {
	tests := []struct {
		name      string
		value     Validation
		wait      bool
		wantError string
		wantCode  fault.Code
	}{
		{name: "boolean kind", value: Validation{Kind: "visible"}},
		{name: "scalar kind", value: Validation{Kind: "text_equals", Expected: "ready", IgnoreCase: true}},
		{name: "regex kind", value: Validation{Kind: "text_matches", Expected: "^ready$"}},
		{name: "templated regex", value: Validation{Kind: "text_matches", Expected: "${pattern}"}},
		{name: "set kind", value: Validation{Kind: "selected_set_equals", ExpectedValues: []string{"east", "west"}}},
		{name: "attribute kind", value: Validation{Kind: "attribute_equals", Attribute: "role", Expected: "button"}},
		{name: "wait at lower boundaries", value: Validation{Kind: "visible", MaxWaitMS: validationMinWaitMS, StabilityMS: validationMinStabilityMS}, wait: true},
		{name: "wait at upper boundaries", value: Validation{Kind: "visible", MaxWaitMS: validationMaxWaitMS, StabilityMS: validationMaxStabilityMS}, wait: true},
		{name: "invalid scalar interpolation", value: Validation{Kind: "text_equals", Expected: "${broken"}, wantCode: interpolation.CodeExpressionInvalid},
		{name: "invalid collection interpolation", value: Validation{Kind: "selected_set_equals", ExpectedValues: []string{"${broken"}}, wantCode: interpolation.CodeExpressionInvalid},
		{name: "boolean comparison options", value: Validation{Kind: "visible", IgnoreCase: true}, wantError: "does not accept"},
		{name: "scalar collection option", value: Validation{Kind: "text_equals", ExpectedValues: []string{"ready"}}, wantError: "one scalar"},
		{name: "regex ignore case", value: Validation{Kind: "text_matches", Expected: "ready", IgnoreCase: true}, wantError: "regular expression"},
		{name: "invalid regular expression", value: Validation{Kind: "text_matches", Expected: "["}, wantError: "invalid regular expression"},
		{name: "set scalar option", value: Validation{Kind: "selected_set_equals", Expected: "east"}, wantError: "only expected values"},
		{name: "attribute missing name", value: Validation{Kind: "attribute_equals"}, wantError: "attribute name"},
		{name: "attribute collection option", value: Validation{Kind: "attribute_equals", Attribute: "role", ExpectedValues: []string{"button"}}, wantError: "one scalar"},
		{name: "attribute variable", value: Validation{Kind: "attribute_equals", Attribute: "${name}"}, wantError: "does not accept variable"},
		{name: "unsupported kind", value: Validation{Kind: "UNKNOWN"}, wantError: "unsupported validation kind"},
		{name: "maximum wait below boundary", value: Validation{Kind: "visible", MaxWaitMS: validationMinWaitMS - 1, StabilityMS: validationMinStabilityMS}, wait: true, wantError: "maximum wait"},
		{name: "maximum wait above boundary", value: Validation{Kind: "visible", MaxWaitMS: validationMaxWaitMS + 1, StabilityMS: validationMinStabilityMS}, wait: true, wantError: "maximum wait"},
		{name: "stability below boundary", value: Validation{Kind: "visible", MaxWaitMS: validationMinWaitMS, StabilityMS: validationMinStabilityMS - 1}, wait: true, wantError: "stability window"},
		{name: "stability above boundary", value: Validation{Kind: "visible", MaxWaitMS: validationMaxWaitMS, StabilityMS: validationMaxStabilityMS + 1}, wait: true, wantError: "stability window"},
		{name: "stability equals maximum wait", value: Validation{Kind: "visible", MaxWaitMS: validationMinWaitMS, StabilityMS: validationMinWaitMS}, wait: true, wantError: "shorter"},
		{name: "group member declares wait", value: Validation{Kind: "visible", MaxWaitMS: validationMinWaitMS}, wantError: "inherit the group wait"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.value.Validate(test.wait)
			if test.wantCode != "" {
				if !fault.IsCode(err, test.wantCode) {
					t.Fatalf("Validate() error = %v, want code %s", err, test.wantCode)
				}
				return
			}
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.wantError)
			}
		})
	}
}

func validValidationGroupContract() (Step, *ValidationGroup) {
	member := Step{
		ID: "member", DisplayName: "Member", Kind: ValidationStep,
		ElementTargetID: "node", ElementTargetVersionID: "node-v1", Validation: &Validation{Kind: "visible"},
	}
	return Step{ID: "group", DisplayName: "Group", Kind: ValidationGroupStep}, &ValidationGroup{
		MaxWaitMS: validationMinWaitMS, StabilityMS: validationMinStabilityMS,
		Branches: []ValidationBranch{{ID: "branch", Name: "Branch", Steps: []Step{member}}},
	}
}

// TestValidationGroupValidateScenarioMatrix exercises VALIDATION_GROUP step
// shape checking through the public WorkflowSnapshot.Validate() boundary. The
// "member duplicate outside group" case adds a sibling root step with the
// colliding id instead of pre-seeding an internal seen-set, because that set
// is no longer exposed outside the step-shape walk.
func TestValidationGroupValidateScenarioMatrix(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*Step, **ValidationGroup)
		extraStep *Step
		wantOK    bool
		wantField string
		wantCode  fault.Code
	}{
		{name: "valid group", wantOK: true},
		{name: "nil group", mutate: func(_ *Step, group **ValidationGroup) { *group = nil }, wantField: "steps", wantCode: fault.CodeFieldRequired},
		{name: "unsupported group step field", mutate: func(step *Step, _ **ValidationGroup) { step.Action = "click" }, wantField: "steps", wantCode: fault.CodeFieldInvalid},
		{name: "invalid group wait", mutate: func(_ *Step, group **ValidationGroup) {
			(*group).MaxWaitMS = validationMinWaitMS - 1
		}, wantField: "steps", wantCode: fault.CodeFieldInvalid},
		{name: "missing branches", mutate: func(_ *Step, group **ValidationGroup) { (*group).Branches = nil }, wantField: "steps", wantCode: fault.CodeFieldInvalid},
		{name: "too many branches", mutate: func(_ *Step, group **ValidationGroup) {
			branch := (*group).Branches[0]
			(*group).Branches = make([]ValidationBranch, validationMaxBranches+1)
			for index := range (*group).Branches {
				copy := branch
				copy.ID = string(rune('a' + index))
				copy.Steps = append([]Step(nil), branch.Steps...)
				copy.Steps[0].ID = "member-" + copy.ID
				(*group).Branches[index] = copy
			}
		}, wantField: "steps", wantCode: fault.CodeFieldInvalid},
		{name: "blank branch identity", mutate: func(_ *Step, group **ValidationGroup) { (*group).Branches[0].ID = " " }, wantField: "steps", wantCode: fault.CodeFieldRequired},
		{name: "duplicate branch identity", mutate: func(_ *Step, group **ValidationGroup) {
			copy := (*group).Branches[0]
			copy.Steps = append([]Step(nil), copy.Steps...)
			copy.Steps[0].ID = "member-2"
			(*group).Branches = append((*group).Branches, copy)
		}, wantField: "steps", wantCode: fault.CodeFieldDuplicate},
		{name: "branch without members", mutate: func(_ *Step, group **ValidationGroup) { (*group).Branches[0].Steps = nil }, wantField: "steps", wantCode: fault.CodeFieldInvalid},
		{name: "member duplicate outside group", extraStep: &Step{ID: "member", DisplayName: "Outer", Kind: ActionStep, Action: "noop", ElementTargetID: "node", ElementTargetVersionID: "node-v1"}, wantField: "steps", wantCode: fault.CodeFieldDuplicate},
		{name: "blank member identity", mutate: func(_ *Step, group **ValidationGroup) { (*group).Branches[0].Steps[0].ID = "" }, wantField: "steps", wantCode: fault.CodeFieldRequired},
		{name: "non-validation member", mutate: func(_ *Step, group **ValidationGroup) {
			(*group).Branches[0].Steps[0].Kind = WaitStep
		}, wantField: "steps", wantCode: fault.CodeFieldInvalid},
		{name: "invalid validation member", mutate: func(_ *Step, group **ValidationGroup) {
			(*group).Branches[0].Steps[0].ElementTargetID = ""
		}, wantField: "steps", wantCode: fault.CodeFieldRequired},
		{name: "aggregate members above boundary", mutate: func(_ *Step, group **ValidationGroup) {
			template := (*group).Branches[0].Steps[0]
			(*group).Branches = make([]ValidationBranch, validationMaxBranches)
			for branchIndex := range (*group).Branches {
				branch := ValidationBranch{ID: string(rune('a' + branchIndex)), Name: "Branch"}
				for memberIndex := 0; memberIndex < validationMaxGroupSteps/validationMaxBranches+1; memberIndex++ {
					member := template
					member.ID = branch.ID + string(rune('a'+memberIndex))
					branch.Steps = append(branch.Steps, member)
				}
				(*group).Branches[branchIndex] = branch
			}
		}, wantField: "steps", wantCode: fault.CodeFieldInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			step, group := validValidationGroupContract()
			step.ValidationGroup = group
			if test.mutate != nil {
				test.mutate(&step, &group)
				step.ValidationGroup = group
			}
			workflow := validWorkflowSnapshot()
			var steps []Step
			if test.extraStep != nil {
				steps = append(steps, *test.extraStep)
			}
			steps = append(steps, step)
			workflow.Steps = steps
			err := workflow.Validate()
			if test.wantOK {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			requireStepShapeViolation(t, err, test.wantField, test.wantCode)
		})
	}
}
