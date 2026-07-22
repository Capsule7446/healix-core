# 录制相对时间轴与叶子步骤边界实施计划

> 状态：**已实现**。核心契约位于 `domain/node/lifecycle.go`，Engine 编排位于 `application/engine/coordinator.go`。

## 1. 背景

当前 `node.Recorder` 只管理一次 `node.Program` 的开始与结束；`node.Event` 表达节点阶段，但没有录制相对时间，也没有形成只针对叶子步骤的稳定边界契约。

Core 需要能够回答：

- 一次录制开始后经过了多久；
- 哪一个叶子步骤在何时开始和结束；
- 该步骤是哪一次实际执行；
- 该步骤最终成功、失败还是取消；
- 时间线记录失败时，本次执行应如何反馈。

## 2. 目标

- 以录制成功启动时刻为零点表达经过时间，不使用日期或墙上时钟时间。
- 为时间线事件提供稳定顺序，解决相同时间精度下的排序问题。
- 只为可独立执行的叶子步骤产生开始和结束边界。
- 使用运行时身份区分重复引用和重复执行。
- 明确定义 Retry、Repeat、失败、取消以及记录端口失败的业务语义。
- 保持现有执行事实、操作观测和录制生命周期职责清晰。
- 通过核心业务测试固定所有不变量和可观察反馈。

## 3. 边界

- 时间线只表达 Core 内的录制生命周期、叶子步骤边界和执行结果。
- 组合节点不产生叶子步骤边界。
- Runtime 保持单顺序执行模型。
- 本次契约不定义并行步骤的活动范围。
- 相对时间不加入现有 `node.Event`，避免改变执行事实的既有语义。
- 如需消费步骤时间线，可实现 `StepTimelineSink`；具体消费方式不属于本契约。

## 4. 领域模型

### 4.1 时间标记

```go
type TimelineMark struct {
    Offset   time.Duration
    Sequence uint64
}
```

业务含义：

- `Offset` 是从录制零点开始经过的时长。
- `Sequence` 是一次录制内严格递增的事件序号。
- 时间线事件按 `(Offset, Sequence)` 排序。

不变量：

- `Offset >= 0`。
- 后生成的 Mark，其 Offset 不得小于此前 Mark。
- `Sequence` 必须从正数开始并严格递增。
- 同一次录制中的 Mark 必须来自同一个 Timeline。

### 4.2 时间轴端口

```go
type RecordingTimeline interface {
    Mark() TimelineMark
}
```

端口约束：

- `Mark` 必须支持安全的并发调用。
- 实现必须基于单调经过时间，不能因系统时间调整而倒退。
- Core 消费该端口，不持有具体计时实现。

### 4.3 叶子步骤执行身份

```go
type StepExecutionRef struct {
    RunID      string
    NodeID     string
    Occurrence int
}
```

不变量：

- `RunID` 必须非空。
- `NodeID` 必须是编译后不碰撞的运行时节点身份。
- `Occurrence >= 1`。
- 同一 Run 内 `(NodeID, Occurrence)` 唯一标识一次叶子步骤执行。

### 4.4 步骤边界与结果

```go
type StepBoundary string

const (
    StepBoundaryStarted  StepBoundary = "STARTED"
    StepBoundaryFinished StepBoundary = "FINISHED"
)

type StepOutcome string

const (
    StepOutcomeSucceeded StepOutcome = "SUCCEEDED"
    StepOutcomeFailed    StepOutcome = "FAILED"
    StepOutcomeCanceled  StepOutcome = "CANCELED"
)
```

### 4.5 步骤时间线事件

```go
type StepTimelineEvent struct {
    Step     StepExecutionRef
    Boundary StepBoundary
    Outcome  StepOutcome
    Mark     TimelineMark
}
```

不变量：

- `STARTED` 的 Outcome 必须为空。
- `FINISHED` 的 Outcome 必须是 SUCCEEDED、FAILED 或 CANCELED。
- 同一次 StepExecution 最多产生一个成功记录的 STARTED 和一个成功记录的 FINISHED。
- FINISHED 必须对应此前成功记录的 STARTED。
- FINISHED 的 Mark 不得早于 STARTED。

