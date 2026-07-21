# 提交步骤终态迁移

## 目标

定义一次原子提交步骤终态及最终事实的端口契约；当前没有 application service 实现。

## 输入

- `context.Context`。
- fenced `WorkerScope`。
- `evidence.StepTransitionCommit`，含 commit identity、期望 revision、终态事实及 promotion/reset 目标。

## 输出

`evidence.StepTransitionCommitResult` 或 error；标准冲突分类为 `ErrStepRevisionConflict`、`ErrCommitIdentityConflict`。

## 时序

```mermaid
sequenceDiagram
    participant Engine
    participant Port as FactCommitter
    participant Adapter
    Engine->>Port: CommitStepTransition(scope, commit)
    Port->>Adapter: 原子校验并写入
    Adapter-->>Engine: result / typed conflict / error
```

## 流程与错误

```mermaid
flowchart TD
    A[接收 commit] --> B{claim fencing 有效?}
    B -- 否 --> E1[拒绝过期 worker]
    B -- 是 --> C{commit identity 冲突?}
    C -- 是 --> E2[ErrCommitIdentityConflict]
    C -- 否 --> D{expected revision 匹配?}
    D -- 否 --> E3[ErrStepRevisionConflict]
    D -- 是 --> F{promotion/reset 属于 sealed dependency?}
    F -- 否 --> E4[拒绝越权目标]
    F -- 是 --> G[原子写终态与最终事实]
    G --> H[返回 CommitResult]
```

## 不变量

- 终态迁移与最终 facts 必须同一原子事务。
- adapter 必须校验 fencing、revision 与 commit identity 幂等性。
- promotion/reset 目标必须属于被提交步骤的 sealed node dependencies。
- port 契约不等于已存在持久化实现。

## 当前边界与延期能力

以下能力当前**不受支持或明确延期**：lease heartbeat 与过期恢复、active cancellation registry、完整队列实现、参数优先级合并、生产级 adapters 与 read projections。调用方不得从现有接口推断这些能力已经存在。

## 源码与测试

- 源码：[`application/execution/ports.go`](../../../application/execution/ports.go)
- 测试：[`application/execution/ports_test.go`](../../../application/execution/ports_test.go)
