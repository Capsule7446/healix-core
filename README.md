# healix-core

`healix-core` 是一个面向浏览器自动化产品的 Go 领域与执行内核。它把可发布的自动化资产、不可变执行计划、确定性节点运行、自愈决策和执行事实建模为与传输、数据库和浏览器实现无关的包，供 Desktop、CLI、CI Runner 或 Server 作为库嵌入。

当前公开 API 仍处于 **v0**：代码可用且受测试约束，但尚未承诺 v1 兼容性。

## 它是什么，不是什么

**它提供：**

- Environment、Folder 等受修订号控制的普通可变自动化资产，以及 Node、Workflow、TestTask 等具有不可变发布版本的自动化资产及其不变量；
- 从已发布快照构建并封存多顶层执行项 `execution.Plan`；
- 按顶层执行项顺序和失败策略进行纯调度决策与领取执行权协调；
- 把每个执行计划顶层执行项独立编译为 `node.Program`，并通过注入端口运行；
- 选择器、元素指纹、变量插值、采样会话、确定性自愈和不可变执行事实；
- Repository、Driver、Recorder、调度和事实提交等宿主端口。

**它不提供：**

- HTTP API、数据库模式、消息协议、UI 或查询投影；
- Rod、Playwright、Wails、SQLite、rrweb 或文件存储；
- 生产级数据库、队列、浏览器或事实存储适配器；
- 一个完整的“领取任务 → 编译 → 运行 → 持久化终态”工作器循环。

换言之，Core 负责规则、决策和编排契约；宿主负责 IO、事务、并发控制、授权、审计与部署。

## 架构一览

```mermaid
flowchart TB
  Host[宿主组合根<br/>Desktop / CLI / CI / Server]

  subgraph App[应用层]
    AA[自动化<br/>资产命令与发布]
    AS[调度<br/>执行计划与调度决策]
    AE[执行引擎<br/>执行计划编译与执行程序运行]
    AX[执行<br/>工作器栅栏校验与事实提交]
  end

  subgraph Domain[九个领域包]
    DA[automation]
    DS[sampling]
    DE[execution]
    DN[node]
    DH[heal]
    DV[evidence]
    DF[fingerprint]
    DI[interpolation]
    DP[parameter]
  end

  Adapters[(宿主适配器<br/>数据库 / 队列 / 浏览器 / 事实)]

  Host --> AA
  Host --> AS
  Host --> AE
  Host --> AX
  AA --> DA
  AA --> DS
  AS --> DA
  AS --> DE
  AE --> DE
  AE --> DN
  AE --> DH
  AX --> DV
  DA --> DF
  DS --> DF
  DN --> DF
  DN --> DI
  DH --> DF
  AA -. 仓储端口 .-> Adapters
  AS -. 领取 / 状态 / 决策端口 .-> Adapters
  AE -. Driver / Recorder / Facts .-> Adapters
  AX -. 进度 / 提交端口 .-> Adapters
```

依赖始终由宿主指向应用层，再指向领域层；Core 不反向依赖基础设施。

## 九个领域与四个应用模块

### 领域层

| 包 | 已实现能力 |
|---|---|
| `domain/automation` | 版本化资产、Revision、发布快照、引用锁定、生命周期和文件夹树规则 |
| `domain/sampling` | 交互式采样会话、capture 幂等、匹配和临时工作流 |
| `domain/execution` | 封存的执行实例快照、调用作用域、Environment `Properties` 快照、执行实例/顶层执行项状态、预算与转换不变量；`domain/heal` 的决策在执行时归属 Execution |
| `domain/node` | 临时 `Program`/`Runtime` 执行模型、动作/等待/校验、重试、生命周期和运行端口 |
| `domain/heal` | 无浏览器或 LLM 依赖的确定性重定位、评分、评估与候选证据 |
| `domain/evidence` | 进度事实、终态事件、修复/校验观察和原子提交值语义 |
| `domain/fingerprint` | `NodeSpec`、Selector、Fingerprint 和框架检测值对象 |
| `domain/interpolation` | `${name}` 表达式解析与展开 |
| `domain/parameter` | 封闭类型的 `Value`（TEXT/NUMBER/BOOLEAN/SINGLE_SELECT/MULTI_SELECT）与字面量/父级引用 `Binding` |

