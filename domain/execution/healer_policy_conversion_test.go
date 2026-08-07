package execution

import (
	"math"
	"reflect"
	"testing"

	"github.com/Capsule7446/healix-core/domain/heal"
)

// TestHealPolicyCarriesEveryScoredDimension is the regression this conversion
// exists for. Framework was a scorer dimension with no snapshot field, so it
// could only be set outside the digest: two runs sealing to the same digest
// could score differently. The assertion is structural rather than a list of
// hand-copied numbers -- it walks heal.Weights by reflection, so a dimension
// added later fails here until the conversion carries it.
func TestHealPolicyCarriesEveryScoredDimension(t *testing.T) {
	snapshot := DefaultHealerPolicySnapshot()
	distinct := map[string]float64{
		"Tag": .11, "ID": .12, "RoleName": .13, "Class": .14, "Attrs": .15,
		"Text": .16, "Index": .17, "Neighbor": .18, "LabelText": .19,
		"Container": .21, "Framework": .22,
	}
	source := reflect.ValueOf(&snapshot.Weights).Elem()
	for name, value := range distinct {
		field := source.FieldByName(name)
		if !field.IsValid() {
			t.Fatalf("HealerWeightsSnapshot is missing the %s dimension", name)
		}
		field.SetFloat(value)
	}

	policy, err := snapshot.HealPolicy()
	if err != nil {
		t.Fatalf("HealPolicy() = %v", err)
	}

	scored := reflect.TypeOf(heal.Weights{})
	if scored.NumField() != len(distinct) {
		t.Fatalf("heal.Weights has %d dimensions, the fixture covers %d", scored.NumField(), len(distinct))
	}
	got := reflect.ValueOf(policy.Weights)
	for index := 0; index < scored.NumField(); index++ {
		name := scored.Field(index).Name
		want, covered := distinct[name]
		if !covered {
			t.Fatalf("heal.Weights dimension %s is not carried by HealerWeightsSnapshot", name)
		}
		if got.Field(index).Float() != want {
			t.Errorf("weight %s = %v, want %v", name, got.Field(index).Float(), want)
		}
	}
	if policy.Thresholds.ReviewCap != snapshot.ReviewCap || policy.Thresholds.AppliedCap != snapshot.AppliedCap {
		t.Errorf("thresholds = %+v, want review=%v applied=%v", policy.Thresholds, snapshot.ReviewCap, snapshot.AppliedCap)
	}
	if policy.Version != heal.PolicyVersionV1 {
		t.Errorf("version = %d, want %d", policy.Version, heal.PolicyVersionV1)
	}
}

// TestHealPolicyRejectsPolicyTheScorerCannotUse covers the second reason the
// conversion validates: environment validation and the scorer's own rules are
// not the same rule set, and a healer built from a policy that scores nothing
// fails silently rather than loudly.
func TestHealPolicyRejectsPolicyTheScorerCannotUse(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*HealerPolicySnapshot)
	}{
		{"zero total weight", func(p *HealerPolicySnapshot) { p.Weights = HealerWeightsSnapshot{} }},
		{"inverted thresholds", func(p *HealerPolicySnapshot) { p.ReviewCap, p.AppliedCap = .9, .5 }},
		{"non-finite dimension", func(p *HealerPolicySnapshot) { p.Weights.Framework = math.Inf(1) }},
		{"negative dimension", func(p *HealerPolicySnapshot) { p.Weights.Framework = -1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := DefaultHealerPolicySnapshot()
			test.mutate(&snapshot)
			policy, err := snapshot.HealPolicy()
			if err == nil {
				t.Fatalf("HealPolicy() accepted %s", test.name)
			}
			if !reflect.DeepEqual(policy, heal.PolicyV1{}) {
				t.Fatalf("rejected conversion returned %+v, want the zero policy", policy)
			}
		})
	}
}
