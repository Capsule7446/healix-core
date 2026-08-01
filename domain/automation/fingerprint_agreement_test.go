package automation

import (
	"testing"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

// ElementTargetAggregate.Validate re-implements the fingerprint rules instead of
// delegating, and the two copies drifted: a negative sibling index and a
// malformed framework stack were rejected the moment an element was sampled but
// accepted on the way into an immutable published version.
//
// Removing the duplication means exporting a violation appender from
// domain/fingerprint, which is a wider change than the gap warrants. What has to
// stay true either way is that the two validators agree: a fingerprint that one
// rejects, the other must reject. This is the assertion that catches the next
// rule added to only one of them.
func TestBothFingerprintValidatorsAgree(t *testing.T) {
	valid := fingerprint.Fingerprint{
		Tag:        "button",
		Attributes: map[string]string{"id": "submit"},
		Path:       []string{"html", "body", "button"},
		Framework:  fingerprint.FrameworkStack{{Kind: fingerprint.FrameworkReact, Confidence: 0.9}},
	}

	cases := map[string]func(*fingerprint.Fingerprint){
		"valid":            nil,
		"blank tag":        func(f *fingerprint.Fingerprint) { f.Tag = "  " },
		"nil attributes":   func(f *fingerprint.Fingerprint) { f.Attributes = nil },
		"negative sibling": func(f *fingerprint.Fingerprint) { f.SiblingIndex = -7 },
		"unknown framework": func(f *fingerprint.Fingerprint) {
			f.Framework = fingerprint.FrameworkStack{{Kind: "NOT_A_FRAMEWORK", Confidence: 0.5}}
		},
		"framework confidence": func(f *fingerprint.Fingerprint) {
			f.Framework = fingerprint.FrameworkStack{{Kind: fingerprint.FrameworkReact, Confidence: 42}}
		},
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			candidate.Attributes = map[string]string{"id": "submit"}
			candidate.Path = append([]string(nil), valid.Path...)
			candidate.Framework = valid.Framework.Clone()
			if mutate != nil {
				mutate(&candidate)
			}

			ownerRejects := candidate.Validate() != nil
			aggregateRejects := aggregateFor(candidate).Validate() != nil

			if ownerRejects != aggregateRejects {
				t.Fatalf("domain/fingerprint rejects=%v but ElementTargetAggregate rejects=%v; the two validators have drifted",
					ownerRejects, aggregateRejects)
			}
		})
	}
}

func aggregateFor(f fingerprint.Fingerprint) ElementTargetAggregate {
	version := ElementTargetVersion{
		ID: "version-1", ElementTargetID: "target-1", VersionNumber: 1,
		PageURL: "https://example.test", Origin: "https://example.test",
		Selectors:   []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "#submit", Priority: 1}},
		Fingerprint: f, Source: SourceSampling, CreatedAt: 1,
	}
	return ElementTargetAggregate{
		ElementTarget: ElementTarget{
			ID: "target-1", DisplayName: "Submit", Properties: Properties{},
			CurrentVersionID: "version-1", CreatedAt: 1, UpdatedAt: 1, Revision: 1,
		},
		Current:  version,
		Versions: []ElementTargetVersion{version},
	}
}