以上领域行为均已实现；涉及浏览器、存储或外部服务的部分仍只是端口边界。

### 应用层

| 模块 | 已实现 | 仅端口契约 / 宿主义务 |
|---|---|---|
| `application/automation` | 聚合命令服务、Revision 冲突、采样发布、修复审核 | Repository 事务、ID/时钟、授权、查询 API |
| `application/scheduling` | `CreateRunService`、执行实例快照构建、`DecideAdvance`、`Coordinator` | 在同一目录/事务视图中解析 `latest`、原子创建/领取执行权、租约/栅栏校验、完整队列 |
| `application/engine` | `CompileRunSnapshot`、`RunCompiledEntry`、`RunCompiledEntryWithResult`、步骤元数据映射 | 选择已调度 entry，注入并实现运行端口 |
| `application/execution` | `EntryExecutor`、`execution.WorkerFence` 限定的 entry 执行、修复治理、进度与事实提交接口 | 进度存储和原子终态提交 |

接口存在不代表生产适配器已存在。

## 端到端执行

```mermaid
sequenceDiagram
  participant H as 宿主 / 入站适配器
  participant S as 调度
  participant Q as 领取执行权 / 状态适配器
  participant E as 执行引擎
  participant B as 浏览器适配器
  participant F as 进度 / 事实适配器

  H->>S: `CreateRun`(ctx, command)
  S->>S: 在一致目录视图中解析 `latest` 与依赖
  S-->>H: 不可变 `execution.RunSnapshot`（含已封存的执行计划）
  H->>Q: 原子持久化执行实例、快照与顶层执行项状态
  H->>S: Coordinator.ProcessNext(ctx, workerID, occurredAt)
  S->>Q: ClaimNext(workerID, occurredAt) / LoadEntryStates
  Q-->>S: claim（含不可变 RunSnapshot 与 WorkerFence）及当前状态
  S->>Q: ApplyDecision（原子 fencing）
  S-->>H: claimed / error；可运行 executionID 由 claim / decision 适配器契约承接
  H->>E: CompileRunSnapshot(snapshot)
  E-->>H: CompiledRun / 独立 CompiledEntry
  H->>E: RunCompiledEntry(ctx, entry, Config)
  E->>B: locate / action / wait / validate
  B-->>E: observation 或分类错误
  E->>F: `node.ExecutionSink` 运行事实（若注入）
  E-->>H: Program 运行结果
  H->>F: RecordStepProgress / CommitStepTransition
  F-->>H: 适配器的栅栏校验提交结果
```

关键边界：执行实例创建是解析边界：所有 `latest` 指针必须从同一目录/事务视图解析为具体版本并冻结到不可变 `execution.RunSnapshot`。Core 为每个顶层执行项调用一次 `BrowserSessionFactory.Create`，并在推进到下一项前关闭该会话；宿主浏览器适配器负责确保每次创建具有全新的身份与存储隔离，Core 不验证其新鲜性。嵌套工作流调用复用该顶层执行项的会话。`Program` 与 `Runtime` 都是每次执行临时构造的执行模型，不是持久化资产。领取令牌、步骤修订号、提交身份与终态事实的原子性必须由适配器兑现。

## 安装

远端版本可用时：

```bash
go get github.com/Capsule7446/healix-core@<version>
```

在相邻目录进行本地联调时，可在宿主 `go.mod` 中使用：

```go
require github.com/Capsule7446/healix-core v0.0.0

replace github.com/Capsule7446/healix-core => ../healix-core
```

本模块当前声明 Go `1.26.4`，生产代码仅依赖 Go 标准库和本模块内部包。

## 使用示例

### 1. 封存传输无关的执行计划

`execution.Seal` 会验证 Draft、复制输入并按 `SequenceNumber` 固定入口顺序。真实运行还应提供 Plan 引用到的 workflow、node 与 reference snapshots。

