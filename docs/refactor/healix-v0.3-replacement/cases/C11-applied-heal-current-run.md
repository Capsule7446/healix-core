# C11 — 高置信自愈（APPLIED）

> 来源：Healix 仓库 `docs/refactor/healix-core-v0.3.0-replacement-assessment.md` 对应 Case；本清单以该评估要求为输入，并以 healix-core 当前 `master` 源码重新核验。

## 状态

**当前：运行期恢复已实现；跨 Run streak ingestion 缺失。**

## 业务不变量

APPLIED 候选仅在 safety ALLOW 且重新 locate 成功后恢复当前 Run；成功事实按 run 去重并进入治理 streak，但不直接修改资产。

## 当前证据

- `domain/heal/healer.go`：评分/band
- `domain/heal/assessment.go`：runtime safety
- `domain/node/step.go`：relocate 与 run-local overlay
- `domain/automation/healing.go`：`HealStreak.Observe`

## 调整清单

- [x] candidate score/order/band。
- [x] safety assessment 与 relocate。
- [x] run-local selector overlay。
- [x] 定义 canonical candidate hash。
- [x] 定义 observation 唯一键 `(run,node,base,hash,band)`。
- [x] terminal success 后才计入 streak。
- [x] Evidence → governance observation service。
- [x] 重复、乱序、并发观察幂等。
- [x] base/hash/band 变化触发明确 reset/stale。

## 测试与验收

- [x] safety allow + relocate success 恢复当前 Run。
- [x] relocation failure 不计 streak。
- [x] 同 Run 重放不重复计数。
- [x] 不同 Run 连续成功按规则累积。
- [x] current NodeVersion 不因运行期 overlay 变化。

## 依赖与风险

依赖 C23 原子 terminal evidence；hash canonicalization 和事件顺序是核心风险。

## 审核

- [x] 批准
- [x] 修改：________________
