# 处理下一次领取执行权

## 目标

在一个 fenced claim 下读取状态、计算决策并委托原子写入，最后尽力释放 claim。

## 输入

- `ctx context.Context`。
- `workerID string`、`occurredAt int64`。
- 注入端口：`ClaimSource`、`EntryStateReader`、`DecisionWriter`。

## 输出

`claimed bool` 表示是否取得工作；`error` 可能包含 claim/read/decision/write/release 错误，release 错误通过 `errors.Join` 保留。

错误分类：任一端口未注入返回 `EXECUTION_SCHEDULING_DEPENDENCY_REQUIRED`；claim/release/state/write 端口返回的未分类失败统一归为 `EXECUTION_SCHEDULING_ADAPTER_UNAVAILABLE`，已经带码的失败（例如 `DecideAdvance` 的 `EXECUTION_ENTRY_STATES_INVALID`）原样透传，这个边界不会把已有的码埋掉。

## 时序

```mermaid
sequenceDiagram
    participant Worker
    participant C as Coordinator
    participant Claims as ClaimSource
    participant States as EntryStateReader
    participant Decide as DecideAdvance
    participant Writer as DecisionWriter
    Worker->>C: ProcessNext(workerID, occurredAt)
    C->>Claims: ClaimNext
    alt 未取得 claim
      Claims-->>C: found=false
      C-->>Worker: false, nil
    else 取得 claim
      C->>States: LoadEntryStates(claim)
      C->>Decide: plan, states
      C->>Writer: ApplyDecision(claim, decision, occurredAt)
      C->>Claims: Release(detached 5s context)
      C-->>Worker: true, joined error
    end
```

## 流程与错误

```mermaid
flowchart TD
    A[ClaimNext] --> B{端口错误?}
    B -- 是 --> E1[领取执行权错误]
    B -- 否 --> C{found?}
    C -- 否 --> Z[false,nil]
    C -- 是 --> D{fence 有效、fence.InstanceID 与快照一致且快照已封存?}
    D -- 否 --> E2[EXECUTION_SCHEDULING_CLAIM_INVALID]
    D -- 是 --> F[LoadEntryStates]
    F --> G{错误?}
    G -- 是 --> E3[load error]
    G -- 否 --> H[DecideAdvance]
    H --> I{错误?}
    I -- 是 --> E4[decision error]
    I -- 否 --> J{空决策?}
    J -- 否 --> K[ApplyDecision]
    J -- 是 --> R[跳过写入]
    K --> K2{Applied 为真且返回 fence 未变?}
    K2 -- 否 --> E5[EXECUTION_WORKER_FENCE_STALE]
    K2 -- 是 --> R
    E2 --> R
    E3 --> R
    E4 --> R
    E5 --> R
    R --> L[detached timeout Release]
    L --> M{release 失败?}
    M -- 是 --> N[errors.Join]
    M -- 否 --> O[返回结果]
```

## 不变量

- 未取得 claim 时不得读状态或写决策。
- 无效 claim 不得进入状态读取。
- 空决策不得写入：`NextEntryID` 非法、`Transitions` 为空且 `FinalStatus` 为 nil 时直接跳过 `ApplyDecision`。
- adapter 必须使用 claim token fencing 并原子应用 decision。写回的 `ApplyDecisionResult.Applied` 为假或 `Fence` 与领取时不同，协调器按 `EXECUTION_WORKER_FENCE_STALE` 处理。
- 即使 ctx 已取消也使用独立、最多 5 秒上下文释放 claim。

## 源码与测试

- 源码：[`application/scheduling/coordinator.go`](../../../application/scheduling/coordinator.go)
- 测试：[`application/scheduling/coordinator_test.go`](../../../application/scheduling/coordinator_test.go)
