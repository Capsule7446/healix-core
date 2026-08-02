package fingerprint

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fault"
)

type FrameworkKind string

const (
	FrameworkReact   FrameworkKind = "react"
	FrameworkVue     FrameworkKind = "vue"
	FrameworkAngular FrameworkKind = "angular"
	FrameworkSvelte  FrameworkKind = "svelte"
	FrameworkSolid   FrameworkKind = "solid"
	FrameworkPreact  FrameworkKind = "preact"
	FrameworkUnknown FrameworkKind = "unknown"
)

type FrameworkEvidenceKind string

const (
	EvidenceScriptLink FrameworkEvidenceKind = "script_link"
	EvidenceGlobal     FrameworkEvidenceKind = "global_marker"
	EvidenceRootMarker FrameworkEvidenceKind = "root_marker"
	EvidenceHydration  FrameworkEvidenceKind = "hydration_marker"
)

type FrameworkInfo struct {
	Kind       FrameworkKind
	Version    string
	Confidence float64
	Evidence   FrameworkEvidenceKind
}

func (k FrameworkKind) isSupported() bool {
	switch k {
	case FrameworkReact, FrameworkVue, FrameworkAngular, FrameworkSvelte, FrameworkSolid, FrameworkPreact, FrameworkUnknown:
		return true
	default:
		return false
	}
}

func (e FrameworkEvidenceKind) isSupported() bool {
	switch e {
	case "", EvidenceScriptLink, EvidenceGlobal, EvidenceRootMarker, EvidenceHydration:
		return true
	default:
		return false
	}
}

func (f FrameworkInfo) Validate() error {
	violations := f.appendViolations(nil, "")
	if len(violations) != 0 {
		return frameworkStackInvalidError(violations)
	}
	return nil
}

// appendViolations degrades every framework failure into a violation of the
// aggregate that owns this info, so a framework fault is never nested inside
// another fault. Kinds, evidence kinds, and versions stay out of public text
// even though they are closed sets: an unsupported value is by definition not
// from the closed set, so echoing it would echo arbitrary caller input.
func (f FrameworkInfo) appendViolations(violations []fault.Violation, prefix string) []fault.Violation {
	if !f.Kind.isSupported() {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, joinField(prefix, "kind"), "framework kind is not supported"))
	}
	if math.IsNaN(f.Confidence) || math.IsInf(f.Confidence, 0) || f.Confidence < 0 || f.Confidence > 1 {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, joinField(prefix, "confidence"), "framework confidence must be within the inclusive range from zero through one"))
	}
	if strings.TrimSpace(f.Version) != "" && strings.ContainsAny(f.Version, "\r\n") {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, joinField(prefix, "version"), "framework version must not contain a line break"))
	}
	if !f.Evidence.isSupported() {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, joinField(prefix, "evidence"), "framework evidence kind is not supported"))
	}
	return violations
}

type FrameworkStack []FrameworkInfo

func (s FrameworkStack) Validate() error {
	violations := s.appendViolations(nil, "frameworks")
	if len(violations) != 0 {
		return frameworkStackInvalidError(violations)
	}
	return nil
}

// appendViolations walks the stack in slice order so violation order is
// deterministic; the duplicate set is only ever point-queried. prefix must be a
// non-empty logical path, because an indexed element path cannot start with a digit.
func (s FrameworkStack) appendViolations(violations []fault.Violation, prefix string) []fault.Violation {
	seen := make(map[FrameworkKind]struct{}, len(s))
	for index, info := range s {
		element := fmt.Sprintf("%s.%d", prefix, index)
		violations = info.appendViolations(violations, element)
		if _, duplicate := seen[info.Kind]; duplicate {
			violations = append(violations, mustViolation(fault.CodeFieldDuplicate, joinField(element, "kind"), "framework kind is duplicated"))
		}
		seen[info.Kind] = struct{}{}
	}
	return violations
}

func (s FrameworkStack) Clone() FrameworkStack { return append(FrameworkStack(nil), s...) }

func SortFrameworkStack(stack FrameworkStack) FrameworkStack {
	out := stack.Clone()
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Confidence != out[j].Confidence {
			return out[i].Confidence > out[j].Confidence
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Version < out[j].Version
	})
	return out
}
