package sampling

import (
	"reflect"
	"testing"

	"github.com/Capsule7446/healix-core/domain/automation"
)

func TestRewriteTemporaryNodeReferencesRecursesWithoutMutatingInput(t *testing.T) {
	steps := []automation.FlowFragmentStep{
		{ID: "root", DisplayName: "root", Kind: automation.StepAction, ElementTargetID: "temp-a"},
		{ID: "repeat", DisplayName: "repeat", Kind: automation.StepRepeat, Children: []automation.FlowFragmentStep{{ID: "child", DisplayName: "child", Kind: automation.StepAction, ElementTargetID: "temp-a"}}},
		{ID: "group", DisplayName: "group", Kind: automation.StepValidationGroup, ValidationGroup: &automation.ValidationGroup{Branches: []automation.ValidationBranch{{ID: "branch", Name: "branch", Steps: []automation.FlowFragmentStep{{ID: "validation", DisplayName: "validation", Kind: automation.StepValidation, ElementTargetID: "temp-b"}}}}}},
	}
	original := cloneSamplingSteps(steps)
	mappings := []automation.SamplingNodeMapping{
		{TemporaryElementTargetID: "temp-a", ElementTargetID: "node-a", ElementTargetVersionID: "node-a-v2"},
		{TemporaryElementTargetID: "temp-b", ElementTargetID: "node-b", ElementTargetVersionID: "node-b-v1"},
	}
	got, err := RewriteTemporaryNodeReferences(steps, mappings)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].ElementTargetID != "node-a" || got[0].ElementTargetVersionID != "node-a-v2" || got[1].Children[0].ElementTargetID != "node-a" || got[2].ValidationGroup.Branches[0].Steps[0].ElementTargetVersionID != "node-b-v1" {
		t.Fatalf("rewritten steps = %#v", got)
	}
	if !reflect.DeepEqual(steps, original) {
		t.Fatal("rewrite mutated input")
	}
	got[1].Children[0].ID = "changed"
	if steps[1].Children[0].ID != "child" {
		t.Fatal("rewrite output aliases input")
	}
}

func TestRewriteTemporaryNodeReferencesRequiresExactMappingSet(t *testing.T) {
	steps := []automation.FlowFragmentStep{{ID: "step", DisplayName: "step", Kind: automation.StepAction, ElementTargetID: "temp-a"}}
	for _, test := range []struct {
		name     string
		mappings []automation.SamplingNodeMapping
	}{
		{"missing", nil},
		{"duplicate", []automation.SamplingNodeMapping{{TemporaryElementTargetID: "temp-a", ElementTargetID: "node", ElementTargetVersionID: "version"}, {TemporaryElementTargetID: "temp-a", ElementTargetID: "other", ElementTargetVersionID: "other-version"}}},
		{"extra", []automation.SamplingNodeMapping{{TemporaryElementTargetID: "temp-a", ElementTargetID: "node", ElementTargetVersionID: "version"}, {TemporaryElementTargetID: "temp-b", ElementTargetID: "other", ElementTargetVersionID: "other-version"}}},
		{"blank formal identity", []automation.SamplingNodeMapping{{TemporaryElementTargetID: "temp-a"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := RewriteTemporaryNodeReferences(steps, test.mappings); err == nil {
				t.Fatal("invalid mapping set was accepted")
			}
		})
	}
}

func TestRewriteTemporaryNodeReferencesAllowsStepsWithoutNodes(t *testing.T) {
	steps := []automation.FlowFragmentStep{{ID: "wait", DisplayName: "wait", Kind: automation.StepWait}}
	got, err := RewriteTemporaryNodeReferences(steps, nil)
	if err != nil || !reflect.DeepEqual(got, steps) {
		t.Fatalf("node-free rewrite = %#v, %v", got, err)
	}
}
