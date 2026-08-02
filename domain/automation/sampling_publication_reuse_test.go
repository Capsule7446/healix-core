package automation

import (
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

// REUSE used to skip element target validation altogether. That was safe only
// on the mapper path, which validates the loaded history first; Publish accepts
// a caller-built SamplingPublication, so the skip was reachable with content
// nobody had checked.
//
// The projection genuinely cannot satisfy the full aggregate rule — it holds the
// selected historical version as Current while the target still points at the
// live one for the compare-and-swap — so the fix had to validate the selected
// version rather than the aggregate. These pin both halves: bad content is
// rejected, and the legitimate historical projection is still accepted.

func reuseAggregate(mutate func(*ElementTargetVersion)) ElementTargetAggregate {
	selected := ElementTargetVersion{
		ID: "existing-v1", ElementTargetID: "existing", VersionNumber: 1,
		PageURL: "https://example.test/form", Origin: "https://example.test",
		Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#submit", Priority: 1}},
		Fingerprint: fingerprint.Fingerprint{
			Tag: "button", Attributes: map[string]string{"id": "submit"},
			Path: []string{"html", "body", "button"},
		},
		Source: SourceSampling, CreatedAt: 1,
	}
	if mutate != nil {
		mutate(&selected)
	}
	live := selected
	live.ID = "existing-v2"
	live.VersionNumber = 2
	return ElementTargetAggregate{
		// The pointer names the live version, not the selected one. This is the
		// shape REUSE publishes and the reason the aggregate rule cannot apply.
		ElementTarget: ElementTarget{
			ID: "existing", DisplayName: "Submit", Properties: Properties{},
			CurrentVersionID: "existing-v2", CreatedAt: 1, UpdatedAt: 1, Revision: 2,
		},
		Current:  selected,
		Versions: []ElementTargetVersion{selected, live},
	}
}

func reusePublication(t *testing.T, aggregate ElementTargetAggregate) SamplingPublication {
	t.Helper()
	// A valid fragment, so the node check is what the assertion is measuring
	// rather than the fragment failing first.
	fragment, err := NewFlowFragment(
		FlowFragment{ID: "workflow", DisplayName: "FlowFragment", Properties: Properties{}, CreatedAt: 1, UpdatedAt: 1},
		FlowFragmentVersion{ID: "workflow-v1", FlowFragmentID: "workflow", VersionNumber: 1, CreatedAt: 1,
			Definition: FlowFragmentContent{Steps: []FlowFragmentStep{
				{ID: "wait", DisplayName: "Wait", Kind: StepWait, WaitKind: "sleep", WaitMS: 1},
			}}})
	if err != nil {
		t.Fatalf("fragment fixture: %v", err)
	}
	return SamplingPublication{
		Nodes: []SamplingElementTargetPublication{{
			TemporaryElementTargetID: "temporary-node",
			ResolutionMode:           "REUSE",
			Aggregate:                aggregate,
			ExpectedRevision:         aggregate.ElementTarget.Revision,
			ExpectedCurrentVersionID: aggregate.ElementTarget.CurrentVersionID,
		}},
		FlowFragment: fragment,
	}
}

func TestReusePublicationValidatesTheSelectedVersionContent(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ElementTargetVersion)
	}{
		{"no selectors", func(v *ElementTargetVersion) { v.Selectors = nil }},
		{"zero version number", func(v *ElementTargetVersion) { v.VersionNumber = 0 }},
		{"unknown source", func(v *ElementTargetVersion) { v.Source = "NOT_A_SOURCE" }},
		{"missing fingerprint tag", func(v *ElementTargetVersion) { v.Fingerprint.Tag = "" }},
		{"nil fingerprint attributes", func(v *ElementTargetVersion) { v.Fingerprint.Attributes = nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := reusePublication(t, reuseAggregate(test.mutate)).Validate()
			if err == nil {
				t.Fatal("a reuse node published unchecked content")
			}
			if !strings.Contains(err.Error(), "element target") {
				t.Fatalf("rejected for the wrong reason: %v", err)
			}
		})
	}
}

// The counterpart: the legitimate projection, whose current pointer names a
// different version than the selection by design, must still be accepted.
func TestReusePublicationAcceptsAHistoricalSelection(t *testing.T) {
	aggregate := reuseAggregate(nil)
	if err := aggregate.Current.ValidateFor(aggregate.ElementTarget); err != nil {
		t.Fatalf("the selected historical version was rejected: %v", err)
	}
	if err := aggregate.Validate(); err == nil {
		t.Fatal("the full aggregate rule was expected not to hold for this projection; " +
			"if it now does, the reuse path no longer needs its own validator")
	}
}
