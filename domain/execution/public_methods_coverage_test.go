package execution

import (
	"strings"
	"testing"

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
	snapshot, err := SealRunSnapshot(validRunSnapshotInput(t))
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

func TestReferenceValidateConfigurationMatrix(t *testing.T) {
	literal := parameter.LiteralBinding(parameter.TextValue("value"))
	tests := []struct {
		name      string
		reference *Reference
		step      Step
		want      string
	}{
		{name: "minimal reference", reference: &Reference{FlowFragmentID: "workflow"}, step: Step{DisplayName: "call"}},
		{name: "parent binding is resolved later", reference: &Reference{FlowFragmentID: "workflow", ParameterBindings: map[string]parameter.Binding{"value": parameter.ParentReferenceBinding("parent")}}, step: Step{DisplayName: "call"}},
		{name: "nil reference", step: Step{DisplayName: "call"}, want: "requires a workflow reference"},
		{name: "blank workflow", reference: &Reference{}, step: Step{DisplayName: "call"}, want: "requires a workflow reference"},
		{name: "unsupported residual configuration", reference: &Reference{FlowFragmentID: "workflow"}, step: Step{DisplayName: "call", Action: "click"}, want: "unsupported step configuration"},
		{name: "blank binding name", reference: &Reference{FlowFragmentID: "workflow", ParameterBindings: map[string]parameter.Binding{" ": literal}}, step: Step{DisplayName: "call"}, want: "empty parameter binding"},
		{name: "invalid binding kind", reference: &Reference{FlowFragmentID: "workflow", ParameterBindings: map[string]parameter.Binding{"value": {}}}, step: Step{DisplayName: "call"}, want: string(parameter.CodeBindingUnresolvable)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			problems := test.reference.Validate(test.step)
			joined := strings.Join(problems, "; ")
			if test.want == "" && len(problems) != 0 {
				t.Fatalf("Validate() problems = %v", problems)
			}
			if test.want != "" && !strings.Contains(joined, test.want) {
				t.Fatalf("Validate() problems = %v, want containing %q", problems, test.want)
			}
		})
	}
}

