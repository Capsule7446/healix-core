# 恢复期终态语义（D-18）

**上游答复：采用方案 (b)，增加可区分性。** 宿主可据此钉死实现，不必再讨论。

## 问题

宿主进程崩溃重启后会发现孤儿 `RUNNING` entry：claim 还在、gate 还开着，但那次
执行的引擎结果从未被观测到，因为观测它的进程没了。终结它才能释放 claim，而
`DecideEntryCompletion` 要求喂一个 `EngineOutcome`。

在 v0.7 里宿主手上唯一合法的构造是 `NotStartedEngineOutcome()`，它在意图 `NONE`
下判为 `FAILED`。于是两件不同的事写成了同一个字：

| 真实发生的事 | 落进存储的终态 |
|---|---|
| Entry 真的跑了，业务断言失败 | `FAILED` |
| Entry 压根没跑起来（栅栏陈旧、授权被拒、浏览器建不起来） | `FAILED` |
| Entry 可能跑完了，但没人活着看到 | `FAILED` |

而且用 `NotStartedEngineOutcome()` 表达第三种情况**不只是有损，是假陈述**：
「未启动」断言引擎已知没有开始，孤儿则相反——引擎很可能真的跑完了，只是把结果
一起带走了。这两句话不能用同一个值说。

## 答复

新增第三个观测值，并把「观测」从「结果」里分出来成为独立的一轴。

### 两条轴

- **`EntryStatus`** 回答「这个 entry 最后成了什么」。终态意图可以左右它。
  七态机**不变**：`FAILED` 仍是一个既没成功、也没被要求停止的 entry 唯一诚实的
  终态，不新增第八个状态。
- **`TerminalCause`** 回答「有没有人看见它发生」。**任何意图都改变不了它。**

`EntryCompletionDecision` 现在同时携带两者。cause 跟着 decision 走而不是留给宿主
从 command 推导，因为宿主是把这个结构原样持久化为权威终态记录的——一个需要它
自行计算的字段，就是两个宿主可能算出不同结果的字段。

| `ExecutionOutcome` | `TerminalCause` | 含义 |
|---|---|---|
| `SUCCEEDED` / `FAILED` / `CANCELED` | `COMPLETED` | 引擎跑了并报告了。结果可以是失败；使它 completed 的是**它被观测到了** |
| `NOT_STARTED` | `NOT_STARTED` | 已知引擎从未开始 |
| `INTERRUPTED` | `INTERRUPTED` | 运行未被观测到结束，是否真的跑完不可知 |

### 新构造

`InterruptedEngineOutcome()` 是恢复终结孤儿 entry 时使用的构造。
`engine.ExecutionInterrupted` 只能经由它抵达决策——`RunProgram` 永远不会返回它，
因为一个还能返回结果的进程并没有丢失观测。

### 辅助观测轴同样说「未知」

`InterruptedEngineOutcome()` 在录制与时间线两条轴上返回新增的
`RecordingUnobserved` / `TimelineUnobserved`，而不是 `DISABLED`。

理由与执行轴完全相同：`DISABLED` 断言功能被关掉了，而观测者已死的 entry
很可能正在录制、并留下了一个部分文件——宿主日后找到它，却无法与一条声称
「录制未启用」的记录对上。孤儿 entry 在**每一条轴**上都只能说未知。

对照：真的从未启动的 entry 确实没有录制，`NotStartedEngineOutcome()` 仍然且应当
返回 `DISABLED`。

### 状态轴上未变

`INTERRUPTED` 在状态轴上与 `NOT_STARTED` 相同：`NONE`→`FAILED`、
`CANCEL`→`CANCELED`、`ABORT`→`ABORTED`。区别整个走 cause 轴。

## 宿主需要做什么

1. 恢复路径改用 `InterruptedEngineOutcome()`，不要再用
   `NotStartedEngineOutcome()` 表达孤儿。
2. **持久化 `EntryCompletionDecision.TerminalCause`**，与 `entry_status` 并列。
   这是本次答复唯一要求宿主新增的存储列。
3. 失败率统计、自愈候选筛选、运行历史展示应按 cause 过滤，把
   `INTERRUPTED` 从「业务失败」里排除。

### 一处破坏性变更

`conformancetest.CompletionSnapshot` 增加了 `TerminalCause` 字段，宿主的 D-12
fixture 需要补一行。这是有意的：不读回它，宿主完全可以决策出 cause 却从不落盘，
而套件照样绿——D-18 就白做了。

## 源码与测试

- 源码：[`application/execution/entry_completion.go`](../../../application/execution/entry_completion.go)、[`application/engine/engine.go`](../../../application/engine/engine.go)
- 测试：[`application/execution/recovery_terminal_test.go`](../../../application/execution/recovery_terminal_test.go)、[`application/execution/entry_completion_test.go`](../../../application/execution/entry_completion_test.go)
