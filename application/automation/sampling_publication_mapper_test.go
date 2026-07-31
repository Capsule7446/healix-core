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

func sampledCurrentNode(t *testing.T) domainautomation.ElementTargetAggregate {
	t.Helper()
	aggregate, err := domainautomation.NewElementTarget(
		domainautomation.ElementTarget{ID: "existing", DisplayName: "existing", Properties: domainautomation.Properties{"owner": "kept"}, CreatedAt: 1, UpdatedAt: 1},
		domainautomation.ElementTargetVersion{ID: "existing-v1", PageURL: "/old", Origin: "old", Selectors: sampledSelector("#old"), Fingerprint: sampledFingerprint("old"), Source: domainautomation.SourceManual, CreatedAt: 1},
	)
	if err != nil {
		t.Fatalf("NewElementTarget: %v", err)
	}
	return aggregate
}

func sampledWorkflow(mode sampling.ResolutionMode) sampling.UnpublishedFlowFragment {
	return sampling.UnpublishedFlowFragment{
		ID: "temporary-workflow", DisplayName: "sampled", Properties: domainautomation.Properties{"kind": "sampled"},
		Steps: []domainautomation.FlowFragmentStep{{ID: "repeat", DisplayName: "repeat", Kind: domainautomation.StepRepeat, RepeatCount: 1, Children: []domainautomation.FlowFragmentStep{{ID: "action", DisplayName: "action", Kind: domainautomation.StepAction, Action: "click", ElementTargetID: "temporary-node"}}}},
		Nodes: []sampling.UnpublishedElementTarget{{ID: "temporary-node", DisplayName: "sampled-node", Properties: domainautomation.Properties{"sampled": "yes"}, PageURL: "/new", Origin: "new", Selectors: sampledSelector("#new"), Fingerprint: sampledFingerprint("new"), ResolutionMode: mode}},
	}
}

func TestMapSamplingPublicationModes(t *testing.T) {
	current := sampledCurrentNode(t)
	tests := []struct {
		name      string
		mode      sampling.ResolutionMode
		authority SamplingNodeAuthority
		wantID    string
		wantVer   string
		publish   bool
	}{
		{name: "create", mode: sampling.ResolutionModeCreate, authority: SamplingNodeAuthority{TemporaryElementTargetID: "temporary-node", ElementTargetID: "created", ElementTargetVersionID: "created-v1"}, wantID: "created", wantVer: "created-v1", publish: true},
		{name: "force create", mode: sampling.ResolutionModeCreate, authority: SamplingNodeAuthority{TemporaryElementTargetID: "temporary-node", ElementTargetID: "forced", ElementTargetVersionID: "forced-v1"}, wantID: "forced", wantVer: "forced-v1", publish: true},
		{name: "merge", mode: sampling.ResolutionModeMerge, authority: SamplingNodeAuthority{TemporaryElementTargetID: "temporary-node", ElementTargetID: "existing", ElementTargetVersionID: "existing-v2", Current: &current, ExpectedRevision: current.ElementTarget.Revision, ExpectedCurrentVersionID: current.Current.ID}, wantID: "existing", wantVer: "existing-v2", publish: true},
		{name: "reuse", mode: sampling.ResolutionModeReuse, authority: SamplingNodeAuthority{TemporaryElementTargetID: "temporary-node", ElementTargetID: "existing", ElementTargetVersionID: "existing-v1", Current: &current, ExpectedRevision: current.ElementTarget.Revision, ExpectedCurrentVersionID: current.Current.ID}, wantID: "existing", wantVer: "existing-v1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace := sampledWorkflow(test.mode)
			before := sampledWorkflow(test.mode)
			publication, err := MapSamplingPublication(SamplingPublicationRequest{FlowFragmentID: "workflow", WorkflowVersionID: "workflow-v1", PublishedAt: 2, Workspace: workspace, Nodes: []SamplingNodeAuthority{test.authority}})
			if err != nil {
				t.Fatalf("MapSamplingPublication: %v", err)
			}
			node := publication.Nodes[0]
			if node.Aggregate.ElementTarget.ID != test.wantID || node.Aggregate.Current.ID != test.wantVer || node.PublishVersion != test.publish {
				t.Fatalf("node decision = %#v", node)
			}
			step := publication.FlowFragment.Current.Definition.Steps[0].Children[0]
			if step.ElementTargetID != test.wantID || step.ElementTargetVersionID != test.wantVer {
				t.Fatalf("rewritten step = %#v", step)
			}
			if test.mode == sampling.ResolutionModeMerge {
				if node.Aggregate.ElementTarget.DisplayName != current.ElementTarget.DisplayName || node.Aggregate.ElementTarget.Properties["owner"] != "kept" || node.Aggregate.Current.Selectors[0].Value != "#new" || len(node.Aggregate.Current.Selectors) != 1 {
					t.Fatalf("merge did not preserve metadata and replace version content: %#v", node.Aggregate)
				}
			}
			if test.mode == sampling.ResolutionModeReuse {
				node.Aggregate.ElementTarget.Properties["owner"] = "mutated"
				node.Aggregate.Current.Fingerprint.Attributes["id"] = "mutated"
				if current.ElementTarget.Properties["owner"] != "kept" || current.Current.Fingerprint.Attributes["id"] != "old" {
					t.Fatal("reuse publication aliases current aggregate")
				}
			}
			if !reflect.DeepEqual(workspace, before) {
				t.Fatal("mapper mutated temporary workflow")
			}
		})
	}
}

