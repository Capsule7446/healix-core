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
func TestPublicationDigestIsStableAcrossRuns(t *testing.T) {
	first := digestOf(t, publicationWithDefault(t, "alpha"))
	for run := 0; run < 50; run++ {
		if again := digestOf(t, publicationWithDefault(t, "alpha")); again != first {
			t.Fatalf("run %d produced %s, run 0 produced %s", run, again, first)
		}
	}
}
