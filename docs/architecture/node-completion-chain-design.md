# 节点完成后阻塞式处理链实施计划

> 状态：**已实现**。核心契约与统一叶子生命周期位于 `domain/node/lifecycle.go`。

## 1. 目标

在每个叶子 Node 完成后、下一个 Node 开始前，执行一个阻塞式处理链：

```text
叶子 Node A 完成
→ 固化 A 的执行结果快照
→ 按注册顺序执行 Completion Handler Chain
→ 等待全部 Handler 结束
→ 严格按照 A 的原始结果继续或终止执行
```

处理链必须满足：

- 以叶子 Node 的一次完整 `Run` 为触发边界。
- Handler 严格串行执行，全部结束前不得开始下一个 Node。
- Handler 只能读取节点完成快照。
- Handler 的成功或失败不改变 Node 结果和主执行链决策。
- 一个 Handler 失败后继续执行后续 Handler。
- 第一阶段只提供显式的只读浏览器能力，不开放完整浏览器 Driver。

## 2. 边界

- Handler 不能修改 Node、Runtime、执行计划或变量。
- Handler 不能通过 `ReadOnlyBrowser` 改变页面状态。
- Handler 错误与 Node 执行错误独立记录。
- Handler 不能控制重试、跳过、中止或继续。
- Handler 固定串行执行，运行中不能动态增删。
- Workflow、Repeat、WorkflowCall 等组合 Node 不执行处理链。
- 如需在叶子完成后读取 Core 暴露的状态，可实现 `NodeCompletionHandler` 并仅使用 `NodeCompletionContext` 提供的能力。

## 3. 节点边界

处理链只按照 Node 类型判断，不分析 Node 内部操作：

- `StepNode` 是叶子 Node。
- 独立执行的 `WaitNode` 是叶子 Node。
- 独立执行的 Validation Node 是叶子 Node。
- Workflow、Repeat、WorkflowCall 及其他编排节点不是叶子 Node。

内部 Retry 属于同一次 Node 执行，不重复触发。Repeat 每次重新执行叶子 Node 都产生新的 occurrence，并分别触发处理链。

## 4. 节点完成快照

```go
type NodeOutcome string

const (
    NodeOutcomeSucceeded NodeOutcome = "SUCCEEDED"
    NodeOutcomeFailed    NodeOutcome = "FAILED"
    NodeOutcomeCanceled  NodeOutcome = "CANCELED"
    NodeOutcomeSkipped   NodeOutcome = "SKIPPED"
)

type NodeExecutionRef struct {
    RunID      string
    NodeID     string
    Occurrence int
}

type ExecutionErrorSnapshot struct {
    Kind    string
    Message string
}

type NodeExecutionSnapshot struct {
    Execution NodeExecutionRef
    NodeKind  string
    Outcome   NodeOutcome

    StartedAt   time.Time
    CompletedAt time.Time
    Duration    time.Duration

    Error *ExecutionErrorSnapshot
}
```

约束：

- 快照在 Node 原始结果确定后构造。
- 快照是不可变值，不暴露 `Node`、`Runtime`、`Driver` 或 `Element`。
- `Error` 只描述 Node 原始错误，不包含 Handler 错误。
- 复合字段必须深复制，不能共享 Core 内部可变数据。
- 时间字段只统计 Node 自身执行，不包含处理链耗时。

## 5. 只读浏览器能力

Completion Handler 不接收完整 `Driver`，而是接收一个按能力拆分的只读端口。第一阶段允许以下能力：

- 截取当前页面；
- 获取当前页面的只读 DOM 快照；
- 按 `NodeSpec` 定位元素并读取存在性、可见性、文本和属性。

这些能力对应 Core 当前已经存在的 `Driver.Snapshot`、`Locator` 和 `Reader` 语义，不引入浏览器写操作。

