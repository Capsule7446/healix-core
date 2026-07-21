# 公共契约

## 稳定入口

公共消费者应以 Go package 导出的类型与函数为准：`application/scheduling` 的 plan 构建、纯决策与 coordinator；`application/execution` 的 worker-scoped ports/credential service；`application/engine` 的 compile/run。领域 `execution.Plan` 是运行成员、顺序、版本和 failure policy 的封印快照。

## 调用链

```mermaid
flowchart LR
  Host[Host / API] --> Build[BuildExecutionPlan]
  Build --> Schedule[Scheduling Coordinator]
  Schedule --> Compile[CompilePlan]
  Compile --> Run[RunProgram]
  Run --> Ports[Execution Ports]
  Ports --> Adapters[Host Adapters]
```

## 契约义务

- Host 负责生成唯一 RunID/ExecutionID 和持久化 publication snapshot。
- Claim adapter 负责 fencing、原子 decision apply 与安全 release。
- `RunCommands`、`QueueOrderWriter` 是 port-only 契约，不是 core 已实现 use case。
- 错误应通过 `errors.Is/As` 保持类别，不应依赖完整错误字符串。
- Core 不承诺 HTTP、数据库 schema、消息格式或序列化兼容性。

## 当前边界与延期能力

以下能力当前**不受支持或明确延期**：lease heartbeat 与过期恢复、active cancellation registry、完整队列实现、参数优先级合并、生产级 adapters 与 read projections。调用方不得从现有接口推断这些能力已经存在。

## 证据

- [`contract/public_api_test.go`](../../contract/public_api_test.go)
- [`architecture/dependencies_test.go`](../../architecture/dependencies_test.go)
- [`application/scheduling/ports.go`](../../application/scheduling/ports.go)
