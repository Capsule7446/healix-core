# Healix → healix-core 差异整合：总体调整清单

> 原始评估：相邻 Healix 仓库 `C:\Users\Paul\workspace\healix\docs\refactor\healix-core-v0.3.0-replacement-assessment.md`。
> 案例映射：C01–C28 分别对应原始评估中同编号的表格行与详细章节。
> 核验基线：healix-core 当前实现（`12d1ba2`）；原始评估未在本审核包中固定修订号。
> 本文保留历史计划与案例依据，并以当前已实现模型记录验收结果；未决行为保持未决。

## 1. 总体判断

当前核心库已具备稳定资产/不可变版本、封存执行计划、精确版本编译、typed 参数、执行实例创建时的 `LATEST` 冻结、顶层执行项浏览器隔离、自愈治理及采样发布。替换后的边界为：

1. 环境是普通属性；核心库不提供凭据子系统。
2. `parameter.Value` / `parameter.Binding` 贯穿发布、执行实例快照和编译。
3. 执行程序/运行时仅存在于一次执行；执行实例/执行计划属于执行，自愈判断属于 `domain/heal`。
4. `HealObservation` 可进入原子提交意图，晋升只能由提交结果报告。
5. SamplingWorkspace 与 Session 分离；TestTaskVersion 只由人工用例保存，采样/自愈不发布测试任务。
6. 调度保持 application orchestration；显式中止原子提交 `ABORTED` 后发送取消信号，普通执行上下文取消保持 `CANCELED`。

## 2. 整合原则

### 必须进入核心库

- 领域状态、业务不变量和确定性决策。
- 不可变版本、精确依赖、参数及策略快照。
- 发布、取消、中止、重排、晋升、审核的应用用例语义。
- 原子事务必须满足的业务边界、幂等和冲突分类。
- 宿主需要实现的最小端口及适配器一致性测试。

### 必须留在宿主

- SQLite/SQL、事务实现、队列表、锁及工作器存活检测。
- active-run goroutine/process 注册表与 `context.CancelFunc`。
- Rod/CDP、DOM、截图、rrweb、文件和对象存储。
- Wails/UI DTO、Dashboard、查询投影和指标。

### 明确禁止

- 在核心库中增加第二套 Runner 或兼容旧宿主的平行执行入口。
- 把数据库实体、UI ViewModel 或读模型搬进领域层。
- 不在本次重构中预埋 CredentialReference、Provider、Vault/KMS 或兼容 facade。
- 让临时运行时或 `HealObservation` 直接发布 NodeVersion；晋升只能由原子提交结果产生。
- 以接口存在代替“功能已闭环”的判断。

## 3. P0 调整清单

### P0-A：不可变执行实例创建边界（C02、C03、C07）

- [x] 设计 `RunSnapshot`，绑定 `RunID`、`TestTaskVersionID` 和 schema version。
- [x] 快照纳入精确 Workflow/Node/Reference 图。
- [x] 将根和嵌套 `LATEST` 从“发布时解析”迁到“执行实例创建时解析”。
- [x] 快照环境修订号、BaseURL 和完整属性。
- [x] 快照 ScreenshotPolicy 与 HealerPolicy。
- [x] 定义 canonical digest；同一执行实例不得替换快照内容。
- [x] 新增 CreateRun 应用服务，原子创建执行实例、entries、快照和 queue membership。
- [x] 定义命令 identity、幂等重放及同 ID 异 payload 冲突。
- [x] 新模型直接替换旧执行实例创建契约；不保留无 RunSnapshot 的执行路径，既有不兼容执行实例数据不进入新执行链路。

### P0-B：类型化参数闭环（C04、C05、C06）

- [x] 固定 WorkflowVersion 参数 schema 字段语义：`Name`→`${}` 变量名、`DisplayName`→测试任务表单 title、`Description`→表单 desc、`Type`→输入控件和值类型。
- [x] 测试任务编辑/发布必须绑定所选精确 WorkflowVersion 的 parameter schema；仅保存 `Name`→typed value，显示元数据仍归 WorkflowVersion。
- [x] 用封闭 typed value 模型替换 `DefaultValue string`、测试任务参数中的 `map[string]any` 以及封存边界中的字符串退化。
- [x] 默认值具有显式 presence，可区分“未提供”与合法空字符串。
- [x] 强制 `Required=false` 必须有合法 typed default；`Required=true` 不允许默认值兜底，根工作流由 测试任务人工输入，嵌套工作流可由父级 binding 提供。
- [x] 支持 TEXT、NUMBER、BOOLEAN、SINGLE_SELECT、MULTI_SELECT。
- [x] 拒绝 unknown、duplicate、missing required 和类型不匹配参数。
- [x] 定义 NUMBER 精度/规范形式、BOOLEAN 接受形式、MULTI 顺序/去重规则。
- [x] 发布时验证 typed default 与 options；测试任务输入也必须通过相同约束。
- [x] 建立 invocation graph 和稳定 call-path/scope ID。
- [x] 在执行实例创建时解析父子 binding、类型兼容及 optional default。
- [x] 为每个 root execution 和 nested invocation 冻结 ParameterSnapshot。
- [x] 参数上下文与运行时 extraction scratchpad 分离。
- [x] Compiler 只消费已解析 scope，不再补默认值或隐式 `fmt.Sprint` 转换。
- [x] 明确参数与环境属性的命名空间、快照和错误语义。