```go
type ScreenshotOptions struct {
    FullPage bool
}

type ScreenshotArtifact struct {
    MediaType string
    Data      []byte
}

type ElementObservation struct {
    Exists    bool
    Visible   bool
    Text      string
    Attributes map[string]string
}

type ReadOnlyBrowser interface {
    CaptureScreenshot(
        context.Context,
        ScreenshotOptions,
    ) (ScreenshotArtifact, error)

    SnapshotDOM(context.Context) (heal.DOMSnapshot, error)

    ObserveElement(
        context.Context,
        fingerprint.NodeSpec,
        []string,
    ) (ElementObservation, error)
}
```

约束：

- 所有方法只能读取当前页面状态。
- 不扩张现有 `Driver`，也不把 `Driver`、`Element` 或 `Locator` 直接交给 Handler。
- `ObserveElement` 在 Core 内完成定位和读取，只返回深复制的值对象，不返回实时 `Element` 句柄。
- `ObserveElement` 应使用 Runtime 的有效 selector overlay，保证与正常执行时的定位语义一致。
- 属性名由调用方显式提供，避免无界读取全部属性。
- `SnapshotDOM` 返回的端口只允许枚举快照候选；不得通过快照取得实时页面句柄。
- `ScreenshotArtifact.Data`、`ElementObservation.Attributes` 及 DOM 快照中的复合数据返回后不得与 Core 共享可变底层存储。
- 不提供点击、输入、导航、按键、执行脚本、等待条件、轮询、Cookie、Storage 或页面切换能力。
- 只读调用与正常 Node 执行不得并发访问浏览器会话。
- 新增只读能力必须单独评估，不能通过扩张为通用 Driver 访问来绕过边界。

## 6. Completion Handler Chain

```go
type NodeCompletionContext struct {
    Snapshot NodeExecutionSnapshot
    Browser  ReadOnlyBrowser
}

type NodeCompletionHandler interface {
    Name() string
    Handle(context.Context, NodeCompletionContext) error
}

type CompletionHandlerResult struct {
    HandlerName string
    StartedAt   time.Time
    CompletedAt time.Time
    Error       *ExecutionErrorSnapshot
}

type NodeCompletionChain struct {
    handlers []NodeCompletionHandler
}
```

执行规则：

1. 注册顺序就是执行顺序。
2. 执行开始前固定 Handler 列表，运行中不得改变。
3. 前一个 Handler 返回后才能执行下一个 Handler。
4. Handler 返回错误时记录结果，但继续执行后续 Handler。
5. 所有 Handler 结束后，Chain 才返回。
6. 空 Chain 不产生额外行为。
7. Chain 的返回结果只用于观测，不参与主执行链判断。

Handler 可以根据 `Snapshot.Outcome` 自行决定是否执行。例如只在成功后截图；Core 不为成功、失败分别维护不同 Chain。

## 7. 主执行链不变量

Core 必须先保存 Node 原始结果，再运行 Chain：

```go
nodeErr := runLeafNode(ctx, rt, leaf)
snapshot := buildNodeExecutionSnapshot(leaf, nodeErr)
handlerResults := rt.runNodeCompletionChain(snapshot)
rt.observeNodeCompletionHandlers(snapshot.Execution, handlerResults)
return nodeErr
```

以上代码只表达顺序，不限定最终函数结构。

结果矩阵：

| Node 原始结果 | Handler 结果 | 主执行链行为 |
|---|---|---|
| 成功 | 全部成功 | 继续下一个 Node |
| 成功 | 存在失败 | 记录失败，继续下一个 Node |
| 失败 | 全部成功 | 保持 Node 失败 |
| 失败 | 存在失败 | 记录失败，保持 Node 失败 |
| 取消 | 任意 | 保持 Node 取消 |
| 跳过 | 任意 | 保持原跳过语义 |

唯一允许的行为差异是节点之间增加处理链耗时。

## 8. 超时与取消

处理链是阻塞式的，但不能无限阻塞：

