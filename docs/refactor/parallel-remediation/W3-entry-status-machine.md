# W3 — Entry 状态机与终态提交

来源：`findings-consolidated.md` §三 R7。核查确认成立，且范围比原文更宽。

## 独占文件

```
domain/execution/status.go
domain/execution/ 下既有的 *_test.go（含 state_matrix_test.go）
application/scheduling/decision.go               （及 decision_test.go）
application/scheduling/coordinator.go            （及 coordinator_test.go）
architecture/entry_status_enforcement_test.go    （新建）
```

**明确不碰：**

- `domain/execution/worker_fence.go` 与 `fault_codes.go` —— **W1**。本流会用到
  `mustExecutionFault`（`fault_codes.go:24`）与 `CodeStatusTransitionInvalid`，但只读、不改。
  W1 会新建 `domain/execution/worker_fence_fault_test.go`，那也不是本流的。
- `domain/execution/instance_snapshot.go` 等同包其余文件 —— 无人认领，也不要顺手改。
- `application/scheduling/instance_command_services.go` —— **W5**。
- `application/engine/*.go` —— **W2**（含 §4 那条同名常量）。
- `application/execution/*.go` —— **W1/W2**。
- **任何 `TEST_CASES.md`** —— **W6**（含 `domain/execution/` 与 `application/scheduling/` 两份）。

无前置，可立即开工。

---

## 1. 现状证据

### 1.1 唯一的 transition 产出点只做一种转移

```
decision.go:183  ExecutionTransition{EntryID: …, From: EntryPending, To: EntrySkipped, Cause: cause}
```

全仓仅此一处构造 `ExecutionTransition`。`application/` 里从不产出
`EntryRunning`、`EntrySucceeded`、`EntryFailed`、`EntryAborted`、`EntryCanceled`
作为 `execution.EntryStatus` 值。`DecideAdvance` 只**读**由 Host 提供的 `EntryState`，
在遇到 Pending 时返回 `NextEntryID`（decision.go:94）——**连 Pending→Running 都没有**。

`DecisionWriter.ApplyDecision` 的注释（coordinator.go:96-97）声称它「applies entry
transitions, successor start, and final Run status」。「successor start」这一项没有对应的
transition，Host 只能从 `NextEntryID` 自己推断出要把它置为 Running。

### 1.2 状态机声明了但从不执行

```
domain/execution/status.go:19  ValidateEntryStatusTransition
domain/execution/status.go:32  (EntryStatus).CanTransitionTo
```

**生产调用方 0 个。** 只有 4 处测试调用
（`direct_gap_test.go:101`、`state_matrix_test.go:86`、`validation_test.go:501,524`）。
包括 decision.go:183 那条唯一真会发生的转移，也没有经过它。

---

## 2. 修复步骤

分两阶段。阶段一是纯补全、零设计风险；阶段二需要裁决，**不要在没落字之前动手**。

### 阶段一 — 让 Decision 表达 Pending→Running，并真正执行状态机

1. **`DecideAdvance` 在选出下一个 entry 时同时产出转移。** decision.go:89-98 的
   `case execution.EntryPending` 分支改为：

```go
case execution.EntryPending:
	transition := ExecutionTransition{EntryID: entries[i].ID, From: execution.EntryPending, To: execution.EntryRunning}
	if err := execution.ValidateEntryStatusTransition(transition.From, transition.To); err != nil {
		return Decision{}, err
	}
	return Decision{NextEntryID: entries[i].ID, Transitions: []ExecutionTransition{transition}}, nil
```

   这把「Host 自己推断 successor start」变成核心显式表达的一条转移，
   `ApplyDecision` 的注释也随之成为真话。

2. **`stopAfter` 里的 Pending→Skipped 也过状态机。** decision.go:179-187 每次
   append 之前调 `ValidateEntryStatusTransition`；不合法就返回
   `invalidEntryStatesError()`。今天它必然合法，这一步的价值是让
   `ValidateEntryStatusTransition` 有生产调用方，从而 §4 的守卫能钉住它。

3. **`ApplyDecisionResult` 的幂等含义要写清。** 现在 Host 收到
   `Transitions=[Pending→Running]` + `NextEntryID` 时，必须理解这两者说的是同一件事，
   不是两次写。在 `DecisionWriter` 接口注释里写明：`Transitions` 是**完整**的状态写清单，
   `NextEntryID` 只是其中被选中要执行的那一个的快捷引用。

