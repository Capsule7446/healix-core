package fingerprint

import (
	"context"
	"strings"
)

// PageObservation is the safe browser-to-domain projection used for framework detection.
// It intentionally contains no raw DOM, JS object, browser handle, or arbitrary page text.
type PageObservation struct {
	PageURL       string
	ScriptURLs    []string
	GlobalMarkers []string
	RootMarkers   []string
	Hydration     []string
}

type FrameworkMatch struct {
	Info FrameworkInfo
}

type FrameworkDetector interface {
	Detect(context.Context, PageObservation) ([]FrameworkMatch, error)
}

// DetectFrameworks runs detectors and returns a validated, deterministic stack.
func DetectFrameworks(ctx context.Context, observation PageObservation, detectors []FrameworkDetector) (FrameworkStack, error) {
	stack := make(FrameworkStack, 0)
	for _, detector := range detectors {
		matches, err := detector.Detect(ctx, observation)
		if err != nil {
			return nil, err
		}
		stack = append(stack, matchInfos(matches)...)
	}
	stack = mergeFrameworkStack(SortFrameworkStack(stack))
	if err := stack.Validate(); err != nil {
		return nil, err
	}
	return stack, nil
}

func mergeFrameworkStack(stack FrameworkStack) FrameworkStack {
	seen := make(map[FrameworkKind]struct{}, len(stack))
	out := make(FrameworkStack, 0, len(stack))
	for _, info := range stack {
		if _, ok := seen[info.Kind]; ok {
			continue
		}
		seen[info.Kind] = struct{}{}
		out = append(out, info)
	}
	return out
}

func matchInfos(matches []FrameworkMatch) FrameworkStack {
	stack := make(FrameworkStack, 0, len(matches))
	for _, match := range matches {
		match.Info.Version = strings.TrimSpace(match.Info.Version)
		stack = append(stack, match.Info)
	}
	return stack
}