- 每个 Handler 使用独立的有界 context。
- Node context 已取消时，使用 `context.WithoutCancel` 创建有界处理 context。
- 单个 Handler 超时时，Core 取消该 Handler 的 context；Handler 返回后记录失败，再决定是否执行下一个 Handler。
- Chain 可配置总超时；Core 在启动每个 Handler 前检查总超时，达到上限后不再启动剩余 Handler，主链保持 Node 原始结果。
- Core 不在后台运行 Handler，也不强制终止 Handler goroutine；`Handle` 是同步调用。
- Handler 必须正确响应 context 取消并及时返回；不响应 context 的 Handler 会继续阻塞 Chain 和后续 Node。

## 9. 与现有端口的关系

### ExecutionSink

`ExecutionSink` 继续负责执行阶段和终态事实。处理链不复用该端口，因为两者的失败语义不同。

### OperationObserver

`OperationObserver` 继续记录 Node 内部操作。Completion Chain 只在整个叶子 Node 完成后触发。

### Driver

现有 `Driver` 保持不变。Completion Handler 通过独立的 `ReadOnlyBrowser` 读取页面，不能获得改变页面状态的能力。

## 10. Core 集成位置

处理链必须接入统一的叶子 Node 完成边界，不能放在组合节点的 children 循环中。

需要覆盖：

- `StepNode` 的成功、失败、取消和可选跳过出口；
- `WaitNode` 的成功、失败和取消出口；
- 独立 Validation Node 的成功、失败和取消出口；
- 后续新增并被定义为叶子的 Node。

组合 Node 只等待 child `Run` 返回。叶子 `Run` 在 Chain 完成前不返回，因此现有顺序编排自然保证下一个 Node 不会提前开始。

## 11. 核心业务测试案例

以下案例用于验收 Core 业务行为。每个案例必须明确前置条件、是否允许执行以及可观察反馈。

### 11.1 情景 A：叶子 Node 完成后允许执行处理链

**Given** `StepNode` 完成一次 occurrence，且已配置 `NodeCompletionChain` 与 `ReadOnlyBrowser`。
**When** Node 原始结果已经确定。
**Then** Core 允许执行处理链；所有 Handler 按注册顺序运行一次；处理链结束前下一个 Node 不得开始。

### 11.2 情景 B：组合 Node 完成后禁止执行处理链

**Given** 完成的是 Workflow、WorkflowCall 或 Repeat 组合 Node。
**When** 组合 Node 返回。
**Then** Core 不执行处理链；只有其内部实际运行的叶子 occurrence 可以触发处理链。

### 11.3 情景 C：处理链缺少只读能力时禁止开始 Program

**Given** `CompletionChain` 非空，但 `ReadOnlyBrowser` 为空。
**When** 调用 `RunProgramWithResult`。
**Then** Core 在 root 执行前返回配置错误；root 和任何 Handler 均不得执行。

### 11.4 情景 D：未配置处理链时允许正常执行

**Given** `CompletionChain` 为空。
**When** Program 包含一个或多个叶子 Node。
**Then** Core 正常执行叶子 Node；不调用任何 Handler；节点结果与未启用处理链时一致。

### 11.5 情景 E：多个 Handler 允许按注册顺序执行

**Given** Chain 依次注册 Handler A、B、C。
**When** 一个叶子 occurrence 完成。
**Then** 调用顺序严格为 A、B、C；B 不得在 A 返回前开始，C 不得在 B 返回前开始。

### 11.6 情景 F：Handler 并行执行被禁止

**Given** Handler A 尚未返回。
**When** Chain 准备执行 Handler B。
**Then** B 必须等待 A 返回；Core 不允许两个 Handler 并行处理同一叶子完成事件。

### 11.7 情景 G：Handler 失败后允许后续 Handler 继续

**Given** Handler A 返回错误，后续存在 Handler B。
**When** Chain 执行本次叶子完成处理。
**Then** Core 记录 A 的失败并继续执行 B；A 的错误不得短路整条 Chain。

### 11.8 情景 H：Handler 失败禁止改写成功节点结果

**Given** 叶子 Node 原始结果为成功，Handler 返回错误。
**When** Chain 完成。
**Then** Node 结果仍为成功；若存在下一个 Node，则允许继续执行；Handler 错误仅通过处理结果观测。

