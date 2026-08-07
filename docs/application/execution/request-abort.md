# 请求中止（D-17）

`DecideAbortRequest(state, request)` 是把终态意图从 `NONE` / `CANCEL` 推进到
`ABORT` 的唯一入口。它是纯函数：无 `Context`、无端口、无时钟，同一对输入永远
得到同一个决策，宿主可以先调用它做预检，拿到的答案与提交时用的一致。

它**不是** `AbortInstanceService`。后者语义是立即终止，第一步就 invalidate
claim；本入口只记录「有人要求停」，终态仍由
[`EntryCompletionTransaction.CompleteEntry`](../../../application/execution/entry_completion_transaction.go)
唯一承担，abort 与正常完成因此汇流到同一条终结写入路径，而不是两条会互相矛盾
的路径。

## 决策

`AbortRequestDecision` 只有六个字段：`Current*` 三元组是精确 CAS 谓词，`Next*`
三元组是逐字写入值。**没有 `EntryStatus`，这个缺席就是要点。**

| 起始意图 | 结果 |
|---|---|
| `NONE` | → `ABORT`，revision +1 |
| `CANCEL` | → `ABORT`，revision +1（允许升级：abort 严格强于 cancel——cancel 停住还没开始的，abort 结束正在跑的那个） |
| `ABORT` | 拒绝，`EXECUTION_ABORT_REQUEST_ALREADY_ABORTING` |

非 `RUNNING` 一律拒绝，`EXECUTION_ABORT_REQUEST_NOT_RUNNING`。它与
`EXECUTION_ENTRY_COMPLETION_NOT_RUNNING` 是两个码：迟到的 completion 通常是待查
的重放，而对已完成 entry 的 abort 请求是无事可做。

### 为什么重复 abort 是冲突而不是幂等命中

空推一次 revision 看起来无害，实则有害：它会把 CAS 谓词从**已经读到旧值的并发
completion** 脚下抽走，于是一次重复点击被放大成一个 completion 冲突。fail-closed
更安全。命令级的重放由 digest 幂等承担，不需要决策层再兜一次。

### 为什么请求阶段不推进 `CancellationGeneration`

D-12 只在意图**真被执行**（终态落到 `CANCELED` / `ABORTED`）时花掉一个
generation。请求不是执行。请求也推进等于一次 abort 花掉两个 generation，而调度器
读它决定实例还能不能推进。

推论：generation 已经在 `MaxExpectedEntryCompletionRevision` 也**不**阻塞请求——
它在这一步没有后继要耗尽。

### 命令身份不参与决策

`AbortRequest.AbortPendingCommandID` 是幂等/审计身份，不是决策依据，沿用 D-12
裁决二的定位。同一 state 喂两个不同的命令 ID，`DecideAbortRequest` 返回逐字段
相同的决策；这条有专测。

## 事务

纯决策不足以验收「每个注入点重放后结果与一次成功等价」，因此同时提供与 D-12
逐项对应的幂等事务：`RequestAbortCommand` / `RequestAbortDigest` /
`RequestAbortIntent` / `ValidateRequestAbortIntentDigest` /
`AbortRequestTransaction` / `AbortRequestService`。

命令刻意不带时间戳，理由与 `CompleteEntryCommand` 相同：挂钟会让每次重试的
digest 都不同，把「崩溃后重试」变成「第二次应用」。

**原子边界**——下列写入必须在同一事务内，否则一个都不能落：

- pending 终态意图，按 `Current*` 做 CAS、写入 `Next*`
- 以 `AbortPendingCommandID` 为键的 abort 命令回执
- 以 `(EntryID, RequestDigest)` 为键的幂等回执，**最后**写，这样崩溃在它之前
  会让整批不可见且可重试

**这里不得发生**：改 entry 状态、写事实、invalidate claim、终结 action gate。
宿主若在这里 invalidate claim，随后 completion 的 authority CAS 就会打在 stale
行上——这正是 D-17 存在的原因。

## Conformance

[`conformancetest.RunAbortRequest`](../../../application/execution/conformancetest/abort_request_suite.go)
提供四个注入点：`BEFORE_REPLAY` / `AFTER_DECISION` / `AFTER_INTENT` /
`AFTER_RECEIPT`。

套件里 `request-leaves-the-entry-running` 一格专门钉死上面那条禁令：宿主适配器
若在此终结 entry 或花掉 generation，直接变红。

## 源码与测试

- 源码：[`application/execution/abort_request.go`](../../../application/execution/abort_request.go)、[`application/execution/abort_request_transaction.go`](../../../application/execution/abort_request_transaction.go)
- 测试：[`application/execution/abort_request_test.go`](../../../application/execution/abort_request_test.go)、[`application/execution/abort_request_transaction_test.go`](../../../application/execution/abort_request_transaction_test.go)、[`application/execution/conformancetest/abort_request_suite.go`](../../../application/execution/conformancetest/abort_request_suite.go)