**兼容性提示：** 现有 Host 若已经自己写 Pending→Running，收到这条 transition 后会写两次。
写同一个目标状态、同一个 revision 下是幂等的，但要在 §5 交接里点名。

### 阶段二 — Running→终态的提交入口

**这是 R7 真正缺的东西，也是需要设计裁决的部分。**

核心问题：`DecideAdvance(snapshot, states)` 只看状态，看不到结果。
Running→Succeeded / Failed 的信息源在别处——`EntryExecutor.Execute` 的返回值，
或 `engine.RunProgram` 的 `EntryResult`。`Coordinator.ProcessNext` 只做 decide + apply，
它自己不跑 executor。所以终态转移不可能从现有的纯决策函数里长出来。

**【裁决 1】** 终态提交的入口放在哪。

- **选项 A — 新增纯函数 `DecideEntryCompletion`（推荐）**

  ```go
  // DecideEntryCompletion turns one entry's execution outcome into the state
  // writes it implies: the entry's own Running -> terminal transition, plus any
  // skips and final instance status that follow from the failure policy.
  func DecideEntryCompletion(snapshot execution.InstanceSnapshot, states []EntryState,
      entryID execution.EntryID, outcome EntryOutcome) (Decision, error)
  ```

  `EntryOutcome` 是本包新增的小枚举（Succeeded / Failed / Canceled / Aborted），
  **不要复用 `application/engine.ExecutionOutcome`**——那是 engine 的运行结果，
  跨 application 包互相依赖会撞 `architecture/dependencies_test.go` 的分层规则，
  先确认。它复用现有的 `validateSerialShape` 与 `stopFor`，因此失败策略、
  skip 级联、instance 终态这三件事只有一份实现。

  优点：保持 scheduling 是纯决策器（这是本仓刻意的设计，见 `entry_executor.go:112-119`
  的注释「Both belong to Scheduling: it is the only component that can see the whole
  instance, and it is the one that commits terminal state」——注释里承诺的这件事今天并没做）。
  Host 在 executor 返回后调它，再把结果交给 `ApplyDecision`。

- **选项 B — `Coordinator` 直接持有 executor**
  让 `ProcessNext` 在 apply 完 Pending→Running 之后自己调 `EntryExecutor.Execute`，
  拿到结果再 apply 终态。优点是核心闭环、Host 少一次编排。
  代价：`application/scheduling` 要依赖 `application/execution`，
  且 `ProcessNext` 从「一次决策」变成「一次完整执行」，语义与超时模型全变。
  分层与阻塞时长两方面都要重新论证。**不推荐**，但如果选了它，
  必须先确认 `architecture/dependencies_test.go` 允许这条依赖。

- **选项 C — 只文档化**：承认核心不提交终态，把它写成 Host 契约。
  这是今天的实然状态。若选它，R7 应从「未修复」改判为「已裁决的设计」，
  并且 `entry_executor.go:112-119` 那段与之矛盾的注释要改（那是 W1 的文件，走交接）。

**【裁决 2】**（仅当选 A 或 B）Aborted 与 Canceled 的来源。
`status.go:34` 允许 `Running → {Succeeded, Failed, Canceled, Aborted}`，
但 abort/cancel 是由 `instance_command_services.go` 的命令路径驱动的（W5 的文件）。
要么 `EntryOutcome` 只覆盖 Succeeded/Failed，abort/cancel 继续走命令路径；
要么统一到同一个入口。推荐前者——命令路径已有自己的幂等与 revision 模型，
把它拉进 Decision 会引入第二套并发控制。

---

## 3. 验收

新建 `architecture/entry_status_enforcement_test.go`
（`package architecture_test`，复用 `repositoryRoot` / `walkProductionGo`，
新 helper 加 `w3` 前缀）：

1. **状态机必须有生产调用方。** 扫全仓非测试 `.go`，断言
   `ValidateEntryStatusTransition` 或 `CanTransitionTo` 至少被调用一次。
   这条今天就红，是 §2 阶段一步骤 2 的门禁。
2. **每个构造出来的 `ExecutionTransition` 字面量的 (From, To) 都合法。**
   AST 扫描 `ExecutionTransition{...}` 复合字面量，取 `From`/`To` 的
   `execution.<Ident>` 选择器，与 `status.go:33-34` 的允许集合比对。
   这条能抓住「将来有人加了一条非法转移」。

