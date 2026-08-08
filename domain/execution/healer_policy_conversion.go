package execution

import (
	"github.com/Capsule7446/healix-core/domain/heal"
)

// HealPolicy 将封存的自愈策略转换为评分器消费的形状，并在返回前执行 heal 自身的策略校验。
// 转换结果包含 Framework 权重，确保快照摘要涵盖评分所需的全部维度。
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
