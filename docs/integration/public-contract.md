# 公共契约

## 稳定入口

公共消费者应以 Go 包导出的类型与函数为准：`application/scheduling` 的 `CreateInstanceService.CreateInstance`、纯决策与协调器；`application/execution` 中限定于工作器作用域的执行端口和服务；`application/engine` 的编译和运行。执行实例创建服务通过 `BuildInstanceSnapshot` 封存 `execution.InstanceSnapshot`，其中包含 `execution.Plan`、环境身份/修订号/基础 URL、克隆后的 `EnvironmentSnapshot`（带类型的 `Variables map[string]parameter.Value`，V1 快照另带字符串 `Properties`）及其他冻结执行输入；运行时只读地在 `env.` 下暴露这些属性，Core 不提供凭据子系统。

## 调用链

```mermaid
flowchart LR
  Host[宿主 / API（应用程序接口）] --> Create[CreateInstanceService.CreateInstance]
  Create --> Snapshot[不可变 execution.InstanceSnapshot<br/>BuildInstanceSnapshot]
  Snapshot --> Schedule[调度协调器]
  Snapshot --> Compile[engine.CompilePlan]
  Schedule --> Compile
  Compile --> Run[engine.RunProgram]
  Run --> Ports[执行端口]
  Ports --> Adapters[宿主适配器]
```

## 契约义务

- 宿主负责生成唯一 InstanceID/EntryID 并持久化发布快照。
- 领取执行权适配器负责栅栏校验、原子应用决策与安全释放。
- Core 的 `CancelInstanceService` 与 `AbortInstanceService` 实现取消/中止编排；宿主实现 `InstanceCommandStore`，原子持久化权威执行实例状态、队列成员关系与栅栏失效，并实现 `InstanceCancellationSignaler` 发送活动执行取消信号。提交后的信号失败不得回滚事务，而应由调用方按 `EXECUTION_INSTANCE_SIGNAL_RETRYABLE` 重试信号。
- `EntryExecutor` 新增必填端口 `EntryAuthorizer`：`NewEntryExecutor(authorizer, factory, runner, closeTimeout)`，任一端口为 nil 或 `closeTimeout <= 0` 返回 `EXECUTION_ENTRY_EXECUTOR_CONFIGURATION_INVALID`。`Execute` 先校验 `WorkerFence`：栅栏本身不合法直接返回 `EXECUTION_WORKER_FENCE_INVALID`，此时授权器与工厂都不会被调用；栅栏合法才执行 `AuthorizeEntry`，通过后才 `factory.Create`。授权失败原样透传。
- 授权当前由两个互不相关的端口承担，宿主必须分别实现，且不得假设其中一个覆盖另一个：`EntryAuthorizer.AuthorizeEntry(ctx, fence, entry)` 只看得到 `WorkerFence`（`InstanceID` + `ClaimToken`）与 `Entry`，看不到 `SnapshotDigest`；`ExecutionAuthorityVerifier.VerifyExecutionAuthority` 看到完整四元组 `ExecutionAuthority{InstanceID, SnapshotDigest, EntryID, ClaimToken}`。开浏览器之前的预检因此严格弱于运行中的校验，Core 也不强制两者由同一决策支撑；若宿主不自行保证两端口对同一次领取给出一致答案，预检放行而运行中拒绝的窗口只会在浏览器已经打开之后才关闭。
- `QueueCommandStore` 是宿主必须原子兑现的队列修订 CAS 与完整排列写入契约。
- 错误分类以 `domain/fault` 为准，不应依赖完整错误字符串：`fault.CodeOf` / `fault.Describe` 给出边界处唯一的分类码（用于路由与渲染），`fault.IsCode` 回答某个码是否出现在错误链的任意一层（用于判断某种失败是否参与其中）。两者会对同一个错误给出不同答案，不得混用。
- `node.Recorder.Start` 成功后返回本次执行唯一的 `RecordingTimeline`；启用 `StepTimelineSink` 时不得返回 nil。如需消费叶子步骤时间线，可实现 `StepTimelineSink`。
- `engine.CompilePlan(snapshot)` 是唯一公开编译入口；`CompiledPlan` 只通过返回独立副本的 `Entries()` 与 `Entry(entryID)` 暴露结果。
- `engine.RunProgram(ctx, entry, cfg)` 是唯一公开运行入口，接收带私有 Program 与身份封印的 `CompiledEntry`，并分别返回执行、录制和时间线结果；不接受裸 `node.Program`。
- `engine.Config.InstanceID + SnapshotDigest + EntryID + ClaimToken` 必须来自已领取执行权的独立权威。入口先校验前三项与 entry 私有封印一致且 ClaimToken 非空，再通过必填的 `ExecutionAuthorityVerifier` 向领取权威验证完整四元身份；只有验证成功后 Runtime、Driver、Recorder、Facts 等运行端口才可见。身份错配返回 `EXECUTION_IDENTITY_MISMATCH`，缺失 verifier 返回 `EXECUTION_AUTHORITY_VERIFIER_REQUIRED`，权威拒绝或故障原样传播。
- `engine.Config` 不包含运行变量；参数由 `CompilePlan` 从不可变 `InstanceSnapshot` 的 invocation scopes 与 Environment 数据编译到私有 Program。
- 完成处理器只能获得 `NodeExecutionSnapshot` 和受限 `ReadOnlyBrowser`，其错误不得改变节点原始结果。如需在叶子完成后读取状态，可实现 `NodeCompletionHandler`。

## 证据

- [`contract/public_api_test.go`](../../contract/public_api_test.go)
- [`architecture/dependencies_test.go`](../../architecture/dependencies_test.go)
- [`application/scheduling/instance_command_services.go`](../../application/scheduling/instance_command_services.go)
- [`application/scheduling/instance_command_services_test.go`](../../application/scheduling/instance_command_services_test.go)
- [`application/scheduling/instance_command_transaction_conformance_test.go`](../../application/scheduling/instance_command_transaction_conformance_test.go)
- [`application/scheduling/ports.go`](../../application/scheduling/ports.go)
