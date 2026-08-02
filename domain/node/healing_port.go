package node

import (
	"context"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/heal"
)

// HealingPort is the execution-facing seam for selector recovery.
// Node owns when recovery is requested; heal owns decision semantics.
type HealingPort interface {
	Recover(context.Context, fingerprint.ElementTargetSpec, heal.DOMSnapshot) (heal.Decision, error)
}

type healerPortAdapter struct{ healer heal.Healer }

func (a healerPortAdapter) Recover(ctx context.Context, target fingerprint.ElementTargetSpec, snapshot heal.DOMSnapshot) (heal.Decision, error) {
	return a.healer.Heal(ctx, target, snapshot)
}

func adaptHealer(healer heal.Healer) HealingPort {
	if healer == nil {
		return nil
	}
	return healerPortAdapter{healer: healer}
}
