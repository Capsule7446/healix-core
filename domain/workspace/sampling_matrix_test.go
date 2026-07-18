package workspace

import (
	"reflect"
	"strings"
	"testing"
)

func TestRebuildTemporaryNodeReferencesTraversesEditableWorkflowTree(t *testing.T) {
	workflow := TemporarySamplingWorkflow{
		Nodes: []TemporarySamplingNode{
			{ID: "node-a", StepIDs: []string{"stale"}},
			{ID: "node-b", StepIDs: []string{"stale"}},
			{ID: "unused", StepIDs: []string{"stale"}},
		},
		Steps: []WorkflowStep{
			{ID: "root-a", NodeID: "node-a"},
			{ID: "repeat", Children: []WorkflowStep{{ID: "nested-b", NodeID: "node-b"}, {ID: "nested-a", NodeID: "node-a"}}},
			{ID: "group", ValidationGroup: &ValidationGroup{Branches: []ValidationBranch{
				{ID: "branch-a", Steps: []WorkflowStep{{ID: "validation-a", NodeID: "node-a"}}},
				{ID: "branch-b", Steps: []WorkflowStep{{ID: "validation-b", NodeID: "node-b"}}},
			}}},
		},
	}
	if err := RebuildTemporaryNodeReferences(&workflow); err != nil {
		t.Fatalf("RebuildTemporaryNodeReferences: %v", err)
	}
	want := map[string][]string{
		"node-a": {"root-a", "nested-a", "validation-a"},
		"node-b": {"nested-b", "validation-b"},
		"unused": nil,
	}
	for _, node := range workflow.Nodes {
		if !reflect.DeepEqual(node.StepIDs, want[node.ID]) {
			t.Fatalf("node %s StepIDs = %v, want %v", node.ID, node.StepIDs, want[node.ID])
		}
	}

	workflow.Steps = []WorkflowStep{{ID: "only-b", NodeID: "node-b"}}
	if err := RebuildTemporaryNodeReferences(&workflow); err != nil {
		t.Fatalf("second rebuild: %v", err)
	}
	if workflow.Nodes[0].StepIDs != nil || !reflect.DeepEqual(workflow.Nodes[1].StepIDs, []string{"only-b"}) || workflow.Nodes[2].StepIDs != nil {
		t.Fatalf("stale references were not replaced: %#v", workflow.Nodes)
	}
}

