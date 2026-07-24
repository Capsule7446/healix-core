# C20 — 执行实例排队、领取执行权与重排

> 来源：历史替换评估中的同编号案例；本清单以该评估要求为输入，并以 healix-core 当前实现（`12d1ba2`）重新核验。

## 状态

**当前结果：已由 v0.3 替换实现覆盖；以下证据、清单与验收项按当前模型解释。**

## 业务不变量

Queued 执行实例只能被一个工作器以 栅栏 token 独占；reorder 必须是 eligible queue 的原子、并发安全转换。

## 当前证据

- `application/scheduling/coordinator.go`：ClaimNext/Release
- `application/scheduling/ports.go`：QueueOrderWriter
- `domain/execution/run.go`：队列与执行实例状态

## 调整清单

- [x] 领取/释放顺序与 分离式释放超时。
- [x] 定义 queue scope、快照、修订号。
- [x] full reorder 要求 exact permutation。
- [x] partial move 使用独立 `MoveRunBefore/After`。
- [x] 命令 ID 与 WasApplied/new 修订号。
- [x] claimed/active/canceled/terminal IDs 不可 reorder。
- [x] 领取/重排同一事务或类型化冲突。
- [x] 定义工作器所有权失效与 stale token；具体存活检测机制由宿主实现。

## 测试与验收

- [x] 两工作器领取执行权一个执行实例仅一方成功。
- [x] stale token 无法 ApplyDecision。
- [x] duplicate/missing/unknown reorder IDs 被拒绝。
- [x] reorder/claim race 无混合结果。
- [x] caller canceled 后仍尝试释放领取执行权。

## 依赖与风险

依赖宿主原子事务与工作器所有权实现；当前 `[]string` 接口不足以表达并发控制。

## 审核

- [x] 批准 full reorder + 修订号
- [x] 修改：________________