### P0-C：自愈治理闭环（C10–C16、C23）

- [x] 保留“仅明确 NotFound 后触发自愈”的错误分类边界。
- [x] 明确分离 运行时安全 与 asset governance。
- [x] `APPLIED` 与 `BELOW_CAP` 都可在 safety ALLOW 后恢复当前执行实例。
- [x] `BELOW_CAP` 永远不能因 streak 自动发布。
- [x] 将 `HealStreakDecision.Promote bool` 改为 感知分档的处置结果。
- [x] 定义观察事实唯一键并按执行实例去重。
- [x] 实现执行证据 → candidate/streak 观察事实应用服务。
- [x] APPLIED 第三次成功：原子发布 NodeVersion、推进 current、关闭 streak。
- [x] BELOW_CAP 第三次成功：原子转 AwaitingApproval，不改变节点 current version。
- [x] 所有 candidate、streak 和 pending review 以 `NodeID + BaseNodeVersionID + CandidateHash` 绑定；禁止跨 NodeVersion 累计。
- [x] 节点 current version 一旦离开 `BaseNodeVersionID`，基于旧版本的 `OBSERVING/AWAITING_APPROVAL` candidate、未完成 streak 和 pending review 自动失效并退出活跃查询；新版本后续自愈必须建立新候选。
- [x] 旧版本自愈 observations、已晋升/已拒绝候选及审核记录作为审计历史保留，不做物理删除。
- [x] 采用所有治理操作前强制 current-base 校验；若 UI 要求立即移除旧待审项，再由 NodeVersion 推进事务或 outbox 驱动批量 `STALE` 投影。
- [x] Original 选择器恢复：明确事实定义并原子 reset/stale 同 node/base 下相关候选。
- [x] 候选 hash 变化、重复/乱序事件均有确定规则。
- [x] 将步骤终态、校验、HealObservation、reset、streak、晋升纳入同一事务意图。
- [x] 为 adapter 提供原子提交、幂等和并发阈值一致性测试套件。

### P0-D：队列与运行控制（C17–C23）

- [x] Core 对每个顶层执行项调用一次 `BrowserSessionFactory.Create`，并在该顶层执行项结束后关闭返回的会话；在关闭后再推进下一顶层执行项的 Core 顺序保持不变。
- [x] 每次创建全新且相互隔离的浏览器实例/会话上下文，并保证不同身份以及 cookies、LocalStorage、SessionStorage、IndexedDB、cache、页面和登录态隔离，均是宿主适配器义务；当前不透明接口本身不强制或验证这些属性。
- [x] 同一顶层执行项内根工作流与所有嵌套 WorkflowRef 复用该顶层执行项的浏览器会话，嵌套调用不再次调用 `BrowserSessionFactory.Create`。
- [x] Core 定义浏览器 create/close 失败、顶层执行项 success/failure/abort 下的确定清理与 failure-policy 行为；宿主适配器负责使关闭实际完成隔离清理。
- [x] 保留 `DecideAdvance` 纯函数和计划顺序唯一权威。
- [x] `DecisionWriter.ApplyDecision` 原子提交 skips、next/final status。
- [x] 为 Create/Cancel/Abort/Reorder 建独立应用服务，拆分宽泛 `RunCommands`。
- [x] 取消排队项使用 expected status/revision，与领取执行权原子竞争。
- [x] 中止活动项由 `AbortRunService` 要求宿主事务原子提交权威的 `execution.Aborted` 并失效工作器栅栏，提交成功后才发送取消信号；普通执行上下文取消保持 `CANCELED`。
- [x] 提交成功、信号失败保留权威提交结果并返回 `ErrRunSignalRetryable`；由 `application/scheduling/run_command_services_test.go` 与 `run_command_transaction_conformance_test.go` 验收。
- [x] 队列 reorder 带 queue scope、expected 修订号、命令 ID。
- [x] 完整 reorder 校验 exact permutation；partial move 使用独立命令。
- [x] 统一 stale 栅栏错误，覆盖调度、进度和终态写入。
- [x] 定义工作器所有权失效及 stale 栅栏语义；具体存活检测机制留宿主。

### P0-E：采样发布闭环（C24–C27）

- [x] 分离 `Session` 捕获生命周期与 `SamplingWorkspace` 可编辑草稿；保留 CaptureID 首次结果幂等语义。
- [x] 为草稿 insert/update/delete/move/reorder 建领域命令，稳定临时 ID。
- [x] 明确 `IdentityKey`、`NodeUUID`、`TemporaryNodeID` 的统一术语与映射。
- [x] 实现 Snapshot → SamplingPublication 的 application mapper。
- [x] 保留 CREATE/FORCE_CREATE/MERGE/REUSE mode 到审计结果，不在规范化后丢失意图。
- [x] MERGE 保留已有节点身份与元数据，以采样得到的 PageURL/Origin/Selectors/Fingerprint 发布完整的新 NodeVersion；不得与旧版本隐式合并选择器或 fingerprint。
- [x] MERGE 使用 expected current version/revision CAS，并将工作流引用精确重写到新建的 `(NodeID, NodeVersionID)`。
- [x] 实现临时引用递归重写（root、repeat children、validation branches）。
- [x] 校验 input temporary IDs 与 result mappings exact-set equality。
- [x] 发布 ID 同 payload 重放返回原 mapping；异 payload 返回 identity conflict。
- [x] 节点、版本、mapping、WorkflowVersion、outbox 同一原子事务。

