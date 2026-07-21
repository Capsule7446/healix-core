package heal

import (
	"testing"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

func TestFrameworkWeightIsOptional(t *testing.T) {
	target := fingerprint.Fingerprint{Tag: "button", Framework: fingerprint.FrameworkStack{{Kind: fingerprint.FrameworkReact, Version: "18"}}}
	match := target
	mismatch := fingerprint.Fingerprint{Tag: "button", Framework: fingerprint.FrameworkStack{{Kind: fingerprint.FrameworkVue, Version: "3"}}}
	weights := DefaultWeights()
	if score(weights, target, match) != score(weights, target, mismatch) {
		t.Fatal("default zero framework weight changed legacy score")
	}
	weights.Framework = 1
	if score(weights, target, match) <= score(weights, target, mismatch) {
		t.Fatal("framework weight did not favor matching framework")
	}
}
