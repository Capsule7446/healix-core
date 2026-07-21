package automation

import (
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

func TestNodeAggregatePublishVersionAppendsWithoutMutatingHistory(t *testing.T) {
	base := NodeVersion{ID: "node-v1", NodeID: "node", VersionNumber: 1,
		Selectors:   []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#old"}},
		Fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{"id": "old"}},
		Source:      SourceManual, CreatedAt: 1}
	aggregate := NodeAggregate{Node: Node{ID: "node", DisplayName: "提交", Properties: Properties{},
		CurrentVersionID: base.ID, CreatedAt: 1, UpdatedAt: 1, Revision: 1}, Current: base, Versions: []NodeVersion{base}}
	fp := fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{"id": "new"}}
	selectors := []fingerprint.Selector{{Type: fingerprint.SelectorTestID, Value: "submit"}}

	published, err := aggregate.PublishVersion("node-v2", "/checkout", "https://example.test", selectors, fp, SourceAutoHeal, 2)
	if err != nil {
		t.Fatalf("PublishVersion: %v", err)
	}
	if published.Current.VersionNumber != 2 || published.Node.CurrentVersionID != "node-v2" || len(published.Versions) != 2 {
		t.Fatalf("unexpected publication: %#v", published)
	}
	selectors[0].Value = "mutated"
	fp.Attributes["id"] = "mutated"
	if published.Current.Selectors[0].Value != "submit" || published.Current.Fingerprint.Attributes["id"] != "new" {
		t.Fatalf("published version aliases command input: %#v", published.Current)
	}
	if aggregate.Node.CurrentVersionID != "node-v1" || aggregate.Current.VersionNumber != 1 || len(aggregate.Versions) != 1 {
		t.Fatalf("receiver was mutated: %#v", aggregate)
	}
}

func TestNodeAggregateValidateLoadedHistoryRejectsMissingCurrent(t *testing.T) {
	base := NodeVersion{ID: "node-v1", NodeID: "node", VersionNumber: 1,
		Selectors:   []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#old"}},
		Fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{}}, Source: SourceManual}
	aggregate := NodeAggregate{Node: Node{ID: "node", DisplayName: "节点", Properties: Properties{},
		CurrentVersionID: "node-v1"}, Current: base, Versions: nil}
	if err := aggregate.ValidateLoadedHistory(); err == nil || !strings.Contains(err.Error(), "missing from loaded history") {
		t.Fatalf("missing current history error = %v", err)
	}
	aggregate.Versions = []NodeVersion{base}
	if err := aggregate.ValidateLoadedHistory(); err != nil {
		t.Fatalf("valid loaded history: %v", err)
	}
}

func TestWorkflowAggregateValidateLoadedHistoryAllowsAllVersionsDeleted(t *testing.T) {
	deleted := WorkflowVersion{ID: "workflow-v1", WorkflowID: "workflow", VersionNumber: 1,
		Definition: WorkflowDefinition{Steps: []WorkflowStep{{ID: "wait", DisplayName: "等待", Kind: StepWait,
			WaitKind: "sleep", WaitMS: 1}}}, DeletedAt: 2}
	aggregate := WorkflowAggregate{Workflow: Workflow{ID: "workflow", DisplayName: "流程", Properties: Properties{}},
		Versions: []WorkflowVersion{deleted}}
	if err := aggregate.ValidateLoadedHistory(); err != nil {
		t.Fatalf("all-deleted history: %v", err)
	}
}

func TestNodeAggregatePublishVersionRejectsInconsistentCurrentPointer(t *testing.T) {
	base := NodeVersion{ID: "node-v1", NodeID: "node", VersionNumber: 1,
		Selectors:   []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#old"}},
		Fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{}}, Source: SourceManual}
	aggregate := NodeAggregate{Node: Node{ID: "node", DisplayName: "提交", Properties: Properties{}, CurrentVersionID: "other"}, Current: base}
	if _, err := aggregate.PublishVersion("node-v2", "", "", base.Selectors, base.Fingerprint, SourceManual, 2); err == nil {
		t.Fatal("inconsistent current pointer was accepted")
	}
}

func TestWorkflowAggregatePublishVersionDeepCopiesDefinition(t *testing.T) {
	baseDefinition := WorkflowDefinition{Steps: []WorkflowStep{{ID: "step-v1", DisplayName: "等待", Kind: StepWait, WaitKind: "sleep", WaitMS: 1}}}
	base := WorkflowVersion{ID: "workflow-v1", WorkflowID: "workflow", VersionNumber: 1, Definition: baseDefinition, CreatedAt: 1}
	aggregate := WorkflowAggregate{Workflow: Workflow{ID: "workflow", DisplayName: "结账", Properties: Properties{},
		CurrentVersionID: base.ID, CreatedAt: 1, UpdatedAt: 1, Revision: 1}, Current: base, Versions: []WorkflowVersion{base}}
	definition := WorkflowDefinition{Steps: []WorkflowStep{{ID: "step-v2", DisplayName: "输入", Kind: StepAction,
		Action: "input", NodeID: "node", NodeVersionID: "node-v1", Values: []string{"one"}}}}

	published, err := aggregate.PublishVersion("workflow-v2", definition, 2)
	if err != nil {
		t.Fatalf("PublishVersion: %v", err)
	}
	definition.Steps[0].Values[0] = "mutated"
	if published.Current.VersionNumber != 2 || published.Current.Definition.Steps[0].Values[0] != "one" {
		t.Fatalf("published workflow aliases editor input: %#v", published.Current)
	}
	if aggregate.Current.ID != "workflow-v1" || aggregate.Current.Definition.Steps[0].ID != "step-v1" {
		t.Fatalf("receiver history was mutated: %#v", aggregate)
	}
}
