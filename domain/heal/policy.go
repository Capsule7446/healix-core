package heal

import "fmt"

// PolicyVersionV1 identifies the first persisted/configurable DefaultHealer
// policy shape. The version travels with execution snapshots so later policy
// changes do not make an already-created run irreproducible.
const PolicyVersionV1 = 1

// PolicyV1 is the complete deterministic configuration of DefaultHealer.
// Callers should construct it with DefaultPolicyV1 and replace only the fields
// they intentionally customize.
type PolicyV1 struct {
	Version    int
	Weights    Weights
	Thresholds Thresholds
}

// DefaultPolicyV1 returns a fresh value so callers cannot mutate the defaults
// observed by later runs through a shared pointer.
func DefaultPolicyV1() PolicyV1 {
	return PolicyV1{
		Version:    PolicyVersionV1,
		Weights:    DefaultWeights(),
		Thresholds: DefaultThresholds(),
	}
}

func (p PolicyV1) Validate() error {
	if p.Version != PolicyVersionV1 {
		return fmt.Errorf("heal: unsupported policy version %d", p.Version)
	}
	if err := p.Thresholds.Validate(); err != nil {
		return err
	}
	return p.Weights.Validate()
}
