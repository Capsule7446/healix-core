# C23 — 终态事实原子提交

> 来源：历史替换评估中的同编号案例；本清单以该评估要求为输入，并以 healix-core 当前实现（`12d1ba2`）重新核验。

## 状态

**当前结果：已由 v0.3 替换实现覆盖；以下证据、清单与验收项按当前模型解释。**

## 业务不变量

步骤终态、最终校验、自愈观察事实、原选择器重置、streak 和晋升必须在同一 fence/revision/commit identity 下全有或全无。

## 当前证据

- `domain/evidence/commits.go`：`StepTransitionCommit`
- `application/execution/ports.go`：`FactCommitter`
- `docs/application/execution/commit-step-transition.md`
- `domain/automation/healing.go`：连续命中决策

## 调整清单

- [x] 提交 ID、expected 修订号、终态 phase、identity/payload limits。
- [x] final facts 聚合 envelope。
- [x] 新增 thin `StepTransitionService` 验证 scope/commit 后单次委托。
- [x] 统一 stale 栅栏 typed error。
- [x] same ID/same payload 返回原 result/WasApplied=false。
- [x] same ID/different payload 返回 identity conflict。
- [x] 已封存 dependency target 在事务内权威校验。
- [x] streak/promotion/reset 纳入适配器事务实现。
- [x] transactional outbox 与终态 event 同事务。
- [x] 提供 adapter fault-injection conformance suite。

## 测试与验收

- [x] 任一 fact/streak/version 写失败全回滚。
- [x] 失效修订号/栅栏无任何变化。
- [x] duplicate 提交不重复 facts/promotion。
- [x] 并发阈值只产生一个晋升。
- [x] 终态 state 不会缺 final facts。

## 依赖与风险

跨 Evidence/Automation 表的原子性是替换能否成立的关键；接口名称本身不能保证事务。

## 审核

- [x] 批准单事务要求
- [x] 修改：________________
