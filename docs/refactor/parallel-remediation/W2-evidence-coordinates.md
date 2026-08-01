# W2 — 证据坐标与 occurrence

来源：`findings-consolidated.md` §三 R6，外加本轮核查在同一片代码里发现的
`application/engine` 同名常量陷阱（因落在本流独占文件内，一并处理）。

**本流改动面最大，且含公共结构体的破坏性变更。先读完 §1 再动手——R6 的原始表述有一半是错的。**

## 独占文件

```
domain/evidence/*.go                                  （含测试）
domain/node/runtime.go  step.go  composite.go  validation.go （及其测试）
application/engine/*.go                               （engine.go coordinator.go compiler.go …含测试）
application/execution/ 下除 entry_executor*.go 之外的全部 .go：
    ports.go  ports_test.go  heal_governance.go  heal_governance*_test.go
    commit_ownership_test.go  fence_conformance_test.go  conformancetest/**
contract/public_api_test.go                           （仅 L118）
architecture/evidence_coordinate_test.go              （新建）
```

**前置：** `README.md` §0 必须先完成。`ports.go` 与 `ports_test.go` 现有未提交改动
（字节预算 1<<18 → 1<<20 的回退）。

**明确不碰：**
`application/execution/entry_executor.go` · `entry_executor_test.go` · `entry_authorization_test.go`（W1）、
`domain/execution/*.go`（W1/W3）、`application/scheduling/*.go`（W3/W5）、
以及**任何 `TEST_CASES.md`**（W6，含 `domain/evidence/`、`domain/node/`、
`application/engine/`、`application/execution/` 四份）。

---

## 1. 现状：R6 的表述需要更正

原文：「Evidence 缺 InvocationPath，retry 无独立 occurrence」。

**前半句成立。** `InvocationPath` 在 `domain/evidence` 出现 0 次；全仓只在 `domain/execution`、
`application/engine`、`application/scheduling` 用到。

**后半句不成立。** occurrence 存在，而且是真的计数器（`domain/node/runtime.go:341-372`，
每次 `PhaseRunning` 自增，`activeOccurrence` 维护一个栈以支持嵌套）。实际分布是：

| 类型 | Occurrence | EntryID | StepExecutionID |
|---|---|---|---|
| `evidence.StepProgressEvent` / `StepPhaseEvent` | **有**（且 `Validate` 要求 > 0） | 有 | 有 |
| `evidence.HealObservation` | 无 | 有 | 有 |
| `evidence.ValidationObservation` / `ValidationProgressObservation` | 无 | 有 | 有 |
| `evidence.ValidationGroupTerminalObservation` | 无 | 有 | 有 |
| `evidence.HealCandidateReset` | 无 | 有 | 有 |
| `evidence.StepFact` | 无 | 有 | 有 |
| `node.Event` | 有 | **无** | 无 |
| `node.OperationObservation` | 无（只有 `Attempt`） | **无** | **无** |

所以真正的缺口是三条，不是一条：

1. **occurrence 只到两个 event 类型为止**，所有 observation 与 fact 都没有它。
   提交时靠 `StepTransitionCommit.Validate` 把观察绑到 event 上（`commits.go:102,123,154,204`
   校验 `StepExecutionID` 与 `EntryID` 相等），但**落库后的事实本身丢了 occurrence**。
2. **`node.OperationObservation` 连 EntryID 都没有**。一个 instance 下 N 个 entry，
   它只能定位到 `NodeID`。`Attempt` 是单次调用内的重试计数
   （`step.go:127,136,152,188`），不是 occurrence 序号。
3. **调用点信息没丢，只是塞进了字符串。**
   `runtimeInvocationStepID`（`compiler.go:490`）= `"step|" + 长度前缀调用路径 + 长度前缀 stepID`，
   根路径就是 entryID（`compiler.go:171`）。也就是说 `NodeID` 正是 evidence 层缺的那个
   `InvocationPath`，以 stringly-typed 形式存在。

底层结构事实：**两套并行证据模型，核心里没有映射器。**
`domain/node` 按 `(InstanceID, NodeID string, Occurrence int)` 编址，
`domain/evidence` 按 `(InstanceID, EntryID, StepExecutionID)` 编址，两者之间的转换全在
Host 手里，核心不施加任何约束。

比 retry 更硬的例子：`RepeatNode`（`composite.go:184-207`）子节点只编译一次、执行 `Times` 次，
每轮的 NodeID 完全相同——重复迭代产生的观察目前无法互相区分。

---

## 2. 修复步骤

### 步骤 1 — `node.Runtime` 拿到 EntryID

`application/engine/engine.go` 的 `Config` **已经有** `EntryID`（L59），只是没往下传。

1. `domain/node/runtime.go`：`Runtime` 结构体加 `EntryID domainexecution.EntryID`。
2. `application/engine/coordinator.go` 的 `newRuntime`（L158-175）加一行
   `EntryID: cfg.EntryID,`。

`RunProgram` 在 L110-118 已经断言 `cfg.EntryID == entry.identity.entryID`，所以到这里的
EntryID 是已校验的权威身份，不需要再验一次。