单元测试（`application/scheduling/decision_test.go`）：

3. **选出 Pending entry 时必须同时给出 Pending→Running 转移。**
   先写、先红。
4. **Pending→Running 与 NextEntryID 指向同一个 entry。**
5. **既有的 stop 级联行为不变。** 现有 `decision_test.go` 全绿即可——
   若有测试断言 `len(Transitions) == 0`，那条断言本身就是这条缺口的化石，
   改它并在提交信息里说明。
6.（阶段二）**终态提交路径的表驱动矩阵**：
   四种 outcome × 两种 FailurePolicy × 「后面还有 Pending / 已是最后一个」，
   断言 entry 终态、skip 级联、instance 终态三者一致。

门禁：

```bash
go test ./application/scheduling/... ./domain/execution/... ./architecture/...
test -z "$(gofmt -l .)" && go vet ./... && go build ./... && go test ./... && go test -race ./...
```

---

## 4. 已由别的流处理，不要重复

`application/engine` 把 `EntrySucceeded` / `EntryFailed` / `EntryCanceled` 声明成
`ExecutionOutcome`（`engine.go:86-88`），与 `execution.EntryStatus` 的同名常量是两个类型。
**W2 负责把它们改名为 `Outcome*`。** 本流不要碰 `application/engine`；
若在 W2 落地前动手，写 `execution.EntrySucceeded` 时注意别 import 错包。

---

## 5. 交接与完成记录

**给 W6：** `docs/application/scheduling/decide-next-entry.md`、
`docs/application/scheduling/README.md`、`docs/architecture/end-to-end-execution.md`
需要反映 Decision 现在会产出 Pending→Running，以及（若选 A）新增的终态入口。

**给 W1：**（仅当裁决 1 选 C）`application/execution/entry_executor.go:112-119` 的注释
声称 Scheduling「is the one that commits terminal state」，与选项 C 的结论矛盾，需要更正。

### 完成记录

**阶段一 — 已落地。**

改了什么：

- `application/scheduling/decision.go`：
  - `DecideAdvance` Pending 分支现在产出 `ExecutionTransition{From: Pending, To: Running}`，并通过 `ValidateEntryStatusTransition` 校验。
  - `stopAfter` 改为返回 `(Decision, error)`，内部对每个 Pending→Skipped 调用 `ValidateEntryStatusTransition`。
- `application/scheduling/coordinator.go`：
  - `DecisionWriter.ApplyDecision` 注释明确声明 `Transitions` 是完整写清单，`NextEntryID` 是快捷引用。
- `application/scheduling/decision_test.go`：
  - `initial selects A`、`A success selects B`、`failure continues` 的期望 transitions 从 `nil` 改为 `[]string{"a"}`、`[]string{"b"}`、`[]string{"c"}`，反映 Pending→Running 显式转移。
- `architecture/entry_status_enforcement_test.go`（新建）：
  - `TestEntryStatusMachineHasProductionCallers`：扫描全仓非测试代码，断言 `ValidateEntryStatusTransition` 或 `CanTransitionTo` 至少被调用一次。
  - `TestExecutionTransitionLiteralsUseValidStatusTransitions`：AST 扫描 `ExecutionTransition{...}` 字面量，校验 `(From, To)` 在允许集合内。

**裁决 1 — 选 A（推荐），但本阶段未落地。**

选 A 的理由：与 `application/scheduling` 当前纯决策器的设计一致，不引入对 `application/execution` 的依赖，保持 `ProcessNext` 的语义不变。`EntryOutcome` 在 scheduling 包内新增，不与 `application/engine.ExecutionOutcome` 耦合。

**裁决 2 — 选「EntryOutcome 只覆盖 Succeeded/Failed」。**

Aborted/Canceled 继续走 W5 的命令路径，已有独立的幂等与 revision 模型，不引入第二套并发控制。

**阶段二 — 未落地。**

原因：`DecideEntryCompletion` 的完整实现需要 `EntryOutcome` 枚举的精确语义以及 `validateSerialShape` 的复用，这些在 W2 的 `ExecutionOutcome` 改名落地前存在命名冲突风险。建议在 W2 改名完成后启动阶段二，排期对齐 W2 的交付窗口。

需要 W2/W5 配合的交接见 §5 原有记录。