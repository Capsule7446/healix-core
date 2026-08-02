package automation

import (
	"testing"

	domain "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

// The publication request digest hashes the marshalled payload, and parameter
// defaults and bindings hang off that payload. While the parameter types
// encoded as {}, two publications differing only in a default hashed the same,
// so republishing an edit under one command id looked like a replay and the
// edit was returned as already-applied without ever being written.
//
// These assert the property the digest exists to provide: a payload the caller
// can tell apart must hash apart.

func publicationWithDefault(t *testing.T, value string) domain.SamplingPublication {
	t.Helper()
	publication := samplingPublicationFixture(t)
	publication.FlowFragment.Current.Definition.Parameters = []domain.ParameterDefinition{{
		Name: "region", DisplayName: "Region", Type: parameter.Text,
		Default: parameter.PresentValue(parameter.TextValue(value)),
	}}
	publication.FlowFragment.Versions[0] = publication.FlowFragment.Current
	return publication
}

func publicationWithBinding(t *testing.T, binding parameter.Binding) domain.SamplingPublication {
	t.Helper()
	publication := publicationWithDefault(t, "north")
	publication.FlowFragment.Current.Definition.Steps = append(
		publication.FlowFragment.Current.Definition.Steps,
		domain.FlowFragmentStep{
			ID: "call", DisplayName: "Call", Kind: domain.StepFlowFragmentRef,
			Reference: &domain.FlowFragmentReference{
				FlowFragmentID: "child", WorkflowVersionID: "child-v1",
				ParameterBindings: map[string]parameter.Binding{"region": binding},
			},
		})
	publication.FlowFragment.Versions[0] = publication.FlowFragment.Current
	return publication
}

func digestOf(t *testing.T, publication domain.SamplingPublication) string {
	t.Helper()
	digest, err := SamplingPublicationRequestDigest(SamplingPublicationCommand{
		PublicationID: "publication-1", Publication: publication,
	})
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	return digest
}

func TestPublicationDigestSeparatesParameterDefaults(t *testing.T) {
	alpha := digestOf(t, publicationWithDefault(t, "alpha"))
	omega := digestOf(t, publicationWithDefault(t, "omega"))
	if alpha == omega {
		t.Fatalf("two different parameter defaults share digest %s; the default never reached the hash", alpha)
	}
}

func TestPublicationDigestSeparatesParameterBindings(t *testing.T) {
	production := digestOf(t, publicationWithBinding(t, parameter.LiteralBinding(parameter.TextValue("production"))))
	staging := digestOf(t, publicationWithBinding(t, parameter.LiteralBinding(parameter.TextValue("staging"))))
	parent := digestOf(t, publicationWithBinding(t, parameter.ParentReferenceBinding("production")))

	for _, pair := range []struct {
		name string
		a, b string
	}{
		{"literal production vs literal staging", production, staging},
		{"literal production vs parent reference", production, parent},
		{"literal staging vs parent reference", staging, parent},
	} {
		if pair.a == pair.b {
			t.Errorf("%s share digest %s", pair.name, pair.a)
		}
	}
}

// The same payload must hash the same every time, or idempotency is broken in
// the other direction: a genuine replay would be treated as a new request.
//
// The fixture carries a multi-entry map on purpose. The canonical walk sorts map
// keys, and a payload with no map of two or more entries cannot tell whether it
// still does — the first version of this test had none, and deleting the sort
// left the whole suite green.
func TestPublicationDigestIsStableAcrossRuns(t *testing.T) {
	build := func() domain.SamplingPublication {
		publication := publicationWithDefault(t, "alpha")
		publication.FlowFragment.FlowFragment.Properties = domain.Properties{
			"zulu": "z", "alpha": "a", "mike": "m", "bravo": "b",
		}
		publication.FlowFragment.Versions[0] = publication.FlowFragment.Current
		return publication
	}
	first := digestOf(t, build())
	for run := 0; run < 50; run++ {
		if again := digestOf(t, build()); again != first {
			t.Fatalf("run %d produced %s, run 0 produced %s; a map in the payload is reaching the digest in iteration order",
				run, again, first)
		}
	}
}

// publicationWithTypedDefault carries an arbitrary typed default so the digest
// can be probed per parameter type. publicationWithDefault above only ever
// builds Text values, which is how Boolean and MultiSelect came to be the two
// unexecuted arms of encodeCanonicalParameterValue. Select types carry their
// option list because the aggregate rejects a select parameter without one.
func publicationWithTypedDefault(t *testing.T, kind parameter.Type, value parameter.Value, options ...string) domain.SamplingPublication {
	t.Helper()
	publication := samplingPublicationFixture(t)
	publication.FlowFragment.Current.Definition.Parameters = []domain.ParameterDefinition{{
		Name: "region", DisplayName: "Region", Type: kind,
		Options: options,
		Default: parameter.PresentValue(value),
	}}
	publication.FlowFragment.Versions[0] = publication.FlowFragment.Current
	return publication
}

var digestSelectOptions = []string{"north", "south", "east", "ab", "a", "b"}

// TestPublicationDigestSeparatesEveryParameterType is the collision test. An
// unencoded arm does not fail loudly — it makes two distinguishable payloads
// hash identically, and the second publication is then returned as an
// already-applied replay while the edit is silently dropped. Each pair below
// differs only in the value the arm under test is responsible for encoding.
func TestPublicationDigestSeparatesEveryParameterType(t *testing.T) {
	number := func(value string) parameter.Value {
		t.Helper()
		typed, err := parameter.NewNumberValue(value)
		if err != nil {
			t.Fatal(err)
		}
		return typed
	}
	tests := []struct {
		name    string
		kind    parameter.Type
		options []string
		a, b    parameter.Value
	}{
		{name: "text", kind: parameter.Text, a: parameter.TextValue("alpha"), b: parameter.TextValue("omega")},
		{name: "number", kind: parameter.Number, a: number("1"), b: number("2")},
		{name: "boolean", kind: parameter.Boolean, a: parameter.BooleanValue(true), b: parameter.BooleanValue(false)},
		{name: "single select", kind: parameter.SingleSelect, options: digestSelectOptions,
			a: parameter.SingleSelectValue("north"), b: parameter.SingleSelectValue("south")},
		{name: "multi select membership", kind: parameter.MultiSelect, options: digestSelectOptions,
			a: parameter.MultiSelectValue([]string{"north"}), b: parameter.MultiSelectValue([]string{"north", "south"})},
		// Order is part of a multi-select value, so a reordering is a
		// different payload and must not replay as the same request.
		{name: "multi select order", kind: parameter.MultiSelect, options: digestSelectOptions,
			a: parameter.MultiSelectValue([]string{"north", "south"}), b: parameter.MultiSelectValue([]string{"south", "north"})},
		// Concatenation must not be able to forge equality: ["ab"] and
		// ["a","b"] hash alike unless each item carries a length prefix.
		{name: "multi select concatenation", kind: parameter.MultiSelect, options: digestSelectOptions,
			a: parameter.MultiSelectValue([]string{"ab"}), b: parameter.MultiSelectValue([]string{"a", "b"})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			first := digestOf(t, publicationWithTypedDefault(t, test.kind, test.a, test.options...))
			second := digestOf(t, publicationWithTypedDefault(t, test.kind, test.b, test.options...))

			if first == second {
				t.Fatalf("two distinguishable %s defaults share digest %s", test.kind, first)
			}
		})
	}
}