### P0-F：环境属性注入（C28）

- [x] 将环境收敛为一组规范化属性，移除 Variables/Properties/CredentialReferences 的重复模型。
- [x] 不区分 property 的机敏程度，移除 credential-like key 保留字。
- [x] 移除核心库的 CredentialReference、CredentialService、Provider resolution、Vault/rotation/reveal 等专用契约。
- [x] 执行实例保存所选环境的 ID/revision/BaseURL/Properties，并按 C07 的快照语义执行。
- [x] 每个顶层执行项创建新浏览器后，将同一组环境属性注入变量上下文。
- [x] 统一使用 `${env.<name>}`，并与普通工作流参数命名空间隔离。
- [x] 缺失环境 property 返回明确变量错误，不静默使用空字符串。
- [x] 未来若需要凭据安全管理，作为独立需求重新设计，不在本次预埋抽象。

## 4. P1 调整清单

- [x] C09 补齐 ValidationGroup 的 winning BranchID、终态 reason 和 collection-valued expected/actual evidence，使结果可解释并可重建。
- [x] 定义自愈已拒绝候选的重现规则：同一 `NodeID + BaseNodeVersionID + CandidateHash` 在 base 仍为 current 时不重新建立候选或累计 streak；新 base 可重新观察。
- [x] 扩展 `contract/public_api_test.go`，验证自动化、调度、执行、采样的公开服务和端口可由外部宿主正确构造与使用。
- [x] 提供最小宿主适配器一致性测试套件，仅证明 claim/cancel/abort/fence、终态提交、自愈晋升和 采样发布的原子性、幂等及冲突语义。

C01 的人工测试任务发布边界已归入 C01/P0 前置约束；自愈 original recovery 与 old-base 失效已归入 P0-C。队列公平/优先级/retry/dead-letter、采样容量/保留/并发草稿修订号和凭据子系统不在本次范围。公共 API、存储结构与调用方直接迁移到最终模型，不保留兼容 facade、shim 或双轨入口；破坏性变化在最终 PR 说明中完整列出。

## 5. 依赖顺序

```text
RunSnapshot
 ├─ `LATEST` 解析
 ├─ Environment Properties / policies
 └─ Typed values
      ├─ invocation scopes
      └─ parameter snapshots
           └─ compiler integration

Terminal Evidence
 └─ Heal observation ingestion
      ├─ APPLIED auto-publish
      ├─ BELOW_CAP awaiting approval
      └─ original recovery reset

Claim/fence contract
 ├─ cancel queued
 ├─ abort active
 └─ atomic scheduling decisions

Stable sampling identities
 └─ publication mode mapping
      └─ temporary-to-formal rewrite
           └─ atomic sampling publication
```

## 6. 分阶段验收门槛

### 门槛 1：行为基线

- [x] 为所有拟调整行为先建立 RED 测试或确认保持现状。
- [x] `go test -race ./...`、`go vet ./...` 通过。
- [x] 当前公共契约和 architecture tests 固化。

### 门槛 2：领域层与 应用层

- [x] 领域层无 Repository/Store/Query/Projection。
- [x] 应用端口均按用例最小化。
- [x] 所有状态转换、错误和幂等语义有表驱动测试。
- [x] 不新增第二执行入口。

### 门槛 3：宿主事务证明

- [x] 执行实例 + 快照 + queue 创建全有或全无。
- [x] Cancel/claim、Abort/complete、Reorder/claim 竞态只有一个合法结果。
- [x] 终态 facts/status/streak/promotion 故障注入证明原子性。
- [x] sampling 发布任意写点失败均完整回滚。
- [x] 持有失效租约的工作器无法写进度或终态。

### 门槛 4：端到端替换

- [x] 发布资产 → 创建执行实例 → 封存快照 → 领取执行权 → compile → execute → 终态提交。
- [x] 类型化参数和 nested binding 可复现。
- [x] APPLIED 与 BELOW_CAP 治理路径完整。
- [x] Cancel/Reorder 与显式中止均已覆盖；中止原子提交 `ABORTED` 后发信号，信号失败返回 `ErrRunSignalRetryable`。
- [x] 采样 draft 可按四种 mode 发布。
- [x] 环境属性在每个顶层执行项的新浏览器中正确注入并可通过 `${env.<name>}` 使用。

## 7. 审核结论

- [x] 同意总体范围。
- [x] 同意 P0/P1 划分。
- [x] 同意核心库/宿主边界。
- [x] 同意按依赖顺序实施。
- [x] 退回修改：____________________________
