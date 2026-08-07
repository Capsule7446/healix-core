# 执行应用层

该模块拥有已领取执行权的顶层执行项的浏览器会话生命周期编排和带栅栏校验的写入边界。`EntryExecutor.Execute(ctx, fence, entry)` 每次只执行一个顶层 `execution.Entry`：先 `fence.Validate()`——栅栏不合法直接返回 `EXECUTION_WORKER_FENCE_INVALID`，授权器与工厂都不会被调用；栅栏合法才调用必填的 `EntryAuthorizer.AuthorizeEntry`，授权通过后才调用一次 `BrowserSessionFactory.Create`，运行注入的 `EntryRunner`，并在返回前同步关闭会话。运行或关闭发生 panic 时，完成同步关闭尝试后继续传播 panic；两者同时 panic 则抛出 `EntryLifecyclePanic`。执行项之间的顺序以及"失败后是否继续"属于调度模块，不在执行器内。嵌套工作流通过 `EntryRunner` 契约复用该顶层执行项的浏览器。`EntryExecutor` 不接收 `execution.InstanceSnapshot`，也不调用引擎编译；宿主组合层负责调用 `engine.CompilePlan` 编译冻结快照并将相应运行器接入执行生命周期。进度、终态、修复观察和晋升/重置提交通过限定工作器的带栅栏校验端口原子写入执行证据。

- [记录进度](record-progress.md)
- [提交步骤迁移](commit-step-transition.md)
- [请求中止](request-abort.md)
- [恢复期终态语义](recovery-terminal-semantics.md)

## 所有权边界

- 调度模块创建执行实例、冻结快照、排队并授予领取执行权；执行模块不重新解析 `latest` 版本或环境。
- 执行引擎负责通过 `engine.CompilePlan` 把已冻结的 `execution.InstanceSnapshot` 编译为带私有 Program 与身份封印的 `CompiledEntry`，再由 `engine.RunProgram` 运行；宿主组合层将该能力接入 `EntryRunner`。`EntryExecutor` 只拥有所提供顶层执行项的浏览器会话生命周期，不拥有快照编译、队列、领取执行权、执行实例状态或执行证据持久化。
- `EntryExecutor` 每次 `Execute` 调用一次浏览器创建并同步关闭会话；同一顶层执行项内的嵌套工作流按 `EntryRunner` 契约共享该浏览器。
- 授权有两个端口且互不覆盖：`EntryAuthorizer` 在浏览器创建前只看得到 `WorkerFence` 与 `Entry`，`engine.ExecutionAuthorityVerifier` 在运行中看得到含 `SnapshotDigest` 的完整四元组。前者严格弱于后者，Core 不强制两者背后是同一个决策。
- 环境快照仅包含普通 `Properties map[string]string`，运行时注入 `env.` 作用域；Core 不解析凭据或密钥。
- 带类型的参数值和绑定在创建执行实例时校验并冻结，执行模块不执行字符串化降级或运行时回源。
- 执行证据模块拥有执行事实；自动化模块拥有 `NodeVersion` 的发布。`OriginalSelectorResets` 是 `StepTransitionCommit` 的提交输入；晋升由事务内的 `HealGovernancePlanner` 规划，并且只能通过 `StepTransitionCommitResult.Promotions` 返回，调用方不得提供晋升结果。

## 当前边界与延期能力

以下能力当前**不受支持或明确延期**，本目录各用例文件不再重复这份清单：租约心跳与过期恢复、活动取消注册表、完整队列实现、参数优先级合并、生产级适配器与读取投影。调用方不得从现有接口推断这些能力已经存在——接口存在不表示 Core 已提供这些基础设施。

## 源码与测试

- 源码：[`application/execution/entry_executor.go`](../../../application/execution/entry_executor.go)、[`application/execution/ports.go`](../../../application/execution/ports.go)
- 测试：[`application/execution/entry_executor_test.go`](../../../application/execution/entry_executor_test.go)、[`application/execution/conformancetest/suite.go`](../../../application/execution/conformancetest/suite.go)
