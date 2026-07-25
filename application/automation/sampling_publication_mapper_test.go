package automation

import (
	"reflect"
	"testing"

	domainautomation "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/sampling"
)

func sampledFingerprint(tag string) fingerprint.Fingerprint {
	return fingerprint.Fingerprint{Tag: tag, Attributes: map[string]string{"id": tag}}
}

func sampledSelector(value string) []fingerprint.Selector {
	return []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: value}}
}

func sampledCurrentNode(t *testing.T) domainautomation.NodeAggregate {
	t.Helper()
	aggregate, err := domainautomation.NewNode(
		domainautomation.Node{ID: "existing", DisplayName: "existing", Properties: domainautomation.Properties{"owner": "kept"}, CreatedAt: 1, UpdatedAt: 1},
		domainautomation.NodeVersion{ID: "existing-v1", PageURL: "/old", Origin: "old", Selectors: sampledSelector("#old"), Fingerprint: sampledFingerprint("old"), Source: domainautomation.SourceManual, CreatedAt: 1},
	)
	if err != nil {
		t.Fatalf("NewNode: %v", err)
	}
	return aggregate
}

func sampledWorkflow(mode sampling.SamplingResolutionMode) sampling.TemporarySamplingWorkflow {
	return sampling.TemporarySamplingWorkflow{
		ID: "temporary-workflow", DisplayName: "sampled", Properties: domainautomation.Properties{"kind": "sampled"},
		Steps: []domainautomation.WorkflowStep{{ID: "repeat", DisplayName: "repeat", Kind: domainautomation.StepRepeat, RepeatCount: 1, Children: []domainautomation.WorkflowStep{{ID: "action", DisplayName: "action", Kind: domainautomation.StepAction, Action: "click", NodeID: "temporary-node"}}}},
		Nodes: []sampling.TemporarySamplingNode{{ID: "temporary-node", DisplayName: "sampled-node", Properties: domainautomation.Properties{"sampled": "yes"}, PageURL: "/new", Origin: "new", Selectors: sampledSelector("#new"), Fingerprint: sampledFingerprint("new"), ResolutionMode: mode}},
	}
}

func TestMapSamplingPublicationModes(t *testing.T) {
	current := sampledCurrentNode(t)
	tests := []struct {
		name      string
		mode      sampling.SamplingResolutionMode
		authority SamplingNodeAuthority
		wantID    string
		wantVer   string
		publish   bool
	}{
		{name: "create", mode: sampling.SamplingResolutionCreate, authority: SamplingNodeAuthority{TemporaryNodeID: "temporary-node", NodeID: "created", NodeVersionID: "created-v1"}, wantID: "created", wantVer: "created-v1", publish: true},
		{name: "force create", mode: sampling.SamplingResolutionForceCreate, authority: SamplingNodeAuthority{TemporaryNodeID: "temporary-node", NodeID: "forced", NodeVersionID: "forced-v1", ForceCreateAuthorized: true}, wantID: "forced", wantVer: "forced-v1", publish: true},
		{name: "merge", mode: sampling.SamplingResolutionMerge, authority: SamplingNodeAuthority{TemporaryNodeID: "temporary-node", NodeID: "existing", NodeVersionID: "existing-v2", Current: &current, ExpectedRevision: current.Node.Revision, ExpectedCurrentVersionID: current.Current.ID}, wantID: "existing", wantVer: "existing-v2", publish: true},
		{name: "reuse", mode: sampling.SamplingResolutionReuse, authority: SamplingNodeAuthority{TemporaryNodeID: "temporary-node", NodeID: "existing", NodeVersionID: "existing-v1", Current: &current, ExpectedRevision: current.Node.Revision, ExpectedCurrentVersionID: current.Current.ID}, wantID: "existing", wantVer: "existing-v1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace := sampledWorkflow(test.mode)
			before := sampledWorkflow(test.mode)
			publication, err := MapSamplingPublication(SamplingPublicationRequest{WorkflowID: "workflow", WorkflowVersionID: "workflow-v1", PublishedAt: 2, Workspace: workspace, Nodes: []SamplingNodeAuthority{test.authority}})
			if err != nil {
				t.Fatalf("MapSamplingPublication: %v", err)
			}
			node := publication.Nodes[0]
			if node.Aggregate.Node.ID != test.wantID || node.Aggregate.Current.ID != test.wantVer || node.PublishVersion != test.publish {
				t.Fatalf("node decision = %#v", node)
			}
			step := publication.Workflow.Current.Definition.Steps[0].Children[0]
			if step.NodeID != test.wantID || step.NodeVersionID != test.wantVer {
				t.Fatalf("rewritten step = %#v", step)
			}
			if test.mode == sampling.SamplingResolutionMerge {
				if node.Aggregate.Node.DisplayName != current.Node.DisplayName || node.Aggregate.Node.Properties["owner"] != "kept" || node.Aggregate.Current.Selectors[0].Value != "#new" || len(node.Aggregate.Current.Selectors) != 1 {
					t.Fatalf("merge did not preserve metadata and replace version content: %#v", node.Aggregate)
				}
			}
			if test.mode == sampling.SamplingResolutionReuse {
				node.Aggregate.Node.Properties["owner"] = "mutated"
				node.Aggregate.Current.Fingerprint.Attributes["id"] = "mutated"
				if current.Node.Properties["owner"] != "kept" || current.Current.Fingerprint.Attributes["id"] != "old" {
					t.Fatal("reuse publication aliases current aggregate")
				}
			}
			if !reflect.DeepEqual(workspace, before) {
				t.Fatal("mapper mutated temporary workflow")
			}
		})
	}
}