func TestValidationValidateBoundaryAndKindMatrix(t *testing.T) {
	tests := []struct {
		name      string
		value     Validation
		wait      bool
		wantError string
	}{
		{name: "boolean kind", value: Validation{Kind: "visible"}},
		{name: "scalar kind", value: Validation{Kind: "text_equals", Expected: "ready", IgnoreCase: true}},
		{name: "regex kind", value: Validation{Kind: "text_matches", Expected: "^ready$"}},
		{name: "templated regex", value: Validation{Kind: "text_matches", Expected: "${pattern}"}},
		{name: "set kind", value: Validation{Kind: "selected_set_equals", ExpectedValues: []string{"east", "west"}}},
		{name: "attribute kind", value: Validation{Kind: "attribute_equals", Attribute: "role", Expected: "button"}},
		{name: "wait at lower boundaries", value: Validation{Kind: "visible", MaxWaitMS: validationMinWaitMS, StabilityMS: validationMinStabilityMS}, wait: true},
		{name: "wait at upper boundaries", value: Validation{Kind: "visible", MaxWaitMS: validationMaxWaitMS, StabilityMS: validationMaxStabilityMS}, wait: true},
		{name: "invalid scalar interpolation", value: Validation{Kind: "text_equals", Expected: "${broken"}, wantError: "expected value"},
		{name: "invalid collection interpolation", value: Validation{Kind: "selected_set_equals", ExpectedValues: []string{"${broken"}}, wantError: "expected value"},
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

func validValidationGroupContract() (Step, *ValidationGroup, map[string]struct{}) {
	member := Step{
		ID: "member", DisplayName: "Member", Kind: ValidationStep,
		ElementTargetID: "node", ElementTargetVersionID: "node-v1", Validation: &Validation{Kind: "visible"},
	}
	return Step{ID: "group", DisplayName: "Group", Kind: ValidationGroupStep}, &ValidationGroup{
		MaxWaitMS: validationMinWaitMS, StabilityMS: validationMinStabilityMS,
		Branches: []ValidationBranch{{ID: "branch", Name: "Branch", Steps: []Step{member}}},
	}, map[string]struct{}{}
}

func TestValidationGroupValidateScenarioMatrix(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Step, **ValidationGroup, map[string]struct{})
		want   string
	}{
		{name: "valid group"},
		{name: "nil group", mutate: func(_ *Step, group **ValidationGroup, _ map[string]struct{}) { *group = nil }, want: "requires group configuration"},
		{name: "unsupported group step field", mutate: func(step *Step, _ **ValidationGroup, _ map[string]struct{}) { step.Action = "click" }, want: "unsupported step configuration"},
		{name: "invalid group wait", mutate: func(_ *Step, group **ValidationGroup, _ map[string]struct{}) {
			(*group).MaxWaitMS = validationMinWaitMS - 1
		}, want: "group \"Group\" wait"},
		{name: "missing branches", mutate: func(_ *Step, group **ValidationGroup, _ map[string]struct{}) { (*group).Branches = nil }, want: "requires 1-5 branches"},
		{name: "too many branches", mutate: func(_ *Step, group **ValidationGroup, _ map[string]struct{}) {
			branch := (*group).Branches[0]
			(*group).Branches = make([]ValidationBranch, validationMaxBranches+1)
			for index := range (*group).Branches {
				copy := branch
				copy.ID = string(rune('a' + index))
				copy.Steps = append([]Step(nil), branch.Steps...)
				copy.Steps[0].ID = "member-" + copy.ID
				(*group).Branches[index] = copy
			}
		}, want: "requires 1-5 branches"},
		{name: "blank branch identity", mutate: func(_ *Step, group **ValidationGroup, _ map[string]struct{}) { (*group).Branches[0].ID = " " }, want: "branch id and name are required"},
		{name: "duplicate branch identity", mutate: func(_ *Step, group **ValidationGroup, _ map[string]struct{}) {
			copy := (*group).Branches[0]
			copy.Steps = append([]Step(nil), copy.Steps...)
			copy.Steps[0].ID = "member-2"
			(*group).Branches = append((*group).Branches, copy)
		}, want: "duplicate branch id"},
		{name: "branch without members", mutate: func(_ *Step, group **ValidationGroup, _ map[string]struct{}) { (*group).Branches[0].Steps = nil }, want: "requires 1-10 validation steps"},
		{name: "member duplicate outside group", mutate: func(_ *Step, _ **ValidationGroup, seen map[string]struct{}) { seen["member"] = struct{}{} }, want: "duplicate step id"},
		{name: "blank member identity", mutate: func(_ *Step, group **ValidationGroup, _ map[string]struct{}) { (*group).Branches[0].Steps[0].ID = "" }, want: "member step id and display name are required"},
		{name: "non-validation member", mutate: func(_ *Step, group **ValidationGroup, _ map[string]struct{}) {
			(*group).Branches[0].Steps[0].Kind = WaitStep
		}, want: "only accepts VALIDATION steps"},
		{name: "invalid validation member", mutate: func(_ *Step, group **ValidationGroup, _ map[string]struct{}) {
			(*group).Branches[0].Steps[0].ElementTargetID = ""
		}, want: "requires an exact node reference"},
		{name: "aggregate members above boundary", mutate: func(_ *Step, group **ValidationGroup, _ map[string]struct{}) {
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
		}, want: "maximum is 20"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			step, group, seen := validValidationGroupContract()
			if test.mutate != nil {
				test.mutate(&step, &group, seen)
			}
			problems := group.Validate(step, seen)
			joined := strings.Join(problems, "; ")
			if test.want == "" && len(problems) != 0 {
				t.Fatalf("Validate() problems = %v", problems)
			}
			if test.want != "" && !strings.Contains(joined, test.want) {
				t.Fatalf("Validate() problems = %v, want containing %q", problems, test.want)
			}
		})
	}
}