### 步骤 2 — `OperationObservation` 带上坐标

`domain/node/runtime.go` 的 `OperationObservation`（L123-134）加两个字段：

```go
EntryID    domainexecution.EntryID
Occurrence int
```

六个构造点全部补齐：`composite.go:114`、`step.go:127`、`step.go:136`、`step.go:152`、
`step.go:188`、`validation.go:108`。

- `EntryID` 一律取 `rt.EntryID`。
- `Occurrence` 取 `rt.activeOccurrence(nodeID)`（L308-314）的返回值。
  注意该函数在无活跃 occurrence 时返回 error；观察是 best-effort 的
  （`observeOperationBestEffort`），**取不到时不要让它把执行打断**——取 0 并让守卫
  测试去保证「运行中的节点一定有活跃 occurrence」。
- `step.go:152` 的 locate 观察用了 `context.WithoutCancel`，occurrence 的取法与其他五处相同，
  但要确认 nodeID 与当时活跃的那一个一致。

### 步骤 3 — occurrence 下沉到 observation 与 fact

`domain/evidence`：给下列类型各加 `Occurrence int`，并在其 `Validate()` 里要求 `> 0`
（与 `StepProgressEvent.Validate` L40-42 同款措辞，复用现有 `VALIDATION_FIELD_INVALID` 违例码，
**不新增任何顶层 code**）：

```
HealObservation                       observations.go:65
ValidationObservation                 observations.go:330
ValidationProgressObservation         observations.go:295
ValidationGroupTerminalObservation    observations.go:216
HealCandidateReset                    commits.go:12
StepFact                              facts.go:27
```

`ValidationGroupTerminalObservation` 有构造函数 `NewValidationGroupTerminalObservation`
（observations.go:225），签名要一并加参数。

### 步骤 4 — 提交时交叉校验 occurrence

`domain/evidence/commits.go` 现有四处交叉校验只比对身份，扩展为同时比对 occurrence：

| 行 | 现有 | 增加 |
|---|---|---|
| L102 | `validation.StepExecutionID != c.Event.ID \|\| validation.EntryID != c.Event.EntryID` | `\|\| validation.Occurrence != c.Event.Occurrence` |
| L123 | `heal.StepExecutionID != c.Event.ID \|\| heal.EntryID != c.Event.EntryID` | 同上 |
| L154 | `reset.EntryID != c.Event.EntryID \|\| reset.StepExecutionID != c.Event.ID` | 同上 |
| L204 | `group.StepExecutionID != event.ID \|\| group.EntryID != event.EntryID` | 同上 |

这样一次提交里的全部观察必须属于同一个 occurrence，跨轮混入会在域层就被拒。

### 步骤 5 — `InvocationPath` 进入证据身份

给 `StepProgressEvent`（events.go:17）与 `StepPhaseEvent`（events.go:52）各加
`InvocationPath execution.InvocationPath`，`Validate()` 里调 `p.Validate()`。

**【裁决 1】** `StepFact` 要不要也加。它是独立持久化的事实，不经 event 绑定，
所以从「能否独立定位一次调用」的角度应该加。但它已经有 `EntryID + StepExecutionID`，
加了就有三份重叠身份。
- 选项 A（推荐）：加。理由是 `StepExecutionID` 是 Host 生成的不透明串，核心不约束它是否
  按调用点唯一；只有 `InvocationPath` 是核心自己算得出、可复算的坐标。
- 选项 B：不加，改为在文档里要求 Host 保证 `StepExecutionID` 逐 occurrence 唯一。
  代价是把一条核心能强制的不变量降级成一句口头约定。

**【裁决 2】** 要不要在核心里补上 `node` → `evidence` 的映射器，
把 `NodeID` 反解成 `(EntryID, InvocationPath)`。
- 选项 A：补。`compiler.go` 已有 `Metadata map[string]StepMetadata`（键就是 runtimeID），
  再存一份 `InvocationPath` 即可，`CompiledEntry` 已经暴露 `Metadata`。
  这样 Host 不必自己解析 `"step|12:entry-1/4:s001"` 这种格式。
- 选项 B（较小）：只在 `StepMetadata` 里加 `InvocationPath execution.InvocationPath` 字段，
  不做完整映射器，由 Host 查表。
- 推荐 B：范围可控，且把「解析 NodeID 字符串」这件事从 Host 手里拿走，
  这是本条缺口最实际的危害。

### 步骤 6 — 顺手修 `application/engine` 的同名常量陷阱

`engine.go:86-88` 声明的 `EntrySucceeded` / `EntryFailed` / `EntryCanceled` 是
`ExecutionOutcome` 类型，与 `domain/execution` 的 `EntryStatus` 常量**同名不同类型**，
且两者之间没有任何映射。改名为 `OutcomeSucceeded` / `OutcomeFailed` / `OutcomeCanceled`
（`ExecutionNotStarted` 已经不叫 `Entry*`，改后四个常量命名一致）。

同步：`coordinator.go:150-155` 的 `executionOutcome()`、`contract/public_api_test.go:118`、
以及 `application/engine/` 下引用它们的测试。