```go
package main

import (
    "log"

    "github.com/Capsule7446/healix-core/domain/execution"
)

func main() {
    plan, err := execution.Seal(execution.Draft{
        RunID:         "run-42",
        FailurePolicy: execution.FailurePolicyStopOnFailure,
        Entries: []execution.WorkflowEntry{
            {
                ExecutionID:      "execution-1",
                TestTaskItemID:   "item-1",
                SequenceNumber:   1,
                WorkflowID:       "workflow-login",
                WorkflowVersionID: "workflow-login-v3",
            },
        },
        Workflows: []execution.WorkflowSnapshot{
            {
                ID:         "workflow-login",
                WorkflowID: "workflow-login",
                VersionID:  "workflow-login-v3",
                DisplayName:   "登录",
                VersionNumber: 3,
                Steps: []execution.Step{
                    {
                        ID:          "step-wait",
                        DisplayName: "短暂等待",
                        Kind:        execution.WaitStep,
                        WaitKind:    "sleep",
                        WaitMS:      100,
                    },
                },
            },
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    _ = plan // 可交给 scheduling.DecideAdvance；编译需要完整 execution.RunSnapshot
}
```

### 2. 从已发布 TestTask 创建 Run

应用集成通过 `CreateRunService` 在一致目录视图中解析发布物，并原子封存 Run、Snapshot 与 entries：

```go
command := scheduling.CreateRunCommand{
    CommandID:        commandID,
    RunID:            runID,
    TestTaskID:       testTaskID,
    TestTaskVersionID: testTaskVersionID,
    EnvironmentID:    environmentID,
    Entries:          entryValues,
    FailurePolicy:    execution.FailurePolicyStopOnFailure,
    CreatedAt:        createdAt,
    ScreenshotPolicy: screenshotPolicy,
    HealerPolicy:     healerPolicy,
}

service, err := scheduling.NewCreateRunService(store)
if err != nil {
    return fmt.Errorf("create run service: %w", err)
}
result, err := service.CreateRun(ctx, command)
if err != nil {
    return fmt.Errorf("create run: %w", err)
}
snapshot := result.Snapshot
```

`store` 实现 `scheduling.CreateRunStore`，负责在同一事务视图中解析 TestTask、Workflow、Node、Environment 和全部 `latest`，并原子保存创建结果。调度协调器由宿主注入 `ClaimSource`、`EntryStateReader` 与 `DecisionWriter` 构造：`coordinator := scheduling.NewCoordinator(claims, states, writer)`，再调用 `coordinator.ProcessNext(ctx, workerID, occurredAt)`。协调器不接收 Plan 参数；它从 claim 适配器取得不可变 `RunSnapshot`，通过 decision writer 原子应用包含所选 execution identity 的决策。宿主通过 claim / decision 适配器契约承接该身份并驱动对应 entry。宿主必须保留完整 `snapshot` 供编译使用，不能只保存或传递其中的 Plan。

创建执行实例会把 TestTask 顶层执行项参数、Workflow 默认值以及每条嵌套调用边上的 `parameter.Binding` 解析为按调用路径隔离的作用域。`parameter.Value` 保留 TEXT、NUMBER、BOOLEAN、SINGLE_SELECT 与 MULTI_SELECT 类型；绑定只能是类型化字面量或父级引用。所有 `latest` 在此时解析并冻结，运行时不得再次查询当前指针。

### 3. 编译并运行被调度的 entry

```go
compiled, err := engine.CompileRunSnapshot(snapshot)
if err != nil {
    return fmt.Errorf("compile run snapshot: %w", err)
}

entry, ok := compiled.Entry(executionID)
if !ok {
    return fmt.Errorf("compiled entry %q not found", executionID)
}

err = engine.RunCompiledEntry(ctx, entry, engine.Config{
    RunID:        runID,
    ClaimToken:   claimToken,
    Driver:       driver,   // node.Driver，由宿主实现
    Recorder:     recorder, // node.Recorder，由宿主实现
    Facts:        facts,    // node.ExecutionSink，由宿主实现
    Healer:       heal.NewDefaultHealer(), // nil 表示关闭自愈
    StepInterval: 100 * time.Millisecond,
})
```

