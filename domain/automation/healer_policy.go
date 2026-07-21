package automation

import (
	"errors"
	"fmt"
	"math"
)

const HealerPolicySnapshotVersionV1 = 1

// HealerWeightsSnapshotV1 是执行域的修复器权重的工作区端 ACL 表示形式。将此快照保留在本地可防止工作区有界上下文导入域/修复，同时仍允许排队运行冻结其确定性策略。
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

// NormalizeHealerPolicySnapshotV1 将缺失的策略前 JSON 值映射到 V1 默认值。一旦版本存在，每个提供的值都是权威的；特别是，零权重不会被默认值取代。
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
