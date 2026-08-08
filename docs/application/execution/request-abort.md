# 请求中止

`DecideAbortRequest(state, request)` 是把终态意图从 `NONE` / `CANCEL` 推进到 `ABORT` 的唯一纯函数入口。它没有 `Context`、端口或时钟，同一输入始终得到同一决策；宿主可在提交前预检。

该决策不等同于 `AbortInstanceService` 的立即终止。请求阶段只记录待处理中止意图；终态仍由 `EntryCompletionService.Complete` 写入，避免出现两条互相矛盾的终结路径。

## 决策

`AbortRequestDecision` 由 `Current*` CAS 谓词和 `Next*` 写入值组成，不携带 `EntryStatus`：

| 起始意图 | 结果 |
|---|---|
| `NONE` | → `ABORT`，revision +1 |
| `CANCEL` | → `ABORT`，revision +1 |
| `ABORT` | 拒绝，`EXECUTION_ABORT_REQUEST_ALREADY_ABORTING` |

非 `RUNNING` entry 返回 `EXECUTION_ABORT_REQUEST_NOT_RUNNING`。重复请求不在决策层写入空 revision；命令级重放由 request digest 幂等处理。

请求阶段不推进 `CancellationGeneration`。generation 只在终态真正写为 `CANCELED` 或 `ABORTED` 时消耗，避免一次用户请求占用两次完成预算。

`AbortPendingCommandID` 只用于幂等和审计，不参与决策；不同命令身份在相同状态下必须得到逐字段相同的决策。

## 幂等事务

`RequestAbortCommand`、`RequestAbortDigest`、`RequestAbortIntent`、`ValidateRequestAbortIntentDigest`、`AbortRequestTransaction` 和 `AbortRequestService` 共同实现事务边界。命令不携带时间戳，保证崩溃重试使用相同摘要。

下列写入必须在一个事务内完成：

- 按 `Current*` CAS 写入 pending 终态意图和 `Next*` 值；
- 以 `AbortPendingCommandID` 为键保存命令回执；
- 最后写入 `(EntryID, RequestDigest)` 幂等回执，使中途失败的整批操作可重试。

该事务不得修改 entry 状态、写执行事实、失效 claim 或关闭 action gate；这些动作属于完成或工作器生命周期端口。

## 一致性套件

[`conformancetest.RunAbortRequest`](../../../application/execution/conformancetest/abort_request_suite.go) 覆盖 `BEFORE_REPLAY`、`AFTER_DECISION`、`AFTER_INTENT`、`AFTER_RECEIPT` 四个注入点，并验证请求不会提前终结 entry 或消耗 generation。

## 源码与测试

- 源码：[`application/execution/abort_request.go`](../../../application/execution/abort_request.go)、[`application/execution/abort_request_transaction.go`](../../../application/execution/abort_request_transaction.go)
- 测试：[`application/execution/abort_request_test.go`](../../../application/execution/abort_request_test.go)、[`application/execution/abort_request_transaction_test.go`](../../../application/execution/abort_request_transaction_test.go)、[`application/execution/conformancetest/abort_request_suite.go`](../../../application/execution/conformancetest/abort_request_suite.go)
