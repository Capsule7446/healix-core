package fingerprint

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fault"
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
	for index, detector := range detectors {
		if isNilDetector(detector) {
			return nil, frameworkStackInvalidError([]fault.Violation{
				mustViolation(fault.CodeFieldRequired, fmt.Sprintf("detectors.%d", index), "framework detector is required"),
			})
		}
		matches, err := detector.Detect(ctx, observation)
		if err != nil {
			return nil, frameworkDetectorFailedError(err)
		}
		stack = append(stack, matchInfos(matches)...)
	}
	stack = mergeFrameworkStack(SortFrameworkStack(stack))
	// stack.Validate already returns a classified fault; re-wrapping it here would
	// force the host to unwrap recursively before it could classify the failure.
	if err := stack.Validate(); err != nil {
		return nil, err
	}
	return stack, nil
}

func isNilDetector(detector FrameworkDetector) bool {
	if detector == nil {
		return true
	}
	value := reflect.ValueOf(detector)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
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
