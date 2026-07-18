package workspace

import (
	"errors"
	"fmt"
	"math"
)

const HealerPolicySnapshotVersionV1 = 1

// HealerWeightsSnapshotV1 is the workspace-side ACL representation of the
// execution domain's healer weights. Keeping this snapshot local prevents the
// workspace bounded context from importing domain/heal while still allowing a
// queued run to freeze its deterministic policy.
type HealerWeightsSnapshotV1 struct {
	Tag       float64
	ID        float64
	RoleName  float64
	Class     float64
	Attrs     float64
	Text      float64
	Index     float64
	Neighbor  float64
	LabelText float64
	Container float64
}

type HealerPolicySnapshotV1 struct {
	Version    int
	ReviewCap  float64
	AppliedCap float64
	Weights    HealerWeightsSnapshotV1
}

func DefaultHealerPolicySnapshotV1() HealerPolicySnapshotV1 {
	return HealerPolicySnapshotV1{
		Version:    HealerPolicySnapshotVersionV1,
		ReviewCap:  0.60,
		AppliedCap: 0.85,
		Weights: HealerWeightsSnapshotV1{
			Tag: 0.15, ID: 0.20, RoleName: 0.20, Class: 0.10, Attrs: 0.10,
			Text: 0.10, Index: 0.05, Neighbor: 0.10, LabelText: 0.15, Container: 0.10,
		},
	}
}

// NormalizeHealerPolicySnapshotV1 maps a missing pre-policy JSON value to the
// V1 defaults. Once Version is present, every supplied value is authoritative;
// in particular, a zero weight is not replaced by its default.
func NormalizeHealerPolicySnapshotV1(policy HealerPolicySnapshotV1) HealerPolicySnapshotV1 {
	if policy.Version == 0 {
		return DefaultHealerPolicySnapshotV1()
	}
	return policy
}

func (p HealerPolicySnapshotV1) Validate() error {
	p = NormalizeHealerPolicySnapshotV1(p)
	if p.Version != HealerPolicySnapshotVersionV1 {
		return fmt.Errorf("unsupported healer policy version %d", p.Version)
	}
	if !finiteUnit(p.ReviewCap) || !finiteUnit(p.AppliedCap) {
		return errors.New("healer thresholds must be finite and within [0,1]")
	}
	if p.ReviewCap >= p.AppliedCap {
		return errors.New("healer review cap must be lower than applied cap")
	}
	weights := []float64{p.Weights.Tag, p.Weights.ID, p.Weights.RoleName, p.Weights.Class,
		p.Weights.Attrs, p.Weights.Text, p.Weights.Index, p.Weights.Neighbor,
		p.Weights.LabelText, p.Weights.Container}
	total := 0.0
	for _, weight := range weights {
		if math.IsNaN(weight) || math.IsInf(weight, 0) || weight < 0 {
			return errors.New("healer weights must be finite and non-negative")
		}
		total += weight
	}
	if math.IsInf(total, 0) || total == 0 {
		return errors.New("at least one healer weight must be positive")
	}
	return nil
}

func finiteUnit(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}