### 4.6 时间线写入端口

```go
type StepTimelineSink interface {
    RecordStepTimelineEvent(context.Context, StepTimelineEvent) error
}
```

该端口只接收叶子步骤时间线事件，不替代：

- `ExecutionSink` 的执行阶段与终态事实；
- `OperationObserver` 的操作尝试、耗时和错误分类。

## 5. 叶子步骤规则

时间线边界由节点运行路径产生，不能由 Sink 根据 NodeID 推断。

叶子步骤包括：

- 执行实际动作的 StepNode；
- 作为独立工作流步骤执行的等待节点；
- 作为独立工作流步骤执行的验证节点或验证组。

非叶子节点包括：

- WorkflowRef；
- Sequence；
- Repeat 容器；
- 其他只负责组合、委派或控制流程的节点。

WorkflowRef 自身不产生边界；其展开后的叶子步骤正常产生边界。验证组若在工作流中是一个独立步骤，则整个验证组是一项叶子步骤，组内条件不再分别产生时间线边界。

实现前必须用当前所有 `node.Node` 具体类型形成分类表，并以测试固定，不能遗漏节点类型。

## 6. Recorder 与 Runtime 编排

### 6.1 Recorder 契约

将现有 Recorder 调整为：

```go
type Recorder interface {
    Start(context.Context, string) (RecordingTimeline, error)
    Stop(context.Context, bool) error
}
```

`Start` 成功表示录制零点已经建立，并返回本次运行唯一的 Timeline。

### 6.2 Engine Config

Config 增加：

```go
StepTimeline StepTimelineSink
```

Timeline 不由调用方单独注入，而由 Recorder Start 返回，防止 Recorder 与 Runtime 使用不同零点。

### 6.3 执行时序

1. 校验 RunID、Driver、Program Root 及录制配置组合。
2. Recorder 非空时调用 `Start`。
3. `Start` 成功后取得 Timeline。
4. 创建 execution-local Runtime，注入 Timeline 和 StepTimelineSink。
5. 执行 Program Root。
6. 使用脱离原执行取消的有界 context 调用 `Stop`。
7. 合并 Program 执行错误和 Recorder Stop 错误。

### 6.4 配置组合

第一阶段固定以下规则：

| Recorder | StepTimelineSink | 结果 |
|---|---|---|
| nil | nil | 允许；按当前无录制模式执行 |
| 非 nil | nil | 允许；仅管理整次录制生命周期，不记录步骤时间线 |
| 非 nil | 非 nil | 允许；启用完整步骤时间线 |
| nil | 非 nil | 配置错误；没有录制零点，禁止执行 |

这样保留当前 Recorder 的兼容用途，同时拒绝无法定义 Offset 零点的半配置。

## 7. 叶子步骤执行语义

### 7.1 正常执行

```text
分配 occurrence
生成 STARTED Mark
记录 STARTED
执行叶子步骤
生成 FINISHED Mark
记录 FINISHED/SUCCEEDED
返回成功
```

### 7.2 StepTimeline STARTED 写入失败

- 叶子步骤不得开始执行。
- 返回带上下文的时间线开始记录错误。
- 不产生 FINISHED。
- 现有 ExecutionSink 已记录的阶段事实按其自身契约处理，不伪造步骤成功。

### 7.3 叶子步骤执行失败

- 使用有界 cleanup context 记录 FINISHED/FAILED。
- 若 FINISHED 写入成功，返回原始步骤错误。
- 若 FINISHED 写入失败，使用 `errors.Join` 同时返回步骤错误和时间线错误。

### 7.4 执行取消

- 若 STARTED 已成功记录，尽力记录 FINISHED/CANCELED。
- FINISHED 使用 `context.WithoutCancel` 加固定超时。
- 若 CANCELED 写入失败，合并取消错误和时间线错误。

### 7.5 FINISHED 写入失败

