# C11 — 高置信自愈（APPLIED）

> 来源：历史替换评估中的同编号案例；本清单以该评估要求为输入，并以 healix-core 当前实现（`12d1ba2`）重新核验。

## 状态

**当前结果：已由 v0.3 替换实现覆盖；以下证据、清单与验收项按当前模型解释。**

## 业务不变量

APPLIED 候选仅在 safety ALLOW 且重新 locate 成功后恢复当前执行实例；成功事实按 run 去重并进入治理 streak，但不直接修改资产。

## 当前证据

- `domain/heal/healer.go`：评分/band
- `domain/heal/assessment.go`：运行时安全
- `domain/node/step.go`：relocate 与 执行实例局部覆盖层
- `domain/automation/healing.go`：`HealStreak.Observe`

## 调整清单

- [x] 候选项评分、顺序与分档。
- [x] safety assessment 与 relocate。
- [x] 执行实例局部选择器覆盖层。
- [x] 定义 canonical candidate hash。
- [x] 定义观察事实唯一键 `(run,node,base,hash,band)`。
- [x] 终态 success 后才计入 streak。
- [x] 执行证据 → governance 观察事实 service。
- [x] 重复、乱序、并发观察幂等。
- [x] base/hash/band 变化触发明确 reset/stale。

## 测试与验收

- [x] safety allow + relocate success 恢复当前执行实例。
- [x] relocation failure 不计 streak。
- [x] 同执行实例重放不重复计数。
- [x] 不同执行实例连续成功按规则累积。
- [x] current NodeVersion 不因运行期 overlay 变化。

## 依赖与风险

依赖 C23 原子终态 evidence；hash canonicalization 和事件顺序是核心风险。

## 审核

- [x] 批准
- [x] 修改：________________
