# C16 — 原 selector 恢复后重置

> 来源：Healix 仓库 `docs/refactor/healix-core-v0.3.0-replacement-assessment.md` 对应 Case；本清单以该评估要求为输入，并以 healix-core 当前 `master` 源码重新核验。

## 状态

**当前：纯规则存在；runtime fact → streak reset 闭环缺失。**

## 业务不变量

新 Run 中原已发布 selector 在任何 healed overlay 介入前成功，才算 original recovery；应原子 reset/stale 同 node/base 下相关候选。

## 当前证据

- `domain/automation/healing.go`：`HealOriginalRecovered`、`ResetAll`
- `domain/node/step.go`：normal locate first
- `domain/node/runtime.go`：run-local overlay

## 调整清单

- [x] 定义“原 selector”：首选 selector 还是任一 published fallback。
- [x] 显式产生 original-selector-success final fact。
- [x] healed overlay success 不得冒充 original recovery。
- [x] 只在合格 terminal Run 后应用 reset。
- [x] reset scope 包含 node/base。
- [x] observing candidates 原子 reset。
- [x] awaiting candidates 明确 withdraw/stale/保留策略。
- [x] 同 Run 多次 original success 去重。
- [x] 与 candidate observation 乱序时使用 revision/conflict retry。

## 测试与验收

- [x] fresh Run original success reset 一次。
- [x] overlay success 不 reset。
- [x] 后续执行失败是否 reset 按批准策略测试。
- [x] reset 与 success race 结果确定。
- [x] old-base streak 不影响 current base。

## 依赖与风险

依赖 C11/C23；若过早 reset，失败 Run 会错误清除有效候选。

## 审核

- [x] 批准仅 terminal success 后 reset
- [x] 其他策略：________________
