# C20 — Run 排队、Claim 与重排

> 来源：Healix 仓库 `docs/refactor/healix-core-v0.3.0-replacement-assessment.md` 对应 Case；本清单以该评估要求为输入，并以 healix-core 当前 `master` 源码重新核验。

## 状态

**当前：claim coordinator 部分实现；reorder 仅端口。**

## 业务不变量

Queued Run 只能被一个 worker 以 fence token 独占；reorder 必须是 eligible queue 的原子、并发安全转换。

## 当前证据

- `application/scheduling/coordinator.go`：ClaimNext/Release
- `application/scheduling/ports.go`：QueueOrderWriter
- `domain/execution/run.go`：queue/run states

## 调整清单

- [x] claim/release sequencing 与 detached release timeout。
- [x] 定义 queue scope、snapshot、revision。
- [x] full reorder 要求 exact permutation。
- [x] partial move 使用独立 `MoveRunBefore/After`。
- [x] command ID 与 WasApplied/new revision。
- [x] claimed/active/canceled/terminal IDs 不可 reorder。
- [x] claim/reorder 同一事务或 typed conflict。
- [x] 定义 worker 所有权失效与 stale token；具体存活检测机制由 Host 实现。

## 测试与验收

- [x] 两 worker claim 一个 Run 仅一方成功。
- [x] stale token 无法 ApplyDecision。
- [x] duplicate/missing/unknown reorder IDs 被拒绝。
- [x] reorder/claim race 无混合结果。
- [x] caller canceled 后仍尝试释放 claim。

## 依赖与风险

依赖 Host 原子事务与 worker 所有权实现；当前 `[]string` 接口不足以表达并发控制。

## 审核

- [x] 批准 full reorder + revision
- [x] 修改：________________