- 不改变叶子步骤实际执行结果。
- 不将成功执行改写为业务步骤失败；返回独立可识别的时间线终态记录错误。
- 调用方可同时观察“步骤执行结果”和“时间线记录结果”。

为避免单个 `error` 无法表达成功执行加记录失败，应用层应返回结构化结果。

## 8. Core 可观察反馈

### 8.1 运行结果

推荐将 `RunProgram` 从只返回 `error` 演进为：

```go
type RunResult struct {
    ExecutionOutcome ExecutionOutcome
    RecordingOutcome RecordingOutcome
    TimelineOutcome  TimelineOutcome
}
```

最小枚举：

```go
type ExecutionOutcome string

const (
    ExecutionSucceeded ExecutionOutcome = "SUCCEEDED"
    ExecutionFailed    ExecutionOutcome = "FAILED"
    ExecutionCanceled  ExecutionOutcome = "CANCELED"
    ExecutionNotStarted ExecutionOutcome = "NOT_STARTED"
)

type RecordingOutcome string

const (
    RecordingDisabled   RecordingOutcome = "DISABLED"
    RecordingSucceeded  RecordingOutcome = "SUCCEEDED"
    RecordingStartFailed RecordingOutcome = "START_FAILED"
    RecordingStopFailed RecordingOutcome = "STOP_FAILED"
)

type TimelineOutcome string

const (
    TimelineDisabled   TimelineOutcome = "DISABLED"
    TimelineComplete   TimelineOutcome = "COMPLETE"
    TimelineStartFailed TimelineOutcome = "START_FAILED"
    TimelineFinishFailed TimelineOutcome = "FINISH_FAILED"
)
```

如直接修改 `RunProgram` 返回值影响过大，可在第一阶段新增 `RunProgramWithResult`，确认迁移完毕后删除旧入口；不长期维护两套行为不同的执行路径。

### 8.2 错误分类

Core 错误必须支持 `errors.Is` 或 `errors.As` 分类，至少区分：

- Recorder 启动失败；
- Recorder 停止失败；
- Timeline 配置错误；
- STARTED 写入失败；
- FINISHED 写入失败；
- 叶子步骤执行失败；
- 执行取消。

错误必须携带 RunID、NodeID、Occurrence 和 Boundary 上下文，但不得把结构化结果退化为依赖错误字符串解析。

### 8.3 事件反馈

成功记录的 `StepTimelineEvent` 是 Core 对步骤录制边界的事实反馈。事件消费者不应从 `node.Event`、OperationObservation 或错误字符串反推叶子步骤边界。

## 9. Retry 与 Repeat

### Retry

Retry 是同一次叶子步骤执行内部的操作尝试：

```text
Step A occurrence 1 STARTED
  attempt 1 failed
  attempt 2 succeeded
Step A occurrence 1 FINISHED/SUCCEEDED
```

不增加 occurrence，不重复产生 STARTED。尝试详情继续由 OperationObserver 表达。

### Repeat

Repeat 每一轮都会重新运行叶子步骤并创建新 occurrence：

```text
Step A occurrence 1 STARTED/FINISHED
Step A occurrence 2 STARTED/FINISHED
Step A occurrence 3 STARTED/FINISHED
```

Repeat 容器自身不产生时间线边界。

## 10. 核心业务测试案例

以下案例用于验收 Core 业务行为。每个案例都必须明确：前置情景、是否允许 Program 或叶子行为执行、产生哪些事件，以及返回什么结构化结果。

### 执行许可矩阵