// TestPublicationDigestTreatsCanonicallyEqualNumbersAsOneRequest is the other
// side of the collision rule. NewNumberValue canonicalises 1, 1.0 and 1.00 to
// the same value, so they are the same payload and must replay as one request
// — separating them here would turn a genuine retry into a second write.
func TestPublicationDigestTreatsCanonicallyEqualNumbersAsOneRequest(t *testing.T) {
	number := func(value string) parameter.Value {
		t.Helper()
		typed, err := parameter.NewNumberValue(value)
		if err != nil {
			t.Fatal(err)
		}
		return typed
	}
	canonical := digestOf(t, publicationWithTypedDefault(t, parameter.Number, number("1")))
	for _, spelling := range []string{"1.0", "1.00", "+1"} {
		if again := digestOf(t, publicationWithTypedDefault(t, parameter.Number, number(spelling))); again != canonical {
			t.Fatalf("%q hashed to %s, want the canonical %s", spelling, again, canonical)
		}
	}
}

// TestPublicationDigestSeparatesParameterTypes covers the type tag itself: two
// values whose payloads render alike must still differ if their types do.
func TestPublicationDigestSeparatesParameterTypes(t *testing.T) {
	text := digestOf(t, publicationWithTypedDefault(t, parameter.Text, parameter.TextValue("north")))
	single := digestOf(t, publicationWithTypedDefault(t, parameter.SingleSelect, parameter.SingleSelectValue("north"), digestSelectOptions...))

	if text == single {
		t.Fatalf("a Text and a SingleSelect default with the same text share digest %s", text)
	}
}

// TestPublicationDigestIsStableForEveryParameterType pairs the separation
// tests: an encoder that hashed a map or a pointer would separate payloads
// correctly and still break replay detection.
func TestPublicationDigestIsStableForEveryParameterType(t *testing.T) {
	tests := []struct {
		kind    parameter.Type
		value   parameter.Value
		options []string
	}{
		{kind: parameter.Text, value: parameter.TextValue("north")},
		{kind: parameter.Boolean, value: parameter.BooleanValue(true)},
		{kind: parameter.SingleSelect, value: parameter.SingleSelectValue("north"), options: digestSelectOptions},
		{kind: parameter.MultiSelect, value: parameter.MultiSelectValue([]string{"north", "south", "east"}), options: digestSelectOptions},
	}
	for _, test := range tests {
		t.Run(string(test.kind), func(t *testing.T) {
			first := digestOf(t, publicationWithTypedDefault(t, test.kind, test.value, test.options...))
			for run := 0; run < 20; run++ {
				if again := digestOf(t, publicationWithTypedDefault(t, test.kind, test.value, test.options...)); again != first {
					t.Fatalf("run %d produced %s, run 0 produced %s", run, again, first)
				}
			}
		})
	}
}
