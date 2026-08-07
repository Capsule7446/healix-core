package execution

import (
	"github.com/Capsule7446/healix-core/domain/heal"
)

// HealPolicy converts the frozen healer policy into the shape the scorer
// consumes. Without it a host had to hand-map the snapshot, and the one
// dimension the snapshot did not carry -- Framework -- could only be supplied
// from outside the digest, so two runs sealing to the same snapshot digest
// could score differently. That is the failure the whole snapshot exists to
// prevent, so the mapping belongs here rather than in each host.
//
// heal and execution share the execution bounded context and heal imports
// nothing from here, so this direction is the legal one.
//
// The result is validated by heal's own rules before it is returned: a
// snapshot that passed environment validation still has to satisfy the
// scorer's invariants, and failing here beats configuring a healer that
// silently scores nothing.
func (v HealerPolicySnapshot) HealPolicy() (heal.PolicyV1, error) {
	policy := heal.PolicyV1{
		Version: heal.PolicyVersionV1,
		Weights: heal.Weights{
			Tag:       v.Weights.Tag,
			ID:        v.Weights.ID,
			RoleName:  v.Weights.RoleName,
			Class:     v.Weights.Class,
			Attrs:     v.Weights.Attrs,
			Text:      v.Weights.Text,
			Index:     v.Weights.Index,
			Neighbor:  v.Weights.Neighbor,
			LabelText: v.Weights.LabelText,
			Container: v.Weights.Container,
			Framework: v.Weights.Framework,
		},
		Thresholds: heal.Thresholds{
			AppliedCap: v.AppliedCap,
			ReviewCap:  v.ReviewCap,
		},
	}
	if err := policy.Validate(); err != nil {
		return heal.PolicyV1{}, err
	}
	return policy, nil
}
