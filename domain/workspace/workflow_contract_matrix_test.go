package workspace

import (
	"strings"
	"testing"
)

func TestWorkflowAggregateValidateStepKindBusinessMatrix(t *testing.T) {
	tests := []struct {
		name string
		step WorkflowStep
	}{
		{name: "click", step: workflowActionStep("click", "")},
		{name: "input allows empty text", step: workflowActionStep("input", "")},
		{name: "single select value", step: workflowActionStep("select", "east")},
		{name: "multi select values", step: func() WorkflowStep {
			step := workflowActionStep("select", "")
			step.Values = []string{"east", "west"}
			return step
		}()},
		{name: "hover", step: workflowActionStep("hover", "")},
		{name: "navigate without node", step: WorkflowStep{ID: "navigate", DisplayName: "打开页面", Kind: StepAction, Action: "navigate", Value: "https://example.test"}},
		{name: "press without node", step: WorkflowStep{ID: "press", DisplayName: "按键", Kind: StepAction, Action: "press", Value: "Enter"}},
		{name: "noop", step: workflowActionStep("noop", "")},
		{name: "extract", step: workflowActionStep("extract", "order_id")},
		{name: "fixed sleep", step: WorkflowStep{ID: "sleep", DisplayName: "等待", Kind: StepWait, WaitKind: "sleep", WaitMS: 1}},
		{name: "empty wait kind means fixed sleep", step: WorkflowStep{ID: "sleep", DisplayName: "等待", Kind: StepWait, WaitMS: 1}},
		{name: "element wait allows adapter default timeout", step: WorkflowStep{ID: "element", DisplayName: "等待元素", Kind: StepWait, WaitKind: "element", NodeID: "node", NodeVersionID: "node-v1"}},
		{name: "network idle allows adapter default timeout", step: WorkflowStep{ID: "network", DisplayName: "等待网络", Kind: StepWait, WaitKind: "network_idle"}},
		{name: "repeat", step: WorkflowStep{ID: "repeat", DisplayName: "循环", Kind: StepRepeat, RepeatCount: 2, Children: []WorkflowStep{{ID: "nested-wait", DisplayName: "等待", Kind: StepWait, WaitMS: 1}}}},
		{name: "fixed workflow reference", step: WorkflowStep{ID: "fixed", DisplayName: "固定子流程", Kind: StepWorkflowRef, Reference: &WorkflowReference{WorkflowID: "child", WorkflowVersionID: "child-v1"}}},
		{name: "latest workflow reference", step: WorkflowStep{ID: "latest", DisplayName: "最新子流程", Kind: StepWorkflowRef, Reference: &WorkflowReference{WorkflowID: "child", LatestPublished: true}}},
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
		step WorkflowStep
		want string
	}{
		{name: "unsupported action", step: workflowActionStep("double_click", ""), want: "unsupported action"},
		{name: "click needs node", step: WorkflowStep{ID: "click", DisplayName: "点击", Kind: StepAction, Action: "click"}, want: "requires a node"},
		{name: "node needs exact version", step: func() WorkflowStep { step := workflowActionStep("click", ""); step.NodeVersionID = ""; return step }(), want: "exact node version"},
		{name: "navigate needs URL value", step: WorkflowStep{ID: "navigate", DisplayName: "打开", Kind: StepAction, Action: "navigate"}, want: "requires a value"},
		{name: "press needs key value", step: WorkflowStep{ID: "press", DisplayName: "按键", Kind: StepAction, Action: "press"}, want: "requires a value"},
		{name: "extract needs scratchpad name", step: workflowActionStep("extract", ""), want: "requires a value"},
		{name: "select needs one value", step: workflowActionStep("select", ""), want: "at least one value"},
		{name: "fixed wait must be positive", step: WorkflowStep{ID: "wait", DisplayName: "等待", Kind: StepWait, WaitMS: 0}, want: "fixed wait must be > 0"},
		{name: "element wait needs node", step: WorkflowStep{ID: "wait", DisplayName: "等待元素", Kind: StepWait, WaitKind: "element"}, want: "requires a node"},
		{name: "element wait needs exact version", step: WorkflowStep{ID: "wait", DisplayName: "等待元素", Kind: StepWait, WaitKind: "element", NodeID: "node"}, want: "exact node version"},
		{name: "element wait rejects negative timeout", step: WorkflowStep{ID: "wait", DisplayName: "等待元素", Kind: StepWait, WaitKind: "element", NodeID: "node", NodeVersionID: "node-v1", WaitMS: -1}, want: "timeout must be >= 0"},
		{name: "network wait rejects negative timeout", step: WorkflowStep{ID: "wait", DisplayName: "等待网络", Kind: StepWait, WaitKind: "network_idle", WaitMS: -1}, want: "timeout must be >= 0"},
		{name: "unknown wait kind", step: WorkflowStep{ID: "wait", DisplayName: "等待事件", Kind: StepWait, WaitKind: "event"}, want: "unsupported wait kind"},
		{name: "repeat needs positive count", step: WorkflowStep{ID: "repeat", DisplayName: "循环", Kind: StepRepeat, Children: []WorkflowStep{{ID: "child", DisplayName: "等待", Kind: StepWait, WaitMS: 1}}}, want: "requires count and children"},
		{name: "repeat needs children", step: WorkflowStep{ID: "repeat", DisplayName: "循环", Kind: StepRepeat, RepeatCount: 1}, want: "requires count and children"},
		{name: "reference needs target", step: WorkflowStep{ID: "ref", DisplayName: "子流程", Kind: StepWorkflowRef}, want: "requires a workflow reference"},
		{name: "latest reference cannot pin version", step: WorkflowStep{ID: "ref", DisplayName: "子流程", Kind: StepWorkflowRef, Reference: &WorkflowReference{WorkflowID: "child", WorkflowVersionID: "child-v1", LatestPublished: true}}, want: "cannot persist a version"},
		{name: "fixed reference needs version", step: WorkflowStep{ID: "ref", DisplayName: "子流程", Kind: StepWorkflowRef, Reference: &WorkflowReference{WorkflowID: "child"}}, want: "requires a version"},
		{name: "only action can be optional", step: WorkflowStep{ID: "wait", DisplayName: "等待", Kind: StepWait, WaitMS: 1, Optional: true}, want: "only ACTION can be optional"},
		{name: "unknown kind", step: WorkflowStep{ID: "unknown", DisplayName: "未知", Kind: StepKind("UNKNOWN")}, want: "unsupported kind"},
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
		step WorkflowStep
		want string
	}{
		{name: "action cannot carry workflow reference", step: func() WorkflowStep {
			step := workflowActionStep("click", "")
			step.Reference = &WorkflowReference{WorkflowID: "child", WorkflowVersionID: "child-v1"}
			return step
		}(), want: "ACTION contains unsupported step configuration"},
		{name: "action cannot carry children", step: func() WorkflowStep {
			step := workflowActionStep("click", "")
			step.Children = []WorkflowStep{{ID: "child", DisplayName: "等待", Kind: StepWait, WaitMS: 1}}
			return step
		}(), want: "ACTION contains unsupported step configuration"},
		{name: "wait cannot carry action fields", step: WorkflowStep{ID: "wait", DisplayName: "等待", Kind: StepWait, WaitMS: 1, Action: "navigate", Value: "https://example.test"}, want: "WAIT contains unsupported step configuration"},
		{name: "fixed wait cannot carry node", step: WorkflowStep{ID: "wait", DisplayName: "等待", Kind: StepWait, WaitMS: 1, NodeID: "node", NodeVersionID: "node-v1"}, want: "WAIT contains unsupported step configuration"},
		{name: "repeat cannot carry node", step: WorkflowStep{ID: "repeat", DisplayName: "循环", Kind: StepRepeat, RepeatCount: 1,
			NodeID: "node", NodeVersionID: "node-v1", Children: []WorkflowStep{{ID: "child", DisplayName: "等待", Kind: StepWait, WaitMS: 1}}}, want: "REPEAT contains unsupported step configuration"},
		{name: "repeat cannot carry reference", step: WorkflowStep{ID: "repeat", DisplayName: "循环", Kind: StepRepeat, RepeatCount: 1,
			Reference: &WorkflowReference{WorkflowID: "child", WorkflowVersionID: "child-v1"}, Children: []WorkflowStep{{ID: "child", DisplayName: "等待", Kind: StepWait, WaitMS: 1}}}, want: "REPEAT contains unsupported step configuration"},
		{name: "workflow reference cannot carry node", step: WorkflowStep{ID: "ref", DisplayName: "子流程", Kind: StepWorkflowRef,
			NodeID: "node", NodeVersionID: "node-v1", Reference: &WorkflowReference{WorkflowID: "child", WorkflowVersionID: "child-v1"}}, want: "WORKFLOW_REF contains unsupported step configuration"},
		{name: "workflow reference cannot carry children", step: WorkflowStep{ID: "ref", DisplayName: "子流程", Kind: StepWorkflowRef,
			Children: []WorkflowStep{{ID: "child", DisplayName: "等待", Kind: StepWait, WaitMS: 1}}, Reference: &WorkflowReference{WorkflowID: "child", WorkflowVersionID: "child-v1"}}, want: "WORKFLOW_REF contains unsupported step configuration"},
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
		repeat := WorkflowStep{ID: "same", DisplayName: "循环", Kind: StepRepeat, RepeatCount: 1,
			Children: []WorkflowStep{{ID: "same", DisplayName: "等待", Kind: StepWait, WaitMS: 1}}}
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
			{Name: "text", DisplayName: "文本", Type: ParameterText},
			{Name: "number", DisplayName: "数字", Type: ParameterNumber},
			{Name: "boolean", DisplayName: "布尔", Type: ParameterBoolean},
			{Name: "single", DisplayName: "单选", Type: ParameterSingleSelect, Options: []string{"a"}},
			{Name: "multi", DisplayName: "多选", Type: ParameterMultiSelect, Options: []string{"a", "b"}},
		}},
		{name: "missing name", parameters: []ParameterDefinition{{DisplayName: "参数", Type: ParameterText}}, want: "name and display name"},
		{name: "missing display name", parameters: []ParameterDefinition{{Name: "param", Type: ParameterText}}, want: "name and display name"},
		{name: "unsupported type", parameters: []ParameterDefinition{{Name: "param", DisplayName: "参数", Type: ParameterType("DATE")}}, want: "unsupported parameter type"},
		{name: "select needs options", parameters: []ParameterDefinition{{Name: "param", DisplayName: "参数", Type: ParameterSingleSelect}}, want: "requires options"},
		{name: "select rejects blank option", parameters: []ParameterDefinition{{Name: "param", DisplayName: "参数", Type: ParameterMultiSelect, Options: []string{" "}}}, want: "cannot be empty"},
		{name: "select rejects duplicate option", parameters: []ParameterDefinition{{Name: "param", DisplayName: "参数", Type: ParameterSingleSelect, Options: []string{"a", "a"}}}, want: "duplicate option"},
		{name: "duplicate parameter name", parameters: []ParameterDefinition{{Name: "param", DisplayName: "参数 A", Type: ParameterText}, {Name: "param", DisplayName: "参数 B", Type: ParameterText}}, want: "duplicate parameter"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			aggregate := workflowWithSteps(WorkflowStep{ID: "wait", DisplayName: "等待", Kind: StepWait, WaitMS: 1})
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

func workflowActionStep(action, value string) WorkflowStep {
	return WorkflowStep{ID: action, DisplayName: action, Kind: StepAction, Action: action,
		NodeID: "node", NodeVersionID: "node-v1", Value: value}
}

func workflowWithSteps(steps ...WorkflowStep) WorkflowAggregate {
	return WorkflowAggregate{
		Workflow: Workflow{ID: "workflow", DisplayName: "流程", Properties: Properties{}, CurrentVersionID: "workflow-v1"},
		Current: WorkflowVersion{ID: "workflow-v1", WorkflowID: "workflow", VersionNumber: 1,
			Definition: WorkflowDefinition{Steps: steps}},
	}
}
