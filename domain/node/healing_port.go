package node

import (
	"context"

	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/heal"
)

// HealingPort 是执行层请求选择器恢复的端口；node 决定何时请求，heal 拥有决策语义。
type HealingPort interface {
	// Recover 根据目标和 DOM 快照执行自愈评分并返回决策。
	Recover(context.Context, fingerprint.ElementTargetSpec, heal.DOMSnapshot) (heal.Decision, error)
}

// healerPortAdapter 将 heal.Healer 适配为 node 的 HealingPort。
type healerPortAdapter struct{ healer heal.Healer }

// Recover 委托底层 Healer 执行自愈决策。
func (a healerPortAdapter) Recover(ctx context.Context, target fingerprint.ElementTargetSpec, snapshot heal.DOMSnapshot) (heal.Decision, error) {
	return a.healer.Heal(ctx, target, snapshot)
}

// adaptHealer 将非 nil Healer 包装为 HealingPort；nil 输入保持 nil。
func adaptHealer(healer heal.Healer) HealingPort {
	if healer == nil {
		return nil
	}
	return healerPortAdapter{healer: healer}
}