func TestMapSamplingPublicationRejectsInvalidAuthority(t *testing.T) {
	current := sampledCurrentNode(t)
	tests := []struct {
		name      string
		mode      sampling.SamplingResolutionMode
		authority []SamplingNodeAuthority
	}{
		{name: "undecided", mode: sampling.SamplingResolutionUndecided, authority: []SamplingNodeAuthority{{TemporaryNodeID: "temporary-node", NodeID: "node", NodeVersionID: "node-v1"}}},
		{name: "unauthorized force create", mode: sampling.SamplingResolutionForceCreate, authority: []SamplingNodeAuthority{{TemporaryNodeID: "temporary-node", NodeID: "node", NodeVersionID: "node-v1"}}},
		{name: "stale merge revision", mode: sampling.SamplingResolutionMerge, authority: []SamplingNodeAuthority{{TemporaryNodeID: "temporary-node", NodeID: "existing", NodeVersionID: "existing-v2", Current: &current, ExpectedRevision: current.Node.Revision + 1, ExpectedCurrentVersionID: current.Current.ID}}},
		{name: "stale reuse version", mode: sampling.SamplingResolutionReuse, authority: []SamplingNodeAuthority{{TemporaryNodeID: "temporary-node", NodeID: "existing", NodeVersionID: "existing-v1", Current: &current, ExpectedRevision: current.Node.Revision, ExpectedCurrentVersionID: "stale"}}},
		{name: "missing authority", mode: sampling.SamplingResolutionCreate},
		{name: "duplicate formal node", mode: sampling.SamplingResolutionCreate, authority: []SamplingNodeAuthority{{TemporaryNodeID: "temporary-node", NodeID: "node", NodeVersionID: "node-v1"}, {TemporaryNodeID: "extra", NodeID: "node", NodeVersionID: "extra-v1"}}},
		{name: "extra authority", mode: sampling.SamplingResolutionCreate, authority: []SamplingNodeAuthority{{TemporaryNodeID: "temporary-node", NodeID: "node", NodeVersionID: "node-v1"}, {TemporaryNodeID: "extra", NodeID: "extra", NodeVersionID: "extra-v1"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := MapSamplingPublication(SamplingPublicationRequest{WorkflowID: "workflow", WorkflowVersionID: "workflow-v1", PublishedAt: 2, Workspace: sampledWorkflow(test.mode), Nodes: test.authority})
			if err == nil {
				t.Fatal("invalid authority was accepted")
			}
		})
	}
}