| 案例 | 情景 | 执行判定 | Core 反馈 |
|---|---|---|---|
| 10.1 | 普通顺序执行 | 允许 Program 和全部叶子执行 | 每个叶子依次产生 STARTED、FINISHED/SUCCEEDED |
| 10.2 | WorkflowRef 展开 | 允许展开后的叶子执行；引用节点禁止产生边界 | 仅叶子产生完整边界 |
| 10.3 | 嵌套 WorkflowRef | 允许最终叶子执行；各层引用节点禁止产生边界 | 仅最终叶子产生一对边界 |
| 10.4 | 同一 Workflow 多次引用 | 允许各调用位置的叶子执行 | runtime NodeID 不碰撞，occurrence 独立计数 |
| 10.5 | Repeat | 允许每轮叶子执行；Repeat 容器禁止产生边界 | 每轮产生新的 occurrence 和一对边界 |
| 10.6 | Retry 后成功 | 允许内部重试；Retry attempt 禁止重复产生边界 | 单 occurrence、单对边界、最终 SUCCEEDED |
| 10.7 | Retry 耗尽 | 允许重试至策略上限；耗尽后禁止继续尝试 | 单对边界、最终 FAILED |
| 10.8 | 叶子步骤失败 | STARTED 成功后允许叶子执行 | 写入 FINISHED/FAILED，保留原始错误 |
| 10.9 | 执行取消 | 取消前已 STARTED 的叶子不再继续业务行为 | 尽力写入 FINISHED/CANCELED |
| 10.10 | Recorder 启动失败 | 禁止 root 和叶子执行 | NOT_STARTED、START_FAILED，无步骤事件 |
| 10.11 | Recorder 停止失败 | Program 已执行结果保持有效 | ExecutionOutcome 不变，RecordingOutcome 为 STOP_FAILED |
| 10.12 | STARTED 写入失败 | 禁止该叶子业务行为执行 | START_FAILED，不产生 FINISHED |
| 10.13 | 成功步骤的 FINISHED 写入失败 | 叶子已经执行成功，不回滚也不改写 | ExecutionOutcome 为 SUCCEEDED，TimelineOutcome 为 FINISH_FAILED |
| 10.14 | 失败步骤的 FINISHED 写入失败 | 叶子已经执行失败，不重复执行 | 同时保留执行失败和时间线失败 |
| 10.15 | 取消后的 FINISHED 写入失败 | 禁止恢复已取消的叶子执行 | CANCELED 与 FINISH_FAILED 可分别识别 |
| 10.16 | 无录制模式 | 允许 Program 执行 | RecordingOutcome、TimelineOutcome 均为 DISABLED |
| 10.17 | 仅 Recorder 模式 | 允许 Program 执行 | Recorder 启停，不产生步骤时间线 |
| 10.18 | 非法半配置 | 禁止 Program 执行 | Timeline 配置错误，root 不执行 |
| 10.19 | 相同 Offset | 允许生成多个 Mark | 使用 Sequence 保持稳定顺序 |
| 10.20 | 非法事件 | 禁止向 Sink 写入 | 领域校验先失败 |
| 10.21 | 取消后的 Recorder Stop | 禁止继续 root；允许有界清理 | Stop 使用 detached context 调用一次 |
| 10.22 | 每种具体 Node 分类 | 仅叶子允许产生边界 | 分类表对所有 Node 类型作明确断言 |

### 10.1 普通顺序执行

**Given** Program 包含叶子步骤 A、B，Recorder 与 StepTimelineSink 正常。
**When** Program 执行成功。
**Then** 事件严格为 A STARTED、A FINISHED/SUCCEEDED、B STARTED、B FINISHED/SUCCEEDED；Mark 不倒退，Sequence 严格递增；RunResult 三项均成功。

### 10.2 WorkflowRef 展开

**Given** Program 含 WorkflowRef，引用内部包含叶子步骤 A、B。
**When** 执行引用。
**Then** WorkflowRef 没有时间线事件；A、B 各有完整边界。

### 10.3 嵌套 WorkflowRef

**Given** WorkflowRef 多层嵌套，最终包含一个叶子步骤。
**When** 执行 Program。
**Then** 只有最终叶子步骤产生一对边界，所有引用容器均不产生边界。

### 10.4 同一 Workflow 多次引用

**Given** 同一 Workflow 在两个调用位置被引用。
**When** 两个调用依次执行。
**Then** 展开的叶子步骤具有不碰撞的 runtime NodeID；各自 occurrence 从其运行时身份独立计数。

### 10.5 Repeat

**Given** Repeat 执行同一叶子步骤三次。
**When** 三轮均成功。
**Then** 产生 occurrence 1、2、3 的三对边界；Repeat 容器没有边界。