func TestMapSamplingPublicationAllowsHistoricalReuse(t *testing.T) {
	current := sampledCurrentNode(t)
	versioned, err := current.PublishVersion(
		"existing-v2",
		"/current",
		"current",
		sampledSelector("#current"),
		sampledFingerprint("current"),
		domainautomation.SourceManual,
		2,
	)
	if err != nil {
		t.Fatalf("PublishVersion: %v", err)
	}

	publication, err := MapSamplingPublication(SamplingPublicationRequest{
		FlowFragmentID:    "workflow",
		WorkflowVersionID: "workflow-v1",
		PublishedAt:       3,
		Workspace:         sampledWorkflow(sampling.ResolutionModeReuse),
		Nodes: []SamplingNodeAuthority{{
			TemporaryElementTargetID: "temporary-node",
			ElementTargetID:          "existing",
			ElementTargetVersionID:   "existing-v1",
			Current:                  &versioned,
			ExpectedRevision:         versioned.ElementTarget.Revision,
			ExpectedCurrentVersionID: versioned.Current.ID,
		}},
	})
	if err != nil {
		t.Fatalf("MapSamplingPublication: %v", err)
	}

	decision := publication.Nodes[0]
	if decision.Aggregate.Current.ID != "existing-v1" || decision.Aggregate.ElementTarget.CurrentVersionID != "existing-v2" {
		t.Fatalf("reuse projection = current %q / persisted pointer %q, want selected historical version with current CAS pointer", decision.Aggregate.Current.ID, decision.Aggregate.ElementTarget.CurrentVersionID)
	}
	if decision.Aggregate.Current.Selectors[0].Value != "#old" {
		t.Fatalf("reuse selected version content = %#v, want historical version", decision.Aggregate.Current)
	}
	decision.Aggregate.Current.Selectors[0].Value = "#mutated"
	decision.Aggregate.Current.Fingerprint.Attributes["id"] = "mutated"
	if versioned.Versions[0].Selectors[0].Value != "#old" || versioned.Versions[0].Fingerprint.Attributes["id"] != "old" {
		t.Fatal("historical reuse publication aliases authoritative history")
	}
	step := publication.FlowFragment.Current.Definition.Steps[0].Children[0]
	if step.ElementTargetID != "existing" || step.ElementTargetVersionID != "existing-v1" {
		t.Fatalf("rewritten step = %#v, want fixed historical version", step)
	}
}

func TestMapSamplingPublicationRejectsInvalidAuthority(t *testing.T) {
	current := sampledCurrentNode(t)
	tests := []struct {
		name      string
		mode      sampling.ResolutionMode
		authority []SamplingNodeAuthority
	}{
		{name: "undecided", mode: sampling.ResolutionModeUndecided, authority: []SamplingNodeAuthority{{TemporaryElementTargetID: "temporary-node", ElementTargetID: "node", ElementTargetVersionID: "node-v1"}}},
		{name: "stale merge revision", mode: sampling.ResolutionModeMerge, authority: []SamplingNodeAuthority{{TemporaryElementTargetID: "temporary-node", ElementTargetID: "existing", ElementTargetVersionID: "existing-v2", Current: &current, ExpectedRevision: current.ElementTarget.Revision + 1, ExpectedCurrentVersionID: current.Current.ID}}},
		{name: "stale reuse version", mode: sampling.ResolutionModeReuse, authority: []SamplingNodeAuthority{{TemporaryElementTargetID: "temporary-node", ElementTargetID: "existing", ElementTargetVersionID: "existing-v1", Current: &current, ExpectedRevision: current.ElementTarget.Revision, ExpectedCurrentVersionID: "stale"}}},
		{name: "missing historical reuse version", mode: sampling.ResolutionModeReuse, authority: []SamplingNodeAuthority{{TemporaryElementTargetID: "temporary-node", ElementTargetID: "existing", ElementTargetVersionID: "missing", Current: &current, ExpectedRevision: current.ElementTarget.Revision, ExpectedCurrentVersionID: current.Current.ID}}},
		{name: "missing authority", mode: sampling.ResolutionModeCreate},
		{name: "duplicate formal node", mode: sampling.ResolutionModeCreate, authority: []SamplingNodeAuthority{{TemporaryElementTargetID: "temporary-node", ElementTargetID: "node", ElementTargetVersionID: "node-v1"}, {TemporaryElementTargetID: "extra", ElementTargetID: "node", ElementTargetVersionID: "extra-v1"}}},
		{name: "extra authority", mode: sampling.ResolutionModeCreate, authority: []SamplingNodeAuthority{{TemporaryElementTargetID: "temporary-node", ElementTargetID: "node", ElementTargetVersionID: "node-v1"}, {TemporaryElementTargetID: "extra", ElementTargetID: "extra", ElementTargetVersionID: "extra-v1"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := MapSamplingPublication(SamplingPublicationRequest{FlowFragmentID: "workflow", WorkflowVersionID: "workflow-v1", PublishedAt: 2, Workspace: sampledWorkflow(test.mode), Nodes: test.authority})
			if err == nil {
				t.Fatal("invalid authority was accepted")
			}
		})
	}
}
