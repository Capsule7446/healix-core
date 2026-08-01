# 适配器职责

## 入站适配器

- 校验并规范化外部输入，创建 RunID、ExecutionID、工作器 ID 与时间戳。
- 组装发布快照后，通过 `CreateRunService.CreateRun` 在一致目录视图中解析依赖并封存 `RunSnapshot`；不得绕过快照封存。
- 把 Core 错误映射为协议错误，同时保留冲突与校验分类。

## 调度持久化适配器

- `ClaimSource`：原子领取执行权、不可伪造令牌、带栅栏校验的释放。
- `EntryStateReader`：返回与 `RunSnapshot` 中 `execution.Plan` 成员完全一致的状态集合。
- `DecisionWriter`：在同一事务校验令牌并应用状态转换、下一项和最终状态。
- `RunCommandStore`：宿主必须在单个原子事务中兑现取消与中止，持久化权威状态、队列成员关系和栅栏失效；Core 的 `CancelRunService` 与 `AbortRunService` 负责编排命令、校验提交结果，并在需要时调用宿主的 `RunCancellationSignaler`。
- `QueueCommandStore`：宿主负责原子校验队列修订号并持久化完整顺序。

## 执行适配器

- 环境快照适配器 提供所选 环境的身份、修订号、基础 URL 和克隆后的普通 `Properties`；Core 仅接收创建执行实例时冻结的副本，并在 `env.` 下提供，不做凭据特定解释。宿主负责安全存储这些属性、授权访问，并防止敏感值泄漏到日志。
- `ExecutionAuthorityVerifier` 在任何 Driver、Recorder、Facts 或时间线端口可见前，向领取权威验证 `RunID + SnapshotDigest + ExecutionID + ClaimToken` 仍为当前有效组合；非空 token 不得视为授权证明。
- `ProgressWriter` 对非终态事件实施工作器栅栏校验。
- `FactCommitter` 原子提交终态与事实，检查修订号、提交身份及已封存依赖目标。
- 驱动器、录制器、执行接收器必须尊重上下文；清理过程仍可能收到分离的上下文。

## 组合根

```mermaid
sequenceDiagram
  participant Host as 宿主
  participant Core as 核心
  participant DB as 持久化适配器
  participant Browser as 驱动器适配器
  Host->>Core: 注入端口并调用用例
  Core->>DB: 领取/读取/写入/提交
  Core->>Browser: 节点执行
  DB-->>Core: 栅栏校验后的结果/错误
  Browser-->>Core: 观察/错误
  Core-->>Host: 结果/错误
```

## 错误与一致性

```mermaid
flowchart TD
  A[适配器收到调用] --> B{边界输入有效?}
  B -- 否 --> E1[校验错误]
  B -- 是 --> C{领取执行权/修订号/身份有效？}
  C -- 否 --> E2[带类型的冲突/栅栏校验错误]
  C -- 是 --> D[执行 I/O 或事务]
  D --> F{I/O 失败?}
  F -- 是 --> E3[携带上下文包装错误]
  F -- 否 --> G[返回成功]
```

## 当前边界与延期能力

以下能力当前**不受支持或明确延期**：租约心跳与过期恢复、活动取消注册表、完整队列实现、参数优先级合并、生产级适配器与读取投影。调用方不得从现有接口推断这些能力已经存在。

## 当前不应伪造的能力

生产级适配器与读取投影尚未提供。集成方必须明确实现或继续延期：心跳与租约过期恢复、活动取消注册表、完整队列、参数优先级，以及查询投影的一致性与重建。

## 源码证据

- [`application/scheduling/instance_command_services.go`](../../application/scheduling/instance_command_services.go)
- [`application/scheduling/instance_command_services_test.go`](../../application/scheduling/instance_command_services_test.go)
- [`application/scheduling/instance_command_transaction_conformance_test.go`](../../application/scheduling/instance_command_transaction_conformance_test.go)
- [`application/scheduling/coordinator.go`](../../application/scheduling/coordinator.go)
- [`application/execution/ports.go`](../../application/execution/ports.go)
- [`application/engine/engine.go`](../../application/engine/engine.go)