func TestRebuildTemporaryNodeReferencesRejectsInvalidInputMatrix(t *testing.T) {
	if err := RebuildTemporaryNodeReferences(nil); err == nil || !strings.Contains(err.Error(), "workflow is required") {
		t.Fatalf("nil workflow error = %v", err)
	}
	tests := []struct {
		name string
		step WorkflowStep
	}{
		{name: "root", step: WorkflowStep{ID: "root", NodeID: "unknown"}},
		{name: "nested child", step: WorkflowStep{ID: "repeat", Children: []WorkflowStep{{ID: "nested", NodeID: "unknown"}}}},
		{name: "validation branch", step: WorkflowStep{ID: "group", ValidationGroup: &ValidationGroup{Branches: []ValidationBranch{{
			ID: "branch", Steps: []WorkflowStep{{ID: "member", NodeID: "unknown"}},
		}}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workflow := TemporarySamplingWorkflow{Nodes: []TemporarySamplingNode{{ID: "known", StepIDs: []string{"stale"}}}, Steps: []WorkflowStep{test.step}}
			err := RebuildTemporaryNodeReferences(&workflow)
			if err == nil || !strings.Contains(err.Error(), "references unknown temporary node unknown") {
				t.Fatalf("unknown temporary node error = %v", err)
			}
			if workflow.Nodes[0].StepIDs != nil {
				t.Fatalf("rebuild did not clear stale projection before deriving: %v", workflow.Nodes[0].StepIDs)
			}
		})
	}
}

func TestSamplingPublicationValidatesRecursiveExactNodeDecisions(t *testing.T) {
	workflow := sampledPublicationWorkflow("node-a", "node-a-v1")
	workflow.Current.Definition.Steps = []WorkflowStep{
		{ID: "root", DisplayName: "根节点", Kind: StepAction, Action: "click", NodeID: "node-a", NodeVersionID: "node-a-v1"},
		{ID: "repeat", DisplayName: "循环", Kind: StepRepeat, RepeatCount: 1, Children: []WorkflowStep{{
			ID: "child", DisplayName: "子节点", Kind: StepAction, Action: "click", NodeID: "node-b", NodeVersionID: "node-b-v1",
		}}},
		{ID: "group", DisplayName: "验证", Kind: StepValidationGroup, ValidationGroup: &ValidationGroup{
			Wait: ValidationWait{MaxWaitMS: 2_000, StabilityMS: 200},
			Branches: []ValidationBranch{
				{ID: "branch", Name: "分支", Steps: []WorkflowStep{
					{
						ID: "member", DisplayName: "成员", Kind: StepValidation, NodeID: "node-c", NodeVersionID: "node-c-v1",
						Validation: &ValidationConfig{Assertion: ValidationAssertion{Kind: ValidationVisible}},
					},
				}},
			},
		}},
	}
	publication := SamplingPublication{
		Workflow: workflow,
		Nodes: []SamplingNodePublication{
			{TemporaryNodeID: "temporary-a", Aggregate: sampledPublicationNode("node-a", "node-a-v1"), PublishVersion: true},
			{TemporaryNodeID: "temporary-b", Aggregate: sampledPublicationNode("node-b", "node-b-v1"), PublishVersion: true},
			{TemporaryNodeID: "temporary-c", Aggregate: sampledPublicationNode("node-c", "node-c-v1"), PublishVersion: true},
		},
	}
	if err := publication.Validate(); err != nil {
		t.Fatalf("recursive sampled publication rejected: %v", err)
	}

	tests := []struct {
		name  string
		index int
		want  string
	}{
		{name: "root decision missing", index: 0, want: "node-a/node-a-v1"},
		{name: "nested child decision missing", index: 1, want: "node-b/node-b-v1"},
		{name: "validation member decision missing", index: 2, want: "node-c/node-c-v1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invalid := publication
			invalid.Nodes = append([]SamplingNodePublication(nil), publication.Nodes...)
			invalid.Nodes = append(invalid.Nodes[:test.index], invalid.Nodes[test.index+1:]...)
			err := invalid.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("missing recursive decision error = %v, want identity %q", err, test.want)
			}
		})
	}
}

func TestSamplingPublicationRejectsDecisionIdentityMatrix(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SamplingPublication)
		want   string
	}{
		{name: "blank temporary id", mutate: func(publication *SamplingPublication) { publication.Nodes[0].TemporaryNodeID = " " }, want: "temporary id is required"},
		{name: "duplicate temporary id", mutate: func(publication *SamplingPublication) {
			publication.Nodes = append(publication.Nodes, SamplingNodePublication{TemporaryNodeID: publication.Nodes[0].TemporaryNodeID,
				Aggregate: sampledPublicationNode("other", "other-v1"), PublishVersion: true})
		}, want: "duplicate sampled node"},
		{name: "invalid aggregate", mutate: func(publication *SamplingPublication) { publication.Nodes[0].Aggregate.Node.DisplayName = "" }, want: "display name is required"},
		{name: "invalid sampled workflow", mutate: func(publication *SamplingPublication) { publication.Workflow.Current.Definition.Steps = nil }, want: "sampled workflow"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			publication := SamplingPublication{
				Workflow: sampledPublicationWorkflow("node", "node-v1"),
				Nodes:    []SamplingNodePublication{{TemporaryNodeID: "temporary", Aggregate: sampledPublicationNode("node", "node-v1"), PublishVersion: true}},
			}
			test.mutate(&publication)
			if err := publication.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.want)
			}
		})
	}
}