> 这一条不修也不会错，但它是 R7（W3）真正修 EntryStatus 时最容易踩的坑：
> 在 scheduling 里写 `EntrySucceeded` 与在 engine 里写 `EntrySucceeded` 是两回事。
> 因为落在本流独占文件内，由本流处理。

---

## 3. 破坏性变更与影响面

evidence 结构体新增**必填**字段，是 Host 侧的运行期破坏性变更：
keyed struct literal 不会编译失败，但 `Validate()` 会开始拒绝旧的调用。

必须一并修的仓内构造点（用这条找全）：

```bash
grep -rln "evidence\.\(HealObservation\|ValidationObservation\|ValidationProgressObservation\|ValidationGroupTerminalObservation\|HealCandidateReset\|StepFact\|StepProgressEvent\|StepPhaseEvent\)" --include=*.go .
```

已核实的结果是 **9 个文件，全部在 `application/execution/`，且全部在本流名下**：

```
application/execution/commit_ownership_test.go
application/execution/conformancetest/reference_test.go
application/execution/conformancetest/suite.go          （L205-220 是主要构造点）
application/execution/fence_conformance_test.go
application/execution/heal_governance.go
application/execution/heal_governance_matrix_test.go
application/execution/heal_governance_test.go
application/execution/ports.go
application/execution/ports_test.go
```

**这正是本流拿到整个 `application/execution`（除 `entry_executor*`）的原因**——
evidence 类型的构造面完全落在这一个包里，拆开就一定要跨界改别人的文件。
`entry_executor_test.go` 已核实零 `evidence.` 引用，所以 W1 那三个文件是安全的切口。

若这条 grep 将来命中 `application/scheduling/**` 或 `entry_executor*`，**停手**——
那是别人的文件，把需要的改动写进 §5 交接。

> `ports.go` 的 `validateCommitInstanceBinding`（L272+）只**读** `observation.InstanceID`，
> 加字段不影响它的逻辑；但 `ports_test.go` 里的 fixture 会被新的
> `Occurrence > 0` 校验拒掉，需要补 occurrence。
>
> 另：W1 会让 `WorkerFence.Validate()` 返回 coded fault，`ports.go:249-252` 那句
> 「Both validators return their own classified faults」随之自动成立。**不要重写它。**

---

## 4. 验收

新建 `architecture/evidence_coordinate_test.go`（`package architecture_test`，
复用 `repositoryRoot` / `walkProductionGo`，新 helper 一律加 `w2` 前缀）：

1. **占位守卫。** 扫 `domain/evidence`，断言每个含 `EntryID execution.EntryID` 字段的
   结构体同时含 `Occurrence int`。这条把「下次新加一个观察类型又忘了带 occurrence」
   变成红灯，而不是又一次静默缺失。
2. **节点侧守卫。** 断言 `node.OperationObservation` 同时含 `EntryID` 与 `Occurrence`。
3. **NodeID 不再是唯一坐标来源。** 断言 `application/engine` 的 `StepMetadata`
   含 `InvocationPath`（若裁决 2 选 B）。

单元测试：

4. **RepeatNode 的两轮迭代产生不同 occurrence。** 用一个记录全部
   `OperationObservation` 的假 observer，跑一个 `Times: 2` 的 repeat，
   断言两轮的 `Occurrence` 不同。**这条要先写、先红**——它是 R6 的核心场景，
   现在必然失败。
5. **同一 entry 内两个调用点的同一 fragment 可区分。** 编译一个引用同一
   `WorkflowVersionID` 两次的 entry，断言两处的 `StepMetadata.InvocationPath` 不同。
6. **跨 occurrence 的观察被提交拒绝。** 构造一个 event occurrence 为 2、
   heal observation occurrence 为 1 的 `StepTransitionCommit`，断言 `Validate()` 拒绝。

门禁：

```bash
go test ./domain/evidence/... ./domain/node/... ./application/engine/... ./application/execution/... ./architecture/... ./contract/...
test -z "$(gofmt -l .)" && go vet ./... && go build ./... && go test ./... && go test -race ./...
```

---

## 5. 交接与完成记录

**给 W6：** `docs/domains/evidence.md`、`docs/application/execution/*.md`、
`docs/architecture/end-to-end-execution.md` 需要反映新的证据身份三元组
`(EntryID, InvocationPath, Occurrence)`，以及 `ExecutionOutcome` 常量改名。本流不碰这些文件。

**给 W1：** `ports.go` 与 `ports_test.go` 已归本流，evidence 新字段的波及本流自己吸收，
不需要 W1 配合。若发现 `entry_executor.go` 因新字段需要改动（预期不会——它零 `evidence.` 引用），
写在这里，不要自己动。

**给 W3：** `engine.EntrySucceeded` 等已更名为 `OutcomeSucceeded`；在 scheduling 侧写
`execution.EntrySucceeded` 时不会再有歧义。

### 完成记录

> 由执行者填写：实际改了什么 / 跑过哪些验收 / 裁决 1 与裁决 2 各选了什么、为什么 /
> 哪些 Host 侧破坏性变更需要下游配合。
