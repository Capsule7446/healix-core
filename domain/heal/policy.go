package heal

import "fmt"

// PolicyVersionV1 标识第一个持久/可配置的 DefaultHealer 策略形状。该版本随执行快照一起传播，因此以后的策略更改不会使已创建的运行变得不可重现。
const PolicyVersionV1 = 1

// PolicyV1 是 DefaultHealer 的完整确定性配置。调用者应使用 DefaultPolicyV1 构建它，并仅替换他们有意自定义的字段。
type PolicyV1 struct {
	Version    int
	Weights    Weights
	Thresholds Thresholds
}

// DefaultPolicyV1 返回一个新值，因此调用者无法通过共享指针改变稍后运行观察到的默认值。
func DefaultPolicyV1() PolicyV1 {
	return PolicyV1{
		Version:    PolicyVersionV1,
		Weights:    DefaultWeights(),
		Thresholds: DefaultThresholds(),
	}
}

// Validate 校验策略版本、阈值和评分权重。
func (p PolicyV1) Validate() error {
	if p.Version != PolicyVersionV1 {
		return fmt.Errorf("heal: unsupported policy version %d", p.Version)
	}
	if err := p.Thresholds.Validate(); err != nil {
		return err
	}
	return p.Weights.Validate()
}