### 10.6 Retry 后成功

**Given** 叶子步骤第一次操作失败、第二次成功。
**When** RetryPolicy 允许重试。
**Then** 只产生一对步骤边界和一个 occurrence；最终 Outcome 为 SUCCEEDED；OperationObserver 记录两次 attempt。

### 10.7 Retry 耗尽

**Given** 所有操作尝试均失败。
**When** RetryPolicy 耗尽。
**Then** 只产生一对步骤边界；最终 Outcome 为 FAILED；RunResult.ExecutionOutcome 为 FAILED。

### 10.8 叶子步骤失败

**Given** STARTED 写入成功，叶子步骤返回业务错误。
**When** Runtime 完成失败清理。
**Then** 写出 FINISHED/FAILED，返回原始业务错误，TimelineOutcome 为 COMPLETE。

### 10.9 执行取消

**Given** 叶子步骤已经 STARTED。
**When** context 被取消。
**Then** 使用 cleanup context 写出 FINISHED/CANCELED；ExecutionOutcome 为 CANCELED。

### 10.10 Recorder 启动失败

**Given** Recorder.Start 返回错误。
**When** RunProgram 被调用。
**Then** Root 不执行、没有步骤事件、ExecutionOutcome 为 NOT_STARTED、RecordingOutcome 为 START_FAILED。

### 10.11 Recorder 停止失败

**Given** Program 成功，Recorder.Stop 返回错误。
**When** RunProgram 完成清理。
**Then** ExecutionOutcome 仍为 SUCCEEDED，RecordingOutcome 为 STOP_FAILED，错误可分类为 Recorder Stop failure。

### 10.12 STARTED 写入失败

**Given** StepTimelineSink 拒绝 STARTED。
**When** 叶子步骤准备执行。
**Then** 叶子行为不执行、不写 FINISHED、TimelineOutcome 为 START_FAILED，执行返回可分类错误。

### 10.13 成功步骤的 FINISHED 写入失败

**Given** 叶子行为成功，但 Sink 拒绝 FINISHED。
**When** 叶子步骤退出。
**Then** ExecutionOutcome 保持 SUCCEEDED，TimelineOutcome 为 FINISH_FAILED，并返回可分类的时间线记录错误。

### 10.14 失败步骤的 FINISHED 写入失败

**Given** 叶子行为失败，Sink 同时拒绝 FINISHED。
**When** Runtime 完成清理。
**Then** ExecutionOutcome 为 FAILED、TimelineOutcome 为 FINISH_FAILED；返回错误同时保留两种原因。

### 10.15 取消后的 FINISHED 写入失败

**Given** 已 STARTED 的叶子步骤被取消，cleanup 写入也失败。
**When** Runtime 返回。
**Then** ExecutionOutcome 为 CANCELED、TimelineOutcome 为 FINISH_FAILED；取消和写入错误均可通过错误链识别。

### 10.16 无录制模式

**Given** Recorder 和 StepTimelineSink 均为空。
**When** Program 执行。
**Then** 行为与当前版本一致；RecordingOutcome 和 TimelineOutcome 均为 DISABLED。

### 10.17 仅 Recorder 模式

**Given** Recorder 非空、StepTimelineSink 为空。
**When** Program 执行。
**Then** Recorder 正常启停，不产生步骤时间线事件；TimelineOutcome 为 DISABLED。

### 10.18 非法半配置

**Given** Recorder 为空、StepTimelineSink 非空。
**When** RunProgram 校验配置。
**Then** Program 不执行，返回 Timeline 配置错误。

### 10.19 相同 Offset 的稳定排序

**Given** Timeline 为多个事件返回相同 Offset。
**When** 顺序生成多个 Mark。
**Then** Sequence 严格递增，按 `(Offset, Sequence)` 排序可还原生成顺序。

### 10.20 Sink 收到非法事件

**Given** STARTED 携带 Outcome、FINISHED 未携带 Outcome 或字段为空。
**When** 事件进入领域校验。
**Then** 在调用端口前失败，不向 Sink 写入非法事实。

