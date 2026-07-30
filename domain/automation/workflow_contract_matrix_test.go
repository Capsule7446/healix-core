package automation

import (
	"github.com/Capsule7446/healix-core/domain/parameter"
	"strings"
	"testing"
)

func TestWorkflowAggregateValidateStepKindBusinessMatrix(t *testing.T) {
	tests := []struct {
		name string
		step FlowFragmentStep
	}{
		{name: "click", step: workflowActionStep("click", "")},
		{name: "input allows empty text", step: workflowActionStep("input", "")},
		{name: "single select value", step: workflowActionStep("select", "east")},
		{name: "multi select values", step: func() FlowFragmentStep {
			step := workflowActionStep("select", "")
			step.Values = []string{"east", "west"}
			return step
		}()},
		{name: "hover", step: workflowActionStep("hover", "")},
		{name: "navigate without node", step: FlowFragmentStep{ID: "navigate", DisplayName: "打开页面", Kind: StepAction, Action: "navigate", Value: "https://example.test"}},
		{name: "press without node", step: FlowFragmentStep{ID: "press", DisplayName: "按键", Kind: StepAction, Action: "press", Value: "Enter"}},
		{name: "noop", step: workflowActionStep("noop", "")},
		{name: "extract", step: workflowActionStep("extract", "order_id")},
		{name: "fixed sleep", step: FlowFragmentStep{ID: "sleep", DisplayName: "等待", Kind: StepWait, WaitKind: "sleep", WaitMS: 1}},
		{name: "empty wait kind means fixed sleep", step: FlowFragmentStep{ID: "sleep", DisplayName: "等待", Kind: StepWait, WaitMS: 1}},
		{name: "element wait allows adapter default timeout", step: FlowFragmentStep{ID: "element", DisplayName: "等待元素", Kind: StepWait, WaitKind: "element", ElementTargetID: "node", ElementTargetVersionID: "node-v1"}},
		{name: "network idle allows adapter default timeout", step: FlowFragmentStep{ID: "network", DisplayName: "等待网络", Kind: StepWait, WaitKind: "network_idle"}},
		{name: "visible element wait", step: FlowFragmentStep{ID: "visible", DisplayName: "等待可见元素", Kind: StepWait, WaitKind: "element_visible", ElementTargetID: "node", ElementTargetVersionID: "node-v1"}},
		{name: "invisible element wait", step: FlowFragmentStep{ID: "invisible", DisplayName: "等待不可见元素", Kind: StepWait, WaitKind: "element_invisible", ElementTargetID: "node", ElementTargetVersionID: "node-v1"}},
		{name: "repeat", step: FlowFragmentStep{ID: "repeat", DisplayName: "循环", Kind: StepRepeat, RepeatCount: 2, Children: []FlowFragmentStep{{ID: "nested-wait", DisplayName: "等待", Kind: StepWait, WaitMS: 1}}}},
		{name: "fixed workflow reference", step: FlowFragmentStep{ID: "fixed", DisplayName: "固定子流程", Kind: StepFlowFragmentRef, Reference: &FlowFragmentReference{FlowFragmentID: "child", WorkflowVersionID: "child-v1"}}},
		{name: "latest workflow reference", step: FlowFragmentStep{ID: "latest", DisplayName: "最新子流程", Kind: StepFlowFragmentRef, Reference: &FlowFragmentReference{FlowFragmentID: "child", LatestPublished: true}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := workflowWithSteps(test.step).Validate(); err != nil {
				t.Fatalf("valid %s step rejected: %v", test.step.Kind, err)
			}
		})
	}
}

