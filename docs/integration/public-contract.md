# 公共契约

## 稳定入口

公共消费者应以 Go 包导出的类型与函数为准：`application/scheduling` 的 `CreateRunService.CreateRun`、纯决策与协调器；`application/execution` 中限定于工作器作用域的执行端口和服务；`application/engine` 的编译和运行。执行实例创建服务通过 `BuildRunSnapshot` 封存 `execution.RunSnapshot`，其中包含 `execution.Plan`、环境身份/修订号/基础 URL、克隆后的普通 `automation.Properties` 及其他冻结执行输入；运行时只读地在 `env.` 下暴露这些属性，Core 不提供凭据子系统。

## 调用链

```mermaid
flowchart LR
  Host[宿主 / API（应用程序接口）] --> Create[CreateRunService.CreateRun]
  Create --> Snapshot[不可变 execution.RunSnapshot<br/>BuildRunSnapshot]
  Snapshot --> Schedule[调度协调器]
  Snapshot --> Compile[engine.CompileRunSnapshot]
  Schedule --> Compile
  Compile --> Run[engine.RunCompiledEntry<br/>engine.RunCompiledEntryWithResult]
  Run --> Ports[执行端口]
  Ports --> Adapters[宿主适配器]
```

## 契约义务

- 宿主负责生成唯一 RunID/ExecutionID 并持久化发布快照。
- 领取执行权适配器负责栅栏校验、原子应用决策与安全释放。
- Core 的 `CancelRunService` 与 `AbortRunService` 实现取消/中止编排；宿主实现 `RunCommandStore`，原子持久化权威执行实例状态、队列成员关系与栅栏失效，并实现 `RunCancellationSignaler` 发送活动执行取消信号。提交后的信号失败不得回滚事务，而应由调用方按 `ErrRunSignalRetryable` 重试信号。
- `QueueOrderWriter` 是宿主必须原子兑现的端口契约。
- 错误应通过 `errors.Is/As` 保持类别，不应依赖完整错误字符串。
- `node.Recorder.Start` 成功后返回本次执行唯一的 `RecordingTimeline`；启用 `StepTimelineSink` 时不得返回 nil。如需消费叶子步骤时间线，可实现 `StepTimelineSink`。
- `engine.RunCompiledEntryWithResult(ctx, entry, cfg)` 接收 `CompiledEntry` 并分别返回执行、录制和时间线结果；`engine.RunCompiledEntry(ctx, entry, cfg)` 是委托该入口的仅返回错误入口。两者都不接受裸 `node.Program`。
- `engine.Config` 不包含运行变量；参数由 `CompileRunSnapshot` 从不可变 `RunSnapshot` 的 invocation scopes 与 Environment 数据编译到 `CompiledEntry.Program`。
- 完成处理器只能获得 `NodeExecutionSnapshot` 和受限 `ReadOnlyBrowser`，其错误不得改变节点原始结果。如需在叶子完成后读取状态，可实现 `NodeCompletionHandler`。

## 证据

- [`contract/public_api_test.go`](../../contract/public_api_test.go)
- [`architecture/dependencies_test.go`](../../architecture/dependencies_test.go)
- [`application/scheduling/run_command_services.go`](../../application/scheduling/run_command_services.go)
- [`application/scheduling/run_command_services_test.go`](../../application/scheduling/run_command_services_test.go)
- [`application/scheduling/run_command_transaction_conformance_test.go`](../../application/scheduling/run_command_transaction_conformance_test.go)
- [`application/scheduling/ports.go`](../../application/scheduling/ports.go)
