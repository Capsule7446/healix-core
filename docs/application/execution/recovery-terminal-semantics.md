# 恢复期终态语义

恢复流程使用 `InterruptedEngineOutcome()` 终结未能观测到运行结果的孤儿顶层执行项。该值与 `NotStartedEngineOutcome()` 语义不同：前者表示是否完成未知，后者表示引擎已知未启动。

## 两条结果轴

`EntryCompletionDecision` 同时携带执行结果和观测原因。`EntryStatus` 仍按七态机推进，`TerminalCause` 独立表达结果是否被观测：

| `ExecutionOutcome` | `TerminalCause` | 含义 |
|---|---|---|
| `SUCCEEDED` / `FAILED` / `CANCELED` | `COMPLETED` | 引擎执行并报告了结果。 |
| `NOT_STARTED` | `NOT_STARTED` | 已知引擎未开始。 |
| `INTERRUPTED` | `INTERRUPTED` | 运行未被观测到结束，是否完成未知。 |

`INTERRUPTED` 只由 `InterruptedEngineOutcome()` 构造；`RunProgram` 不返回该结果，因为能返回结果的进程没有丢失观测。录制和时间线轴同步返回 `RecordingUnobserved` 与 `TimelineUnobserved`，不能误报为 `DISABLED`。

## 状态决策

终态意图决定状态轴：`NONE` → `FAILED`，`CANCEL` → `CANCELED`，`ABORT` → `ABORTED`。`TerminalCause` 不受意图覆盖，始终随 `EntryCompletionDecision` 进入持久化结果。

## 宿主适配器要求

1. 发现孤儿 `RUNNING` entry 时构造 `InterruptedEngineOutcome()`。
2. 将 `EntryCompletionDecision.TerminalCause` 与 entry 状态一起原子持久化。
3. 业务失败统计和自愈筛选按 cause 过滤，`INTERRUPTED` 不计入已观测业务失败。

## 源码与测试

- 源码：[`application/execution/entry_completion.go`](../../../application/execution/entry_completion.go)、[`application/engine/engine.go`](../../../application/engine/engine.go)
- 测试：[`application/execution/recovery_terminal_test.go`](../../../application/execution/recovery_terminal_test.go)、[`application/execution/entry_completion_test.go`](../../../application/execution/entry_completion_test.go)
