# C16 — 原选择器恢复后重置

> 来源：历史替换评估中的同编号案例；本清单以该评估要求为输入，并以 healix-core 当前实现（`12d1ba2`）重新核验。

## 状态

**当前结果：已由 v0.3 替换实现覆盖；以下证据、清单与验收项按当前模型解释。**

## 业务不变量

新执行实例中原已发布选择器在任何 healed overlay 介入前成功，才算 original recovery；应原子 reset/stale 同 node/base 下相关候选。

## 当前证据

- `domain/automation/healing.go`：`HealOriginalRecovered`、`ResetAll`
- `domain/node/step.go`：优先执行常规定位
- `domain/execution/run.go` 与 `application/engine/coordinator.go`：执行生命周期与执行协调

## 调整清单

- [x] 定义“原选择器”：首选选择器还是任一 published fallback。
- [x] 显式产生 original-selector-success final fact。
- [x] healed overlay success 不得冒充 original recovery。
- [x] 只在合格终态执行实例后应用 reset。
- [x] reset scope 包含 node/base。
- [x] observing candidates 原子 reset。
- [x] awaiting candidates 明确 withdraw/stale/保留策略。
- [x] 同执行实例多次 original success 去重。
- [x] 与候选观察事实乱序时使用 revision/conflict retry。

## 测试与验收

- [x] fresh 执行实例 original success reset 一次。
- [x] overlay success 不 reset。
- [x] 后续执行失败是否 reset 按批准策略测试。
- [x] reset 与 success race 结果确定。
- [x] old-base streak 不影响 current base。

## 依赖与风险

依赖 C11/C23；若过早 reset，失败执行实例会错误清除有效候选。

## 审核

- [x] 批准仅终态 success 后 reset
- [x] 其他策略：________________
