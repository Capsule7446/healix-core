package workspace

import (
	"math"
	"testing"
)

func TestHealerPolicySnapshotDefaultsAndValidation(t *testing.T) {
	policy := NormalizeHealerPolicySnapshotV1(HealerPolicySnapshotV1{})
	if policy != DefaultHealerPolicySnapshotV1() {
		t.Fatalf("normalized policy = %+v", policy)
	}
	if err := policy.Validate(); err != nil {
		t.Fatalf("default policy: %v", err)
	}
}

func TestHealerPolicySnapshotRejectsInvalidValues(t *testing.T) {
	for name, mutate := range map[string]func(*HealerPolicySnapshotV1){
		"unknown version": func(policy *HealerPolicySnapshotV1) { policy.Version++ },
		"equal caps":      func(policy *HealerPolicySnapshotV1) { policy.ReviewCap = policy.AppliedCap },
		"nan cap":         func(policy *HealerPolicySnapshotV1) { policy.ReviewCap = math.NaN() },
		"negative weight": func(policy *HealerPolicySnapshotV1) { policy.Weights.Text = -1 },
		"zero weights":    func(policy *HealerPolicySnapshotV1) { policy.Weights = HealerWeightsSnapshotV1{} },
	} {
		policy := DefaultHealerPolicySnapshotV1()
		mutate(&policy)
		if err := policy.Validate(); err == nil {
			t.Fatalf("%s policy unexpectedly valid", name)
		}
	}
}
