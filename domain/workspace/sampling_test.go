package workspace

import (
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

func TestParameterDefinitionRejectsEmptyAndDuplicateSelectOptions(t *testing.T) {
	for _, options := range [][]string{{""}, {"east", "east"}} {
		definition := ParameterDefinition{Name: "region", DisplayName: "区域", Type: ParameterSingleSelect, Options: options}
		if err := definition.Validate(); err == nil {
			t.Fatalf("options %#v were accepted", options)
		}
	}
}

func TestSamplingPublicationRequiresEveryWorkflowNodeReferenceDecision(t *testing.T) {
	node := sampledPublicationNode("node", "node-v1")
	publication := SamplingPublication{Workflow: sampledPublicationWorkflow("node", "node-v1")}
	if err := publication.Validate(); err == nil || !strings.Contains(err.Error(), "node decision") {
		t.Fatalf("missing node decision error = %v", err)
	}
	publication.Nodes = []SamplingNodePublication{{TemporaryNodeID: "temporary-node", Aggregate: node, PublishVersion: true}}
	if err := publication.Validate(); err != nil {
		t.Fatalf("matching node decision: %v", err)
	}
	publication.Nodes = append(publication.Nodes, SamplingNodePublication{
		TemporaryNodeID: "orphan-node", Aggregate: sampledPublicationNode("orphan", "orphan-v1"), PublishVersion: true,
	})
	if err := publication.Validate(); err != nil {
		t.Fatalf("independent orphan node asset: %v", err)
	}
}

func TestSamplingNodePublicationRejectsInconsistentPublishMode(t *testing.T) {
	merge := sampledPublicationNode("node", "node-v2")
	merge.Current.VersionNumber = 2
	tests := []struct {
		name        string
		aggregate   NodeAggregate
		expected    string
		publish     bool
		wantInvalid bool
	}{
		{name: "create", aggregate: sampledPublicationNode("node", "node-v1"), publish: true},
		{name: "reuse", aggregate: sampledPublicationNode("node", "node-v1"), expected: "node-v1"},
		{name: "merge", aggregate: merge, expected: "node-v1", publish: true},
		{name: "new node without version", aggregate: sampledPublicationNode("node", "node-v1"), wantInvalid: true},
		{name: "reuse different version", aggregate: sampledPublicationNode("node", "node-v1"), expected: "node-v0", wantInvalid: true},
		{name: "merge republishes expected", aggregate: sampledPublicationNode("node", "node-v1"), expected: "node-v1", publish: true, wantInvalid: true},
		{name: "merge starts again at version one", aggregate: sampledPublicationNode("node", "node-v1"), expected: "node-v0", publish: true, wantInvalid: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			publication := SamplingPublication{Workflow: sampledPublicationWorkflow(test.aggregate.Node.ID, test.aggregate.Current.ID),
				Nodes: []SamplingNodePublication{{TemporaryNodeID: "temporary-node", Aggregate: test.aggregate,
					ExpectedCurrentVersionID: test.expected, PublishVersion: test.publish}}}
			err := publication.Validate()
			if test.wantInvalid && err == nil {
				t.Fatal("invalid sampled node publication mode was accepted")
			}
			if !test.wantInvalid && err != nil {
				t.Fatalf("valid sampled node publication mode: %v", err)
			}
		})
	}
}

func sampledPublicationWorkflow(nodeID, versionID string) WorkflowAggregate {
	return WorkflowAggregate{Workflow: Workflow{ID: "workflow", DisplayName: "采样流程", Properties: Properties{},
		CurrentVersionID: "workflow-v1", CreatedAt: 1, UpdatedAt: 1}, Current: WorkflowVersion{
		ID: "workflow-v1", WorkflowID: "workflow", VersionNumber: 1, CreatedAt: 1,
		Definition: WorkflowDefinition{Steps: []WorkflowStep{{ID: "click", DisplayName: "点击", Kind: StepAction,
			Action: "click", NodeID: nodeID, NodeVersionID: versionID}}},
	}}
}

func sampledPublicationNode(nodeID, versionID string) NodeAggregate {
	return NodeAggregate{Node: Node{ID: nodeID, DisplayName: nodeID, Properties: Properties{},
		CurrentVersionID: versionID, CreatedAt: 1, UpdatedAt: 1}, Current: NodeVersion{
		ID: versionID, NodeID: nodeID, VersionNumber: 1,
		Selectors:   []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#" + nodeID}},
		Fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{}}, Source: SourceSampling, CreatedAt: 1,
	}}
}
