package engine

import (
	"testing"

	"github.com/Capsule7446/healix-core/domain/execution"
)

func TestCompilePlanMapsNestedValidationAndReferences(t *testing.T) {
	plan := execution.Plan{RunID: "run", RootVersionID: "root-v1", Workflows: []execution.WorkflowSnapshot{{WorkflowID: "root", VersionID: "root-v1", DisplayName: "root", VersionNumber: 1, Steps: []execution.Step{{ID: "validate", DisplayName: "visible", Kind: execution.ValidationStep, NodeID: "node", NodeVersionID: "node-v1", Validation: &execution.Validation{Kind: "visible", MaxWaitMS: 1000, StabilityMS: 100}}}}}, Nodes: []execution.NodeSnapshot{{NodeID: "node", VersionID: "node-v1", DisplayName: "button"}}}
	if _, err := CompilePlan(plan); err == nil {
		t.Fatal("expected invalid empty node fingerprint to fail compiler preflight")
	}
}

func TestCompilePlanRequiresRunIdentity(t *testing.T) {
	if _, err := CompilePlan(execution.Plan{RootVersionID: "root"}); err == nil {
		t.Fatal("expected run identity validation")
	}
}
