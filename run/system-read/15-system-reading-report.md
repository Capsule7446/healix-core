# 15 系统解读报告

## 业务定位

Healix Core 是一个浏览器工作流执行领域：它将版本化的工作区计划转换为确定性的浏览器操作，在选择器发生漂移时安全地修复目标，并输出让宿主系统能够解释、审查和重放执行结果的证据。

## 代码入口地图

- `domain/workspace.TestTaskRunPlan` 和 `WorkflowExecutionPlan` 是锁定后的规划输入。
- `application/engine.CompileExecution`（`application/engine/compiler.go:40`）校验这些输入，并将其转换为运行时 `node.Program`、按索引组织的 `fingerprint.NodeSpec` 以及身份元数据。
- `application/engine.RunProgram`（`application/engine/engine.go:31`）校验运行配置、创建变量快照、构造 `node.Runtime`、管理可选的 Recorder 生命周期，并调用 `Program.Root.Run`。
- `node.Program.Root.Run` 分派 `WorkflowNode`、`WorkflowCallNode`、`StepNode`、等待/重复节点和验证节点。
- 宿主浏览器适配器实现 `node.Driver`/`node.Element`；Core 不包含浏览器 SDK、HTTP Controller、任务调度器或具体持久化实现。
- 读侧消费者使用 workspace Reader 端口和 metrics Reader 契约；具体 API/Wails/UI 适配器位于本仓库之外。

## 领域模型图谱

### 规划与工作区上下文

`NodeAggregate`、`WorkflowAggregate`、`TestTaskAggregate`、版本、依赖快照和运行计划是预期执行的事实来源。其不变量覆盖当前版本指针、版本顺序、工作流步骤结构、任务顺序和依赖有效性（`domain/workspace/assets.go`、`test_task.go`、`test_task_types.go`）。

### 执行上下文

`Program` 是编译后的可执行树。`Runtime` 是临时的运行协调状态，携带端口、变量、重试、节奏、观测数据和仅限本次运行的 selector overlay。`StepExecution` 保护阶段转换（`domain/node/runtime.go:43`、`:85`）。Runtime 不是持久化聚合。

### 指纹上下文

`Selector`、`Fingerprint`、`NodeSpec` 和 `FrameworkStack` 定义与框架无关的浏览器目标身份（`domain/fingerprint/fingerprint.go`、`framework.go`）。宿主适配器提供经过清洗的 `PageObservation`，Core 检测器在不接触原始 DOM 或 SDK 对象的情况下完成分类。

### 自愈上下文

`Candidate`、`Decision`、确定性的评分/样本排序以及 `Assessment` 表达选择器恢复和安全治理（`domain/heal/heal.go`、`scorer.go`、`sample.go`、`assessment.go`）。自愈上下文不负责持久化，也不拥有浏览器操作。

### 证据/报告上下文

验证、操作、自愈、网络和运行记录，以及 `ExecutionFactCommitter`，保存用于重放/审查的事实（`domain/workspace/evidence.go`、`execution_facts.go`、`ports.go`）。只有通过宿主适配器，这些事实才会变成持久数据。

## 业务交互流程

1. 锁定后的工作区计划被编译为运行时 Program。
2. 应用层启动一次运行，并注入 Driver、可选 Healer、Recorder 和事实端口。
3. 节点按顺序执行；阶段转换和浏览器操作产生事实。
4. 定位失败时返回明确的 `ErrElementNotFound` 业务信号。可选操作可以在不执行动作的情况下成功；否则可以尝试自愈。
5. 自愈对页面进行快照、为候选项评分、校验决策不变量并评估安全性。
6. 如果允许，执行器重新定位候选项，记录决策，然后安装仅限本次运行的 selector overlay，而不修改已编译的规格。
7. 验证会轮询直到稳定成功或超时，并记录观测结果；错误信息中的敏感实际值会被遮蔽。
8. 终态步骤事实以原子方式提交；过程进度事实使用单独的 writer。Recorder 停止失败也可能并入最终运行错误。

