package heal

import (
	"testing"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

func TestEvidenceForUsesStableDimensions(t *testing.T) {
	target := fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{"id": "submit"}, ARIA: fingerprint.ARIA{Role: "button"}, FormID: "checkout"}
	candidate := target
	evidence := EvidenceFor(target, candidate)
	if len(evidence) != 4 {
		t.Fatalf("evidence count=%d", len(evidence))
	}
	for _, item := range evidence {
		if !item.Matched || item.Score != 1 {
			t.Fatalf("evidence=%+v", evidence)
		}
	}
}
