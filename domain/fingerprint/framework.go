package fingerprint

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
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

func (f FrameworkInfo) Validate() error {
	switch f.Kind {
	case FrameworkReact, FrameworkVue, FrameworkAngular, FrameworkSvelte, FrameworkSolid, FrameworkPreact, FrameworkUnknown:
	default:
		return fmt.Errorf("fingerprint: unsupported framework %q", f.Kind)
	}
	if math.IsNaN(f.Confidence) || math.IsInf(f.Confidence, 0) || f.Confidence < 0 || f.Confidence > 1 {
		return errors.New("fingerprint: framework confidence must be within [0,1]")
	}
	if strings.TrimSpace(f.Version) != "" && strings.ContainsAny(f.Version, "\r\n") {
		return errors.New("fingerprint: framework version contains a line break")
	}
	switch f.Evidence {
	case "", EvidenceScriptLink, EvidenceGlobal, EvidenceRootMarker, EvidenceHydration:
	default:
		return fmt.Errorf("fingerprint: unsupported framework evidence %q", f.Evidence)
	}
	return nil
}

type FrameworkStack []FrameworkInfo

func (s FrameworkStack) Validate() error {
	seen := make(map[FrameworkKind]struct{}, len(s))
	for _, info := range s {
		if err := info.Validate(); err != nil {
			return err
		}
		if _, ok := seen[info.Kind]; ok {
			return fmt.Errorf("fingerprint: duplicate framework %q", info.Kind)
		}
		seen[info.Kind] = struct{}{}
	}
	return nil
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
