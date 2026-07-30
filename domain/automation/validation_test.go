package automation

import (
	"fmt"
	"strings"
	"testing"
)

func TestWorkflowAggregateValidateAcceptsStandaloneValidation(t *testing.T) {
	aggregate := validationWorkflow(FlowFragmentStep{
		ID: "validation-status", DisplayName: "验证订单状态", Kind: StepValidation,
		NodeID: "node-status", NodeVersionID: "node-version-status",
		Validation: &ValidationConfig{
			Assertion: ValidationAssertion{Kind: ValidationTextEquals, Expected: "成功"},
			Wait:      ValidationWait{MaxWaitMS: 10_000, StabilityMS: 500},
		},
	})
	if err := aggregate.Validate(); err != nil {
		t.Fatalf("standalone validation rejected: %v", err)
	}
}

func TestValidationAssertionNormalizedClearsIncompatibleComparisonFields(t *testing.T) {
	visible := (ValidationAssertion{Kind: ValidationVisible, Expected: "true", ExpectedValues: []string{"true"}, Attribute: "aria-hidden", IgnoreCase: true}).Normalized()
	if err := visible.Validate(); err != nil {
		t.Fatalf("normalized visible assertion rejected: %v", err)
	}
	if visible.Expected != "" || len(visible.ExpectedValues) != 0 || visible.Attribute != "" || visible.IgnoreCase {
		t.Fatalf("visible comparison fields were retained: %#v", visible)
	}

	attribute := (ValidationAssertion{Kind: ValidationAttributeEquals, Expected: "ready", ExpectedValues: []string{"stale"}, Attribute: "data-state", IgnoreCase: true}).Normalized()
	if err := attribute.Validate(); err != nil {
		t.Fatalf("normalized attribute assertion rejected: %v", err)
	}
	if len(attribute.ExpectedValues) != 0 || attribute.Expected != "ready" || attribute.Attribute != "data-state" || !attribute.IgnoreCase {
		t.Fatalf("attribute fields were not preserved correctly: %#v", attribute)
	}
}

func TestWorkflowAggregateValidateAcceptsValidationGroup(t *testing.T) {
	aggregate := validationWorkflow(FlowFragmentStep{
		ID: "validation-group", DisplayName: "订单结果", Kind: StepValidationGroup,
		ValidationGroup: &ValidationGroup{
			Wait: ValidationWait{MaxWaitMS: 10_000, StabilityMS: 500},
			Branches: []ValidationBranch{
				{ID: "success", Name: "成功", Steps: []FlowFragmentStep{validationMember("status-success", "状态为成功")}},
				{ID: "pending", Name: "处理中", Steps: []FlowFragmentStep{validationMember("status-pending", "状态为处理中")}},
			},
		},
	})
	if err := aggregate.Validate(); err != nil {
		t.Fatalf("validation group rejected: %v", err)
	}
}

