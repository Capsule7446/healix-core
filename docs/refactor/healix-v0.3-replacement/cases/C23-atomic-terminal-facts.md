# C23 — 终态事实原子提交

> 来源：Healix 仓库 `docs/refactor/healix-core-v0.3.0-replacement-assessment.md` 对应 Case；本清单以该评估要求为输入，并以 healix-core 当前 `master` 源码重新核验。

## 状态

**当前：强领域/端口契约存在；完整服务和适配器事务证明待补。**

## 业务不变量

Step terminal、final validations、heal observations、original resets、streak 和 promotion 必须在同一 fence/revision/commit identity 下全有或全无。

## 当前证据

- `domain/evidence/commits.go`：`StepTransitionCommit`
- `application/execution/ports.go`：`FactCommitter`
- `docs/application/execution/commit-step-transition.md`
- `domain/automation/healing.go`：streak decisions

## 调整清单

- [x] commit ID、expected revision、terminal phase、identity/payload limits。
- [x] final facts 聚合 envelope。
- [x] 新增 thin `StepTransitionService` 验证 scope/commit 后单次委托。
- [x] 统一 stale fence typed error。
- [x] same ID/same payload 返回原 result/WasApplied=false。
- [x] same ID/different payload 返回 identity conflict。
- [x] sealed dependency target 在事务内权威校验。
- [x] streak/promotion/reset 纳入适配器事务实现。
- [x] transactional outbox 与 terminal event 同事务。
- [x] 提供 adapter fault-injection conformance suite。

## 测试与验收

- [x] 任一 fact/streak/version 写失败全回滚。
- [x] stale revision/fence 无任何变化。
- [x] duplicate commit 不重复 facts/promotion。
- [x] 并发阈值只产生一个 promotion。
- [x] terminal state 不会缺 final facts。

## 依赖与风险

跨 Evidence/Automation 表的原子性是替换能否成立的关键；接口名称本身不能保证事务。

## 审核

- [x] 批准单事务要求
- [x] 修改：________________