### 11.9 情景 I：Handler 成功禁止改写失败节点结果

**Given** 叶子 Node 原始结果为失败，所有 Handler 均成功。
**When** Chain 完成。
**Then** Node 结果仍为失败；Core 按原始失败语义终止或继续，不允许 Handler 将其改写为成功。

### 11.10 情景 J：节点取消后允许有界执行处理链

**Given** 叶子 Node 原始结果为取消，且 Chain 已配置。
**When** 原 Node context 已取消。
**Then** Core 使用脱离原取消信号的有界 context 执行 Chain；完成快照为 `CANCELED`；Chain 结束后仍返回原取消结果。

### 11.11 情景 K：可选步骤缺少目标时允许执行处理链

**Given** Optional Step 因目标不存在而跳过。
**When** 叶子执行结束。
**Then** Core 执行一次 Chain；完成快照为 `SKIPPED`；执行 phase 和时间线终态保持成功语义。

### 11.12 情景 L：Retry 内部尝试禁止重复触发处理链

**Given** 同一 Step occurrence 的第一次尝试失败，重试后成功。
**When** Step 最终结束。
**Then** Core 只执行一次 Chain；内部每次 Retry attempt 均不得单独触发 Chain。

### 11.13 情景 M：Repeat 每轮叶子执行允许分别触发处理链

**Given** Repeat 将同一叶子 Node 执行三轮。
**When** 三个 occurrence 依次完成。
**Then** Core 分别执行三次 Chain；快照 occurrence 依次为 1、2、3；Repeat 容器自身不得额外触发。

### 11.14 情景 N：Handler 响应超时取消后允许后续 Handler 继续

**Given** Handler A 超过单 Handler 超时，后续存在 Handler B，且 Chain 总超时尚未到达。
**When** Core 取消 A 的 context，且 A 响应取消并返回。
**Then** Core 将 A 记录为失败并继续执行 B；在 A 返回前不得启动 B。

### 11.15 情景 O：Handler 不响应超时取消时继续阻塞

**Given** Handler A 超过单 Handler超时且不响应 context 取消。
**When** A 的 `Handle` 始终没有返回。
**Then** Core 继续同步等待 A；后续 Handler 和下一个 Node 均不得开始；Core 不承诺强制终止 A。

### 11.16 情景 P：Chain 总超时后禁止启动剩余 Handler

**Given** 当前 Handler 已返回，Chain 总超时已经到达，仍有 Handler 尚未开始。
**When** Core 准备启动下一个 Handler。
**Then** Core 不再启动剩余 Handler；Node 原始结果保持不变。

### 11.17 情景 Q：Handler 允许读取明确开放的状态

**Given** Handler 获得有效 `NodeCompletionContext`。
**When** Handler 调用 `CaptureScreenshot`、`SnapshotDOM` 或 `ObserveElement`。
**Then** Core 允许读取，并返回与 Core 内部可变存储隔离的值对象。

### 11.18 情景 R：Handler 禁止改变 Runtime 状态

**Given** Handler 希望点击、输入、导航、执行脚本或取得实时 Element/Driver 句柄。
**When** Handler 检查 `NodeCompletionContext` 与 `ReadOnlyBrowser`。
**Then** Core 不提供这些能力；Handler 不能通过完成处理链改变 Runtime 或页面状态。

### 11.19 情景 S：修改返回值禁止影响 Core 内部状态

**Given** Handler 已取得截图字节、元素属性或 DOM 快照复合数据。
**When** Handler 修改这些返回值。
**Then** Core 内部状态保持不变；后续 Handler 不得观察到由共享底层存储导致的修改。

### 11.20 情景 T：空 Chain 允许作为无操作执行

**Given** `NodeCompletionChain` 已创建但没有注册 Handler。
**When** 叶子 occurrence 完成。
**Then** Chain 立即返回，不产生 Handler 结果，也不改变 Node 结果。

### 11.21 情景 U：运行中禁止修改 Handler 列表