### 10.21 Recorder Stop 在取消后执行

**Given** 原执行 context 已取消。
**When** Engine 清理 Recorder。
**Then** Stop 使用脱离取消且有超时的 context，仍被调用一次。

### 10.22 每种具体 Node 的叶子分类

**Given** 当前代码中的每一种具体 Node 类型。
**When** 分别执行。
**Then** 测试表明确断言它是否产生步骤边界；新增 Node 类型时必须更新该测试表。

## 11. 兼容迁移

`Recorder.Start` 签名和 `RunProgram` 结果会影响现有调用代码。仓库内部按以下顺序迁移：

1. 先新增领域值对象、校验和测试替身。
2. 修改 Recorder Start 返回 Timeline，并更新全部实现和测试。
3. 增加 StepTimelineSink 与 Runtime 字段。
4. 增加结构化 RunResult；短期保留旧入口时，只允许旧入口委托新入口，不复制执行逻辑。
5. 迁移全部调用方后删除临时兼容入口。

不增加长期 shim，不保留两套 Recorder 或两套 Runtime 行为。

## 12. 文件级实施顺序

### 阶段一：领域类型与校验测试

- `domain/node/runtime.go`
  - TimelineMark、RecordingTimeline；
  - StepExecutionRef；
  - StepBoundary、StepOutcome；
  - StepTimelineEvent、StepTimelineSink；
  - Runtime 注入字段。
- 新增或扩展 `domain/node/*_test.go`
  - 值对象与事件不变量；
  - 所有具体 Node 的叶子分类矩阵。

### 阶段二：叶子步骤边界测试与实现

- `domain/node/step.go`
  - 动作步骤边界。
- `domain/node/validation.go`
  - 独立等待与验证步骤边界。
- `domain/node/composite.go`
  - 验证组合节点不产生边界。
- 覆盖 Retry、Repeat、WorkflowRef、嵌套引用、失败和取消。

### 阶段三：Engine 编排与结果

- `application/engine/engine.go`
  - Config 增加 StepTimelineSink；
  - 定义 RunResult 与 Outcome；
  - 提供结构化执行入口。
- `application/engine/coordinator.go`
  - 迁移 Recorder Start；
  - 注入 Timeline；
  - 配置组合校验；
  - Stop cleanup 与错误合并。
- 更新 `application/engine/*_test.go` 的 Recorder、Timeline 和 Sink 测试替身。

### 阶段四：契约文档同步

- `docs/domains/node.md`
  - 领域模型、叶子规则和不变量。
- `docs/application/engine/run-program.md`
  - 应用时序、RunResult 和失败语义。
- `docs/integration/public-contract.md`
  - Recorder 与 RunProgram 的破坏性变更。
- 实现完成后将本文状态改为“已实现”，并链接源码与测试。

## 13. 验收标准

- 第 10 节全部业务案例有自动化测试。
- WorkflowRef 和所有组合容器不产生叶子边界。
- 所有叶子步骤均产生合法、成对、可排序的边界事件。
- Retry 不新增 occurrence；Repeat 每轮新增 occurrence。
- 成功、失败、取消和记录失败均能通过 RunResult 与错误分类准确表达。
- Recorder 启停与 Timeline 零点生命周期一致。
- 无录制模式保持现有执行行为。
- `go test -race ./...` 通过。
- `go test -cover ./...` 通过，新增逻辑覆盖率不低于 80%。
- `go vet ./...` 通过。
- Go reviewer 和通用 code reviewer 无 CRITICAL/HIGH 问题。

## 14. 已固定的业务决策

1. `WaitNode`、`ValidationNode` 和 `ValidationGroupNode` 属于叶子步骤。
2. 叶子行为成功但 FINISHED 写入失败时，ExecutionOutcome 保持 SUCCEEDED，TimelineOutcome 为 FINISH_FAILED。
3. `RunProgram` 保留 error-only 入口并委托 `RunProgramWithResult`，两者共享唯一执行路径。