func TestWorkflowAggregateValidateRejectsStepKindConstraintMatrix(t *testing.T) {
	tests := []struct {
		name string
		step FlowFragmentStep
		want string
	}{
		{name: "unsupported action", step: workflowActionStep("double_click", ""), want: "unsupported action"},
		{name: "click needs node", step: FlowFragmentStep{ID: "click", DisplayName: "点击", Kind: StepAction, Action: "click"}, want: "requires a node"},
		{name: "node needs exact version", step: func() FlowFragmentStep {
			step := workflowActionStep("click", "")
			step.ElementTargetVersionID = ""
			return step
		}(), want: "exact node version"},
		{name: "navigate needs URL value", step: FlowFragmentStep{ID: "navigate", DisplayName: "打开", Kind: StepAction, Action: "navigate"}, want: "requires a value"},
		{name: "press needs key value", step: FlowFragmentStep{ID: "press", DisplayName: "按键", Kind: StepAction, Action: "press"}, want: "requires a value"},
		{name: "extract needs scratchpad name", step: workflowActionStep("extract", ""), want: "requires a value"},
		{name: "select needs one value", step: workflowActionStep("select", ""), want: "at least one value"},
		{name: "fixed wait must be positive", step: FlowFragmentStep{ID: "wait", DisplayName: "等待", Kind: StepWait, WaitMS: 0}, want: "fixed wait must be > 0"},
		{name: "element wait needs node", step: FlowFragmentStep{ID: "wait", DisplayName: "等待元素", Kind: StepWait, WaitKind: "element"}, want: "requires a node"},
		{name: "element wait needs exact version", step: FlowFragmentStep{ID: "wait", DisplayName: "等待元素", Kind: StepWait, WaitKind: "element", ElementTargetID: "node"}, want: "exact node version"},
		{name: "element wait rejects negative timeout", step: FlowFragmentStep{ID: "wait", DisplayName: "等待元素", Kind: StepWait, WaitKind: "element", ElementTargetID: "node", ElementTargetVersionID: "node-v1", WaitMS: -1}, want: "timeout must be >= 0"},
		{name: "network wait rejects negative timeout", step: FlowFragmentStep{ID: "wait", DisplayName: "等待网络", Kind: StepWait, WaitKind: "network_idle", WaitMS: -1}, want: "timeout must be >= 0"},
		{name: "unknown wait kind", step: FlowFragmentStep{ID: "wait", DisplayName: "等待事件", Kind: StepWait, WaitKind: "event"}, want: "unsupported wait kind"},
		{name: "repeat needs positive count", step: FlowFragmentStep{ID: "repeat", DisplayName: "循环", Kind: StepRepeat, Children: []FlowFragmentStep{{ID: "child", DisplayName: "等待", Kind: StepWait, WaitMS: 1}}}, want: "requires count and children"},
		{name: "repeat needs children", step: FlowFragmentStep{ID: "repeat", DisplayName: "循环", Kind: StepRepeat, RepeatCount: 1}, want: "requires count and children"},
		{name: "reference needs target", step: FlowFragmentStep{ID: "ref", DisplayName: "子流程", Kind: StepFlowFragmentRef}, want: "requires a workflow reference"},
		{name: "latest reference cannot pin version", step: FlowFragmentStep{ID: "ref", DisplayName: "子流程", Kind: StepFlowFragmentRef, Reference: &FlowFragmentReference{FlowFragmentID: "child", WorkflowVersionID: "child-v1", LatestPublished: true}}, want: "cannot persist a version"},
		{name: "fixed reference needs version", step: FlowFragmentStep{ID: "ref", DisplayName: "子流程", Kind: StepFlowFragmentRef, Reference: &FlowFragmentReference{FlowFragmentID: "child"}}, want: "requires a version"},
		{name: "only action can be optional", step: FlowFragmentStep{ID: "wait", DisplayName: "等待", Kind: StepWait, WaitMS: 1, Optional: true}, want: "only ACTION can be optional"},
		{name: "unknown kind", step: FlowFragmentStep{ID: "unknown", DisplayName: "未知", Kind: StepKind("UNKNOWN")}, want: "unsupported kind"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := workflowWithSteps(test.step).Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestWorkflowAggregateValidateRejectsDiscriminatedUnionResidualFields(t *testing.T) {
	tests := []struct {
		name string
		step FlowFragmentStep
		want string
	}{
		{name: "action cannot carry workflow reference", step: func() FlowFragmentStep {
			step := workflowActionStep("click", "")
			step.Reference = &FlowFragmentReference{FlowFragmentID: "child", WorkflowVersionID: "child-v1"}
			return step
		}(), want: "ACTION contains unsupported step configuration"},
		{name: "action cannot carry children", step: func() FlowFragmentStep {
			step := workflowActionStep("click", "")
			step.Children = []FlowFragmentStep{{ID: "child", DisplayName: "等待", Kind: StepWait, WaitMS: 1}}
			return step
		}(), want: "ACTION contains unsupported step configuration"},
		{name: "wait cannot carry action fields", step: FlowFragmentStep{ID: "wait", DisplayName: "等待", Kind: StepWait, WaitMS: 1, Action: "navigate", Value: "https://example.test"}, want: "WAIT contains unsupported step configuration"},
		{name: "fixed wait cannot carry node", step: FlowFragmentStep{ID: "wait", DisplayName: "等待", Kind: StepWait, WaitMS: 1, ElementTargetID: "node", ElementTargetVersionID: "node-v1"}, want: "WAIT contains unsupported step configuration"},
		{name: "repeat cannot carry node", step: FlowFragmentStep{ID: "repeat", DisplayName: "循环", Kind: StepRepeat, RepeatCount: 1,
			ElementTargetID: "node", ElementTargetVersionID: "node-v1", Children: []FlowFragmentStep{{ID: "child", DisplayName: "等待", Kind: StepWait, WaitMS: 1}}}, want: "REPEAT contains unsupported step configuration"},
		{name: "repeat cannot carry reference", step: FlowFragmentStep{ID: "repeat", DisplayName: "循环", Kind: StepRepeat, RepeatCount: 1,
			Reference: &FlowFragmentReference{FlowFragmentID: "child", WorkflowVersionID: "child-v1"}, Children: []FlowFragmentStep{{ID: "child", DisplayName: "等待", Kind: StepWait, WaitMS: 1}}}, want: "REPEAT contains unsupported step configuration"},
		{name: "workflow reference cannot carry node", step: FlowFragmentStep{ID: "ref", DisplayName: "子流程", Kind: StepFlowFragmentRef,
			ElementTargetID: "node", ElementTargetVersionID: "node-v1", Reference: &FlowFragmentReference{FlowFragmentID: "child", WorkflowVersionID: "child-v1"}}, want: "WORKFLOW_REF contains unsupported step configuration"},
		{name: "workflow reference cannot carry children", step: FlowFragmentStep{ID: "ref", DisplayName: "子流程", Kind: StepFlowFragmentRef,
			Children: []FlowFragmentStep{{ID: "child", DisplayName: "等待", Kind: StepWait, WaitMS: 1}}, Reference: &FlowFragmentReference{FlowFragmentID: "child", WorkflowVersionID: "child-v1"}}, want: "WORKFLOW_REF contains unsupported step configuration"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := workflowWithSteps(test.step).Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestWorkflowAggregateValidateOwnsStepAndParameterIdentity(t *testing.T) {
	t.Run("duplicate nested step id", func(t *testing.T) {
		repeat := FlowFragmentStep{ID: "same", DisplayName: "循环", Kind: StepRepeat, RepeatCount: 1,
			Children: []FlowFragmentStep{{ID: "same", DisplayName: "等待", Kind: StepWait, WaitMS: 1}}}
		if err := workflowWithSteps(repeat).Validate(); err == nil || !strings.Contains(err.Error(), "duplicate step id") {
			t.Fatalf("duplicate nested step error = %v", err)
		}
	})

	tests := []struct {
		name       string
		parameters []ParameterDefinition
		want       string
	}{
		{name: "all supported parameter kinds", parameters: []ParameterDefinition{
			{Name: "text", DisplayName: "文本", Type: parameter.Text, Default: parameter.PresentValue(parameter.TextValue(""))},
			{Name: "number", DisplayName: "数字", Type: parameter.Number, Required: true},
			{Name: "boolean", DisplayName: "布尔", Type: parameter.Boolean, Default: parameter.PresentValue(parameter.BooleanValue(false))},
			{Name: "single", DisplayName: "单选", Type: parameter.SingleSelect, Options: []string{"a"}, Default: parameter.PresentValue(parameter.SingleSelectValue("a"))},
			{Name: "multi", DisplayName: "多选", Type: parameter.MultiSelect, Options: []string{"a", "b"}, Default: parameter.PresentValue(parameter.MultiSelectValue(nil))},
		}},
		{name: "missing name", parameters: []ParameterDefinition{{DisplayName: "参数", Type: parameter.Text}}, want: "name and display name"},
		{name: "missing display name", parameters: []ParameterDefinition{{Name: "param", Type: parameter.Text}}, want: "name and display name"},
		{name: "unsupported type", parameters: []ParameterDefinition{{Name: "param", DisplayName: "参数", Type: parameter.Type("DATE")}}, want: "unsupported parameter type"},
		{name: "select needs options", parameters: []ParameterDefinition{{Name: "param", DisplayName: "参数", Type: parameter.SingleSelect}}, want: "requires options"},
		{name: "select rejects blank option", parameters: []ParameterDefinition{{Name: "param", DisplayName: "参数", Type: parameter.MultiSelect, Options: []string{" "}}}, want: "cannot be empty"},
		{name: "select rejects duplicate option", parameters: []ParameterDefinition{{Name: "param", DisplayName: "参数", Type: parameter.SingleSelect, Options: []string{"a", "a"}}}, want: "duplicate option"},
		{name: "duplicate parameter name", parameters: []ParameterDefinition{{Name: "param", DisplayName: "参数 A", Type: parameter.Text}, {Name: "param", DisplayName: "参数 B", Type: parameter.Text}}, want: "duplicate parameter"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			aggregate := workflowWithSteps(FlowFragmentStep{ID: "wait", DisplayName: "等待", Kind: StepWait, WaitMS: 1})
			aggregate.Current.Definition.Parameters = test.parameters
			err := aggregate.Validate()
			if test.want == "" && err != nil {
				t.Fatalf("valid parameters rejected: %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func workflowActionStep(action, value string) FlowFragmentStep {
	return FlowFragmentStep{ID: action, DisplayName: action, Kind: StepAction, Action: action,
		ElementTargetID: "node", ElementTargetVersionID: "node-v1", Value: value}
}

func workflowWithSteps(steps ...FlowFragmentStep) FlowFragmentAggregate {
	return FlowFragmentAggregate{
		FlowFragment: FlowFragment{ID: "workflow", DisplayName: "流程", Properties: Properties{}, CurrentVersionID: "workflow-v1"},
		Current: FlowFragmentVersion{ID: "workflow-v1", FlowFragmentID: "workflow", VersionNumber: 1,
			Definition: FlowFragmentContent{Steps: steps}},
	}
}