宿主必须先通过调度决定 entry 可运行，再执行对应 `CompiledEntry`；`CompileRunSnapshot` 本身不会领取任务或写数据库。运行时参数不由 `Config` 提供，而是在编译时从不可变 `RunSnapshot` 的调用作用域与 Environment 数据生成。编译必须接收完整的不可变 `execution.RunSnapshot`，因为除 Plan 中冻结的 workflow/node/reference 图外，编译器还要读取各调用路径冻结的参数值与 `parameter.Binding` 解析结果，并把冻结的 `Environment.Properties` 以 `env.` 前缀注入根调用作用域。只传 `snapshot.Plan()` 会丢失这些执行语义。

## 当前生命周期约束

- Environment 是普通 Automation 资产；Run 只冻结其 `Properties` 副本。Core 没有 `CredentialReference`、`CredentialService`、`SecretProvider` 或 secret-store 模型。
- TestTask 版本只能由显式手工创作创建；Sampling 与 Heal 只发布各自产物，不会隐式发布 TestTask 新版本。
- `SamplingWorkspace` 是采样草稿/重写工作区，`Session` 是浏览器交互会话；二者不是别名，也不共享生命周期。
- `evidence.HealObservation` 只记录观察，没有晋升状态；晋升是栅栏校验提交的结果。
- `domain/heal` 提供确定性算法和值，但在运行生命周期中属于 Execution；`Program`/`Runtime` 同样是临时 Execution 模型。

## Adapter 必须保证什么

- **入站：** 校验外部 DTO，生成唯一 ID 和时间戳；创建执行实例时在同一目录/事务视图中解析完整依赖图、参数作用域、Environment 和所有 `latest`，再封存快照。
- **自动化持久化：** 使用 opaque Revision 做 CAS，并在同一事务保存聚合、版本与指针变化。
- **调度：** 原子 claim、不可伪造 token、worker fencing、事务化应用 decision，并返回与 Plan 完全一致的 entry 状态集合。
- **浏览器执行：** 实现 `BrowserSessionFactory` 与 `node.Driver`/`Element`/`Locator`，尊重 `context.Context`；Core 为每个顶层执行项调用一次 `BrowserSessionFactory.Create` 并在推进前关闭会话，嵌套 Workflow 复用该会话。适配器必须保证每次创建使用全新的身份与存储隔离；Core 不验证会话新鲜性。目标缺失必须返回或包装 `node.ErrElementNotFound`，只有该类别会触发确定性自愈。
- **记录与事实：** 对进度实施 fencing；按 revision、commit identity 和封印依赖目标原子提交终态与 facts。
- **错误：** 保留 `errors.Is` / `errors.As` 分类并补充上下文，不依赖完整错误字符串。

详见[适配器职责](docs/integration/adapter-responsibilities.md)。

## 当前不支持或明确延期

- 租约心跳与租约过期恢复；
- 活跃取消注册表；
- 完整队列与持久化工作器循环；
- 生产级适配器和读取投影；
- HTTP、数据库模式、消息格式及序列化兼容性承诺。

这些是当前代码边界，不是对宿主能力的暗示或路线图承诺。

## 本地验证

```bash
make test       # go test ./...
make race       # go test -race ./...
make vet        # go vet ./...
make build      # go build ./...
make coverage   # 生成 coverage.out 并检查 80% 门槛
make lint       # golangci-lint run ./...
make check      # fmt-check + vet + test + race + coverage + build + lint
```

也可直接运行：

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

## 继续阅读

从[文档导航](docs/README.md)选择适合角色的阅读路径。建议优先阅读：

- [系统总览](docs/architecture/system-overview.md)：领域、应用和 adapter 的全局边界；
- [上下文地图](docs/architecture/context-map.md)：Automation、Execution 与共享内核的协作；
- [依赖规则](docs/architecture/dependency-rules.md)：允许的依赖方向与原子写入要求；
- [端到端执行](docs/architecture/end-to-end-execution.md)：从 TestTask 发布到终态 Evidence；
- [公开契约](docs/integration/public-contract.md)：稳定入口、错误面和兼容边界；
- [适配器职责](docs/integration/adapter-responsibilities.md)：宿主必须兑现的事务与 IO 语义。

源码与测试是最终事实来源；文档中的“已实现”“仅端口契约”“适配器义务”和“不支持”均描述当前仓库事实。
