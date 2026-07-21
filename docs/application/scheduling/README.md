# Scheduling Application

调度模块把不可变、已封印的 `execution.Plan` 转换为串行推进决策。它包含三个已实现用例：构建执行计划、决定下一入口、在 claim 下处理下一次调度。

## 用例

- [构建执行计划](build-execution-plan.md)
- [决定下一入口](decide-next-entry.md)
- [处理下一 Claim](process-next-claim.md)

## 端口边界

`RunCommands` 与 `QueueOrderWriter` 仅声明宿主必须提供的契约义务，不是本模块已实现的 Application Use Case。尤其不能把接口存在解读为已有持久化 run 命令或完整队列。

## 当前边界与延期能力

以下能力当前**不受支持或明确延期**：lease heartbeat 与过期恢复、active cancellation registry、完整队列实现、参数优先级合并、生产级 adapters 与 read projections。调用方不得从现有接口推断这些能力已经存在。

## 源码与测试

- 源码：[`application/scheduling/ports.go`](../../../application/scheduling/ports.go)
- 测试：[`application/scheduling/coordinator_test.go`](../../../application/scheduling/coordinator_test.go)