**Given** 一次 Program 已开始，Chain 的 Handler 列表已固定。
**When** 调用方尝试在运行期间增加、删除或重排 Handler。
**Then** 本次运行不得采用变更后的列表；每个叶子均按运行开始时固定的注册顺序执行。

## 12. 实施步骤

### 阶段一：固定领域契约

- 定义 `NodeOutcome`、`NodeExecutionRef` 和 `NodeExecutionSnapshot`。
- 定义受限的 `ReadOnlyBrowser` 及截图、DOM 快照、元素观察值对象。
- 定义 `NodeCompletionHandler`、`NodeCompletionChain` 和 Handler 结果。
- 明确 Handler 列表不可在执行期间修改。

### 阶段二：建立统一叶子完成路径

- 增加叶子 Node 执行开始时间和 occurrence 捕获。
- 在 Node 原始结果确定后构造快照。
- 在叶子 `Run` 返回前同步执行 Chain。
- 保证成功、失败、取消和跳过出口恰好触发一次。
- 保证组合 Node 不触发 Chain。

### 阶段三：注入只读浏览器能力

- 在单次运行配置中注入 `ReadOnlyBrowser` 和 Completion Chain。
- 在运行前校验 Chain 与只读浏览器能力配置。
- 复用有效 selector overlay 实现元素观察，返回深复制的值对象。
- 保证全部只读调用与正常 Driver 操作严格串行。
- 不修改现有 `Driver`、`Element`、`Locator` 和 `Reader` 接口。

### 阶段四：错误观测与时间边界

- 收集每个 Handler 的开始时间、结束时间和错误。
- Handler 失败后继续执行剩余 Handler。
- 增加单 Handler 与整条 Chain 的有界 context。
- 保证所有 Handler 错误不传播到 Node 返回值。

### 阶段五：契约测试

- 枚举全部叶子和组合 Node，固定触发分类。
- 验证 Handler 注册顺序与严格串行行为。
- 验证 Handler 失败不会短路 Chain。
- 验证 Handler 失败不会改变 Node 结果。
- 验证 Retry 不重复触发、Repeat 分 occurrence 触发。
- 验证 Handler 响应 context 取消后，Chain 才继续执行后续 Handler。
- 验证不响应 context 的 Handler 会保持同步阻塞，Core 不承诺强制终止。
- 验证 Handler 只能获得明确列出的只读浏览器能力。
- 验证元素观察返回值不能修改 Core 内部状态。
- 验证只读能力与正常 Driver 操作不并发。

## 13. 验收标准

- 每次叶子 Node occurrence 完成后恰好执行一次 Chain。
- 组合 Node 不执行 Chain。
- 下一个 Node 必须等待全部 Handler 结束后才能开始。
- Handler 严格按注册顺序串行执行。
- 任意 Handler 失败都不会阻止后续 Handler。
- 任意 Handler 失败都不会改变 Node 原始结果或主链决策。
- 快照准确表达成功、失败、取消和跳过结果。
- Handler 无法修改 Core 状态或执行浏览器写操作。
- `ReadOnlyBrowser` 能够截图、读取 DOM 快照和观察元素，但不能改变页面状态。
- Handler 超时后，Core 取消其 context 并同步等待其返回；只有正确响应取消的 Handler 才能保证 Chain 及时继续。
- Chain 总超时只阻止尚未开始的 Handler，不强制终止当前正在同步执行的 Handler。
- `go test -race ./...` 通过。
- 新增逻辑测试覆盖率不低于 80%。

## 14. 已固定的业务决策

1. 单 Handler 默认超时和整条 Chain 默认超时由 `NodeCompletionChain` 配置固定。
2. 可选 Step 缺少目标时，执行 phase 保持成功，完成快照为 `SKIPPED`。
3. `ScreenshotArtifact.Data` 作为 Core 值对象返回，调用方负责在 Handler 的有界 context 内消费。
4. `StepNode`、`WaitNode`、`ValidationNode` 和 `ValidationGroupNode` 是叶子 Node。