排序不变量是：先决策再安装 overlay、重新定位成功后再安装 overlay、先完成审计再安装 overlay、每次重复执行都创建新的 StepExecution，以及绝不修改已编译的 Program。

## 业务视图图谱

仓库当前隐含了以下视图：

- 节点列表/详情：`NodeQueryResult` 组合节点聚合数据和派生的 `RefCount`。
- 工作流列表/详情：`WorkflowQueryResult` 组合工作流聚合数据和派生的最近运行状态/时间。
- 测试任务列表/详情：`TestTaskQueryResult` 为任务运行提供同类信息。
- 运行仪表盘：`Dashboard` 包含状态计数、最近运行、队列、当前运行和任务投影。
- 执行详情/时间线：`ExecutionDetail` 组合工作流执行、步骤、网络、自愈和验证记录。
- 自愈审查：`HealCandidateRecord`、`HealObservationDetail`、候选证据和确定性样本。
- 自愈质量报告：`metrics.Query` 将不可变的 `ObservationFact` 投影为分桶结果和派生比率。
- 框架诊断：经过清洗的观测数据、框架元数据和指纹摘要。

### 字段来源与同步

权威的工作流/任务/节点/版本字段来自工作区聚合。`RefCount`、最近运行状态/时间、队列顺序、仪表盘计数和质量比率属于读侧派生值。执行详情来自明确的证据记录，而不是活动中的 Runtime。终态事实使用 `ExecutionFactCommitter`，非终态进度使用 `ExecutionProgressWriter`；metrics 是纯投影。Reader 端口定义查询边界，宿主适配器负责数据库模式、物化视图、UI/API DTO、刷新频率和保留策略。

Core 中没有具体的 CQRS 读模型存储。缺少的契约包括投影新鲜度、稳定游标、事件/幂等键、投影版本和一致性策略。

## 写侧/读侧边界

写侧真源是工作区定义/计划，以及在权威转换点产生的执行/自愈事实。读模型必须投影这些事实，不能读取可变的 Runtime、Program 状态、Driver、原始 DOM 或浏览器框架 SDK 对象。`metrics` 展示了当前最清晰的读侧边界。

## 风险与坏味道

1. `Runtime` 的中心性过高，直接耦合执行、自愈和指纹词汇。
2. `workspace` 混合了资产生命周期、证据、审查记录、仪表盘结构和查询端口。
3. 查询结果嵌入完整的写侧聚合，存在 entity-backed API 风险。
4. `TestTaskRun` 混合生命周期事实与展示/进度字段。
5. `Environment` 和 `EnvironmentSnapshot` 包含凭据，不能直接作为 UI 响应复用。
6. Program 的不可变性主要依赖约定，对 map/slice 的复制纪律要求较高。
7. 事实 sink 没有统一的持久化身份/事务/outbox 契约；尽力而为的操作观测不等价于终态提交。
8. 仓库中没有具体 API/controller 和读侧刷新实现，因此端点行为必须结合宿主仓库确认。

## 最小建议

- 让 Core 专注于领域契约；在明确宿主持久化需求前，不要在 Core 中加入读模型存储。
- 使用专用的、面向 UI 安全的查询 DTO，避免嵌入聚合；默认排除凭据。
- 不要让派生字段（`RefCount`、最近运行、仪表盘计数、队列标签、比率）参与聚合校验。
- 在实现适配器时增加新鲜度/来源时间、稳定关联/序列、幂等和投影版本字段。
- 保留 `metrics` 作为读侧分离的参考模式。
- 将证据同步和读投影视为适配器职责；不要让 Runtime 成为查询源。

## G4 结论

本报告支持双向追踪：业务概念可以映射到具体文件和行级入口，代码入口也可以反向映射到规划、执行、自愈、证据和视图含义。报告区分了已观察到的实现和由宿主负责、当前未知的部分。
