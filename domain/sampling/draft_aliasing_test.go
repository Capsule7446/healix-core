package sampling

import (
	"testing"

	"github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

// The existing aliasing test built a draft with no Parameters and no Nodes, so
// the two branches of cloneUnpublishedFlowFragment that handle them never ran
// and it passed while both were shallow. A copy is only proven deep by a
// fixture that actually populates what it copies, so these cases carry a
// populated node and a populated parameter and then reach in and mutate them.
//
// Each case mutates the RETURNED draft and asserts the SOURCE is unchanged.
// That direction matters: an editing API hands back the next version, and the
// caller is entitled to keep the previous one.

func aliasingFixture() UnpublishedFlowFragment {
	return UnpublishedFlowFragment{
		ID:          "fragment-1",
		SessionID:   "session-1",
		DisplayName: "Fragment",
		Lifecycle:   SamplingLifecycleRecording,
		Steps: []automation.FlowFragmentStep{
			{ID: "step-1", DisplayName: "Submit", Kind: automation.StepAction, ElementTargetID: "node-1"},
		},
		Parameters: []automation.ParameterDefinition{{
			Name:        "region",
			DisplayName: "Region",
			Type:        parameter.SingleSelect,
			Options:     []string{"north", "south"},
			Default:     parameter.PresentValue(parameter.SingleSelectValue("north")),
		}},
		Nodes: []UnpublishedElementTarget{{
			ID:          "node-1",
			DisplayName: "Submit",
			PageURL:     "https://example.test/form",
			Origin:      "https://example.test",
			Selectors:   []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#submit", Priority: 1}},
			Fingerprint: fingerprint.Fingerprint{
				Tag:        "button",
				Attributes: map[string]string{"id": "submit"},
				Path:       []string{"html", "body", "form", "button"},
				Framework:  fingerprint.FrameworkStack{{Kind: fingerprint.FrameworkReact, Confidence: 0.9}},
			},
			StepIDs:        []string{"step-1"},
			ResolutionMode: ResolutionModeCreate,
		}},
	}
}

func TestDraftEditingNeverAliasesNodeFingerprintWithItsSource(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*UnpublishedFlowFragment)
		read   func(UnpublishedFlowFragment) string
		want   string
	}{
		{
			name:   "fingerprint attributes",
			mutate: func(v *UnpublishedFlowFragment) { v.Nodes[0].Fingerprint.Attributes["id"] = "mutated" },
			read:   func(v UnpublishedFlowFragment) string { return v.Nodes[0].Fingerprint.Attributes["id"] },
			want:   "submit",
		},
		{
			name:   "fingerprint path",
			mutate: func(v *UnpublishedFlowFragment) { v.Nodes[0].Fingerprint.Path[0] = "mutated" },
			read:   func(v UnpublishedFlowFragment) string { return v.Nodes[0].Fingerprint.Path[0] },
			want:   "html",
		},
		{
			name:   "fingerprint framework",
			mutate: func(v *UnpublishedFlowFragment) { v.Nodes[0].Fingerprint.Framework[0].Kind = "MUTATED" },
			read:   func(v UnpublishedFlowFragment) string { return string(v.Nodes[0].Fingerprint.Framework[0].Kind) },
			want:   string(fingerprint.FrameworkReact),
		},
		{
			name:   "parameter options",
			mutate: func(v *UnpublishedFlowFragment) { v.Parameters[0].Options[0] = "mutated" },
			read:   func(v UnpublishedFlowFragment) string { return v.Parameters[0].Options[0] },
			want:   "north",
		},
		{
			name:   "node selectors",
			mutate: func(v *UnpublishedFlowFragment) { v.Nodes[0].Selectors[0].Value = "#mutated" },
			read:   func(v UnpublishedFlowFragment) string { return v.Nodes[0].Selectors[0].Value },
			want:   "#submit",
		},
		{
			name:   "node step ids",
			mutate: func(v *UnpublishedFlowFragment) { v.Nodes[0].StepIDs[0] = "mutated" },
			read:   func(v UnpublishedFlowFragment) string { return v.Nodes[0].StepIDs[0] },
			want:   "step-1",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := aliasingFixture()
			edited := cloneUnpublishedFlowFragment(source)
			test.mutate(&edited)
			if got := test.read(source); got != test.want {
				t.Fatalf("editing the copy changed the source: %s = %q, want %q", test.name, got, test.want)
			}
		})
	}
}

// Same property through the exported editing API rather than the private clone,
// so the boundary a Host actually calls is the one under test.
func TestUpdateUnpublishedFlowFragmentStepNeverAliasesNodeContent(t *testing.T) {
	source := aliasingFixture()
	edited, err := UpdateUnpublishedFlowFragmentStep(source, automation.FlowFragmentStep{
		ID: "step-1", DisplayName: "Submit again", Kind: automation.StepAction, ElementTargetID: "node-1",
	})
	if err != nil {
		t.Fatalf("update step: %v", err)
	}
	edited.Nodes[0].Fingerprint.Attributes["id"] = "mutated"
	edited.Nodes[0].Fingerprint.Framework[0].Kind = "MUTATED"
	edited.Parameters[0].Options[0] = "mutated"
	if got := source.Nodes[0].Fingerprint.Attributes["id"]; got != "submit" {
		t.Errorf("fingerprint attributes aliased: %q", got)
	}
	if got := string(source.Nodes[0].Fingerprint.Framework[0].Kind); got != string(fingerprint.FrameworkReact) {
		t.Errorf("fingerprint framework aliased: %q", got)
	}
	if got := source.Parameters[0].Options[0]; got != "north" {
		t.Errorf("parameter options aliased: %q", got)
	}
}
