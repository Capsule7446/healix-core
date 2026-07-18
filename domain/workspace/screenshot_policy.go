package workspace

import (
	"errors"
	"strings"
)

// ScreenshotPolicy is the immutable business intent frozen with a
// TestTaskRun. Destination is an opaque delivery target: its path semantics,
// encoding profile and validation belong to outer adapters.
type ScreenshotPolicy struct {
	Enabled     bool
	Destination string
}

func NewScreenshotPolicy(enabled bool, destination string) ScreenshotPolicy {
	return ScreenshotPolicy{Enabled: enabled, Destination: strings.TrimSpace(destination)}
}

// NormalizeScreenshotPolicy keeps historical snapshots that predate this
// capability readable. Zero values therefore mean the V1 disabled default.
func NormalizeScreenshotPolicy(policy ScreenshotPolicy) ScreenshotPolicy {
	policy.Destination = strings.TrimSpace(policy.Destination)
	return policy
}

func (p ScreenshotPolicy) Validate() error {
	p = NormalizeScreenshotPolicy(p)
	if p.Enabled && p.Destination == "" {
		return errors.New("enabled screenshot policy requires a destination")
	}
	return nil
}
