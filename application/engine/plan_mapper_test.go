package engine

import (
	"testing"

	"github.com/Capsule7446/healix-core/domain/execution"
)

func TestCompilePlanMapsNestedValidationAndReferences(t *testing.T) {
	plan := execution.Draft{RunID: "run", Entries: []execution.WorkflowEntry{{ExecutionID: "execution-entry", TestTaskItemID: "task-item", FlowFragmentID: "root", WorkflowVersionID: "root-v1"}}, Workflows: []execution.WorkflowSnapshot{{FlowFragmentID: "root", VersionID: "root-v1", DisplayName: "root", VersionNumber: 1, Steps: []execution.Step{{ID: "validate", DisplayName: "visible", Kind: execution.ValidationStep, NodeID: "node", NodeVersionID: "node-v1", Validation: &execution.Validation{Kind: "visible", MaxWaitMS: 1000, StabilityMS: 100}}}}}, Nodes: []execution.NodeSnapshot{{NodeID: "node", VersionID: "node-v1", DisplayName: "button"}}}
	if _, err := compileDraft(plan); err == nil {
		t.Fatal("expected invalid empty node fingerprint to fail compiler preflight")
	}
}

func TestCompilePlanRequiresRunIdentity(t *testing.T) {
	if _, err := compileDraft(execution.Draft{Entries: []execution.WorkflowEntry{{ExecutionID: "execution-entry", TestTaskItemID: "task-item", FlowFragmentID: "root", WorkflowVersionID: "root"}}}); err == nil {
		t.Fatal("expected run identity validation")
	}
}