func TestWorkflowAggregateValidateRejectsInvalidValidationConfiguration(t *testing.T) {
	valid := FlowFragmentStep{
		ID: "validation", DisplayName: "验证", Kind: StepValidation,
		NodeID: "node", NodeVersionID: "node-version",
		Validation: &ValidationConfig{
			Assertion: ValidationAssertion{Kind: ValidationTextEquals, Expected: "成功"},
			Wait:      ValidationWait{MaxWaitMS: 10_000, StabilityMS: 500},
		},
	}
	cases := []struct {
		name string
		step FlowFragmentStep
		want string
	}{
		{name: "missing one assertion config", step: func() FlowFragmentStep { s := valid; s.Validation = nil; return s }(), want: "requires validation configuration"},
		{name: "missing exact node version", step: func() FlowFragmentStep { s := valid; s.NodeVersionID = ""; return s }(), want: "exact node reference"},
		{name: "invalid wait", step: func() FlowFragmentStep {
			s := valid
			s.Validation = &ValidationConfig{Assertion: s.Validation.Assertion, Wait: ValidationWait{MaxWaitMS: 999, StabilityMS: 200}}
			return s
		}(), want: "maximum wait"},
		{name: "invalid regex", step: func() FlowFragmentStep {
			s := valid
			s.Validation = &ValidationConfig{Assertion: ValidationAssertion{Kind: ValidationTextMatches, Expected: "["}, Wait: s.Validation.Wait}
			return s
		}(), want: "invalid regular expression"},
		{name: "nested in repeat", step: FlowFragmentStep{ID: "repeat", DisplayName: "循环", Kind: StepRepeat, RepeatCount: 1, Children: []FlowFragmentStep{valid}}, want: "must be a root step"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validationWorkflow(tc.step).Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestWorkflowAggregateValidateRejectsInvalidValidationGroup(t *testing.T) {
	validGroup := func() FlowFragmentStep {
		return FlowFragmentStep{ID: "group", DisplayName: "验证组", Kind: StepValidationGroup,
			ValidationGroup: &ValidationGroup{Wait: ValidationWait{MaxWaitMS: 10_000, StabilityMS: 500},
				Branches: []ValidationBranch{{ID: "branch-a", Name: "分支 A", Steps: []FlowFragmentStep{validationMember("member-a", "条件 A")}}}}}
	}
	cases := []struct {
		name string
		step FlowFragmentStep
		want string
	}{
		{name: "empty branch", step: func() FlowFragmentStep { s := validGroup(); s.ValidationGroup.Branches[0].Steps = nil; return s }(), want: "requires 1-10 validation steps"},
		{name: "member action", step: func() FlowFragmentStep {
			s := validGroup()
			s.ValidationGroup.Branches[0].Steps[0].Kind = StepAction
			return s
		}(), want: "only accepts VALIDATION"},
		{name: "member wait", step: func() FlowFragmentStep {
			s := validGroup()
			s.ValidationGroup.Branches[0].Steps[0].Validation.Wait = ValidationWait{MaxWaitMS: 10_000, StabilityMS: 500}
			return s
		}(), want: "must inherit the group wait"},
		{name: "nested group", step: func() FlowFragmentStep {
			s := validGroup()
			s.ValidationGroup.Branches[0].Steps[0].ValidationGroup = &ValidationGroup{}
			return s
		}(), want: "contains unsupported action or child configuration"},
		{name: "too many branches", step: func() FlowFragmentStep {
			s := validGroup()
			for index := 0; index < 5; index++ {
				s.ValidationGroup.Branches = append(s.ValidationGroup.Branches, ValidationBranch{ID: "extra-" + string(rune('a'+index)), Name: "额外", Steps: []FlowFragmentStep{validationMember("extra-member-"+string(rune('a'+index)), "额外条件")}})
			}
			return s
		}(), want: "requires 1-5 branches"},
		{name: "too many total members", step: func() FlowFragmentStep {
			s := validGroup()
			s.ValidationGroup.Branches = nil
			for branch := 0; branch < 3; branch++ {
				members := make([]FlowFragmentStep, 0, 10)
				for member := 0; member < 10; member++ {
					members = append(members, validationMember(string(rune('a'+branch))+string(rune('a'+member)), "条件"))
				}
				s.ValidationGroup.Branches = append(s.ValidationGroup.Branches, ValidationBranch{ID: "branch-" + string(rune('a'+branch)), Name: "分支", Steps: members})
			}
			return s
		}(), want: "maximum is 20"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validationWorkflow(tc.step).Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestWorkflowAggregateValidateRejectsOversizedStepTrees(t *testing.T) {
	t.Run("depth", func(t *testing.T) {
		step := FlowFragmentStep{ID: "leaf", DisplayName: "叶子", Kind: StepAction, Action: "noop"}
		for depth := 0; depth <= maxWorkflowStepDepth; depth++ {
			step = FlowFragmentStep{
				ID:          fmt.Sprintf("repeat-%d", depth),
				DisplayName: "循环",
				Kind:        StepRepeat,
				RepeatCount: 1,
				Children:    []FlowFragmentStep{step},
			}
		}
		err := validationWorkflow(step).Validate()
		if err == nil || !strings.Contains(err.Error(), "maximum nesting depth") {
			t.Fatalf("Validate() error = %v, want depth limit", err)
		}
	})

	t.Run("count", func(t *testing.T) {
		steps := make([]FlowFragmentStep, maxWorkflowStepCount+1)
		for index := range steps {
			steps[index] = FlowFragmentStep{ID: fmt.Sprintf("step-%d", index), DisplayName: "步骤", Kind: StepAction, Action: "noop"}
		}
		aggregate := validationWorkflow(steps[0])
		aggregate.Current.Definition.Steps = steps
		err := aggregate.Validate()
		if err == nil || !strings.Contains(err.Error(), "maximum step count") {
			t.Fatalf("Validate() error = %v, want step count limit", err)
		}
	})
}

func validationWorkflow(step FlowFragmentStep) FlowFragmentAggregate {
	return FlowFragmentAggregate{
		FlowFragment: FlowFragment{ID: "workflow", DisplayName: "验证流程", Properties: Properties{}, CurrentVersionID: "workflow-version"},
		Current: FlowFragmentVersion{ID: "workflow-version", FlowFragmentID: "workflow", VersionNumber: 1,
			Definition: FlowFragmentContent{Steps: []FlowFragmentStep{step}}},
	}
}

func validationMember(id, name string) FlowFragmentStep {
	return FlowFragmentStep{ID: id, DisplayName: name, Kind: StepValidation,
		NodeID: "node-" + id, NodeVersionID: "version-" + id,
		Validation: &ValidationConfig{Assertion: ValidationAssertion{Kind: ValidationVisible}}}
}
