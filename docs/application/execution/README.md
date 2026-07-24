# 执行应用层

该模块拥有已领取执行权的顶层执行项的浏览器会话生命周期编排和带栅栏校验的写入边界。`EntryExecutor` 校验 `WorkerFence`，并围绕注入的 `EntryRunner` 顺序执行调用方提供的顶层 `execution.WorkflowEntry`：每个执行项恰好调用一次 `BrowserSessionFactory.Create`，同步关闭会话后才继续，且遇到创建、运行或关闭错误时停止；运行或关闭发生 panic 时，完成同步关闭尝试后停止并继续传播 panic。嵌套工作流通过 `EntryRunner` 契约复用该顶层执行项的浏览器。`EntryExecutor` 不接收 `execution.RunSnapshot`，也不调用引擎编译；宿主组合层负责调用 `engine.CompileRunSnapshot` 编译冻结快照并将相应运行器接入执行生命周期。进度、终态、修复观察和晋升/重置提交通过限定工作器的带栅栏校验端口原子写入执行证据。

- [记录进度](record-progress.md)
- [提交步骤迁移](commit-step-transition.md)

## 所有权边界

- 调度模块创建执行实例、冻结快照、排队并授予领取执行权；执行模块不重新解析 `latest` 版本或环境。
- 执行引擎负责通过 `engine.CompileRunSnapshot` 把已冻结的 `execution.RunSnapshot` 编译、绑定为 `node.Program` 并运行；宿主组合层将该能力接入 `EntryRunner`。`EntryExecutor` 只拥有所提供顶层执行项的浏览器会话生命周期，不拥有快照编译、队列、领取执行权、执行实例状态或执行证据持久化。
- `EntryExecutor` 为每个所提供的顶层执行项调用一次浏览器创建并同步关闭会话；同一顶层执行项内的嵌套工作流按 `EntryRunner` 契约共享该浏览器。
- 环境快照仅包含普通 `Properties map[string]string`，运行时注入 `env.` 作用域；Core 不解析凭据或密钥。
- 带类型的参数值和绑定在创建执行实例时校验并冻结，执行模块不执行字符串化降级或运行时回源。
- 执行证据模块拥有执行事实；自动化模块拥有 `NodeVersion` 的发布。`OriginalSelectorResets` 是 `StepTransitionCommit` 的提交输入；晋升由事务内的 `HealGovernancePlanner` 规划，并且只能通过 `StepTransitionCommitResult.Promotions` 返回，调用方不得提供晋升结果。

## 当前边界与延期能力

生产级适配器、读取投影、完整队列实现以及租约心跳和过期恢复由宿主提供；接口存在不表示 Core 已提供这些基础设施。

## 源码与测试

- 源码：[`application/execution/entry_executor.go`](../../../application/execution/entry_executor.go)、[`application/execution/ports.go`](../../../application/execution/ports.go)
- 测试：[`application/execution/entry_executor_test.go`](../../../application/execution/entry_executor_test.go)、[`application/execution/conformancetest/suite.go`](../../../application/execution/conformancetest/suite.go)
