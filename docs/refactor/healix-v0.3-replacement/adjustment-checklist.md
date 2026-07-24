# Healix → healix-core 差异整合：总体调整清单

> 原始评估：相邻 Healix 仓库 `C:\Users\Paul\workspace\healix\docs\refactor\healix-core-v0.3.0-replacement-assessment.md`。
> Case 映射：C01–C28 分别对应原始评估中同编号的表格行与详细章节。
> 核验基线：healix-core 当前 `master`（`595a74c`）；原始评估未在本审核包中固定 revision。
> 本文只定义调整范围与实施顺序，不代表已批准实施。

## 1. 总体判断

当前 Core 已具备稳定资产/不可变版本、密封执行计划、精确版本编译、节点状态机、验证稳定窗口、串行调度决策、运行期自愈和采样会话。真正阻碍完整替换的不是缺少零散结构体，而是以下业务闭环尚未形成：

1. 不可变 Run 快照与 Run 创建用例。
2. Typed 参数、嵌套作用域及每次执行参数快照。
3. 运行期 Heal Evidence 到 streak/candidate/promotion 的治理桥接。
4. Cancel/Abort/Reorder 的应用命令和并发语义。
5. 采样草稿到正式资产的规范化转换与原子发布。
6. Environment Properties 与执行策略进入 Run，并在每个 Entry 执行时注入变量上下文。

## 2. 整合原则

### 必须进入 Core

- 领域状态、业务不变量和确定性决策。
- 不可变版本、精确依赖、参数及策略快照。
- 发布、取消、中止、重排、晋升、审核的应用用例语义。
- 原子事务必须满足的业务边界、幂等和冲突分类。
- Host 需要实现的最小端口及适配器一致性测试。

### 必须留在 Host

- SQLite/SQL、事务实现、队列表、锁及 worker 存活检测。
- active-run goroutine/process registry 与 `context.CancelFunc`。
- Rod/CDP、DOM、截图、rrweb、文件和对象存储。
- Wails/UI DTO、Dashboard、查询投影和指标。

### 明确禁止

- 在 Core 中增加第二套 Runner 或兼容旧宿主的平行执行入口。
- 把数据库实体、UI ViewModel 或读模型搬进 Domain。
- 不在本次重构中预埋 CredentialReference、Provider、Vault/KMS 或兼容 facade。
- 让 Runtime 直接发布 NodeVersion；必须经 Evidence → 治理用例。
- 以接口存在代替“功能已闭环”的判断。

## 3. P0 调整清单

### P0-A：不可变 Run 创建边界（C02、C03、C07）

- [x] 设计 `RunSnapshot`，绑定 `RunID`、`TestTaskVersionID` 和 schema version。
- [x] 快照纳入精确 Workflow/Node/Reference 图。
- [x] 将根和嵌套 `LATEST` 从“发布时解析”迁到“Run 创建时解析”。
- [x] 快照 Environment revision、BaseURL 和完整 Properties。
- [x] 快照 ScreenshotPolicy 与 HealerPolicy。
- [x] 定义 canonical digest；同一 Run 不得替换快照内容。
- [x] 新增 CreateRun 应用服务，原子创建 Run、entries、snapshot 和 queue membership。
- [x] 定义 command identity、幂等重放及同 ID 异 payload 冲突。
- [x] 新模型直接替换旧 Run 创建契约；不保留无 RunSnapshot 的执行路径，既有不兼容 Run 数据不进入新执行链路。

### P0-B：Typed 参数闭环（C04、C05、C06）

- [x] 固定 WorkflowVersion 参数 schema 字段语义：`Name`→`${}` 变量名、`DisplayName`→TestTask 表单 title、`Description`→表单 desc、`Type`→输入控件和值类型。
- [x] TestTask 编辑/发布必须绑定所选精确 WorkflowVersion 的 parameter schema；仅保存 `Name`→typed value，显示元数据仍归 WorkflowVersion。
- [x] 用封闭 typed value 模型替换 `DefaultValue string`、TestTask 参数中的 `map[string]any` 以及密封边界中的字符串退化。
- [x] 默认值具有显式 presence，可区分“未提供”与合法空字符串。
- [x] 强制 `Required=false` 必须有合法 typed default；`Required=true` 不允许默认值兜底，根 Workflow 由 TestTask 人工输入，嵌套 Workflow 可由父级 binding 提供。
- [x] 支持 TEXT、NUMBER、BOOLEAN、SINGLE_SELECT、MULTI_SELECT。
- [x] 拒绝 unknown、duplicate、missing required 和类型不匹配参数。
- [x] 定义 NUMBER 精度/规范形式、BOOLEAN 接受形式、MULTI 顺序/去重规则。
- [x] 发布时验证 typed default 与 options；TestTask 输入也必须通过相同约束。
- [x] 建立 invocation graph 和稳定 call-path/scope ID。
- [x] 在 Run 创建时解析父子 binding、类型兼容及 optional default。
- [x] 为每个 root execution 和 nested invocation 冻结 ParameterSnapshot。
- [x] 参数上下文与运行时 extraction scratchpad 分离。
- [x] Compiler 只消费已解析 scope，不再补默认值或隐式 `fmt.Sprint` 转换。
- [x] 明确参数与 Environment Properties 的命名空间、快照和错误语义。

### P0-C：Heal 治理闭环（C10–C16、C23）

- [x] 保留“仅明确 NotFound 后触发 Heal”的错误分类边界。
- [x] 明确分离 runtime safety 与 asset governance。
- [x] `APPLIED` 与 `BELOW_CAP` 都可在 safety ALLOW 后恢复当前 Run。
- [x] `BELOW_CAP` 永远不能因 streak 自动发布。
- [x] 将 `HealStreakDecision.Promote bool` 改为 band-aware disposition。
- [x] 定义 observation 唯一键并按 Run 去重。
- [x] 实现 Evidence → candidate/streak observation 应用服务。
- [x] APPLIED 第三次成功：原子发布 NodeVersion、推进 current、关闭 streak。
- [x] BELOW_CAP 第三次成功：原子转 AwaitingApproval，不改变 Node current version。
- [x] 所有 candidate、streak 和 pending review 以 `NodeID + BaseNodeVersionID + CandidateHash` 绑定；禁止跨 NodeVersion 累计。
- [x] Node current version 一旦离开 `BaseNodeVersionID`，基于旧版本的 `OBSERVING/AWAITING_APPROVAL` candidate、未完成 streak 和 pending review 自动失效并退出活跃查询；新版本后续 Heal 必须建立新候选。
- [x] 旧版本 Heal observations、已晋升/已拒绝候选及审核记录作为审计历史保留，不做物理删除。
- [x] 采用所有治理操作前强制 current-base 校验；若 UI 要求立即移除旧待审项，再由 NodeVersion 推进事务或 outbox 驱动批量 `STALE` 投影。
- [x] Original selector 恢复：明确事实定义并原子 reset/stale 同 node/base 下相关候选。
- [x] 候选 hash 变化、重复/乱序事件均有确定规则。
- [x] 将 Step terminal、Validation、HealObservation、reset、streak、promotion 纳入同一事务意图。
- [x] 为 adapter 提供原子提交、幂等和并发阈值一致性测试套件。

### P0-D：队列与运行控制（C17–C23）

- [x] 将 TestTask 顶层 Workflow Entry 同时定义为浏览器会话隔离边界：每个 Entry 使用 Host 新建的 browser instance/context，结束后关闭，再启动下一 Entry。
- [x] 禁止跨 Entry 继承 cookies、LocalStorage、SessionStorage、IndexedDB、cache、页面和登录态；禁止复用持久 profile/user-data-dir。
- [x] 同一 Entry 内根 Workflow 与所有嵌套 WorkflowRef 共享该 Entry 的 browser session，嵌套调用不重开浏览器。
- [x] 定义 browser create/close 失败、Entry success/failure/abort 下的确定清理与 failure-policy 行为；隔离清理完成前不得推进下一 Entry。
- [x] 保留 `DecideAdvance` 纯函数和计划顺序唯一权威。
- [x] `DecisionWriter.ApplyDecision` 原子提交 skips、next/final status。
- [x] 为 Create/Cancel/Abort/Reorder 建独立应用服务，拆分宽泛 `RunCommands`。
- [x] Cancel queued 使用 expected status/revision，与 claim 原子竞争。
- [x] Abort active 必须先提交 ABORTED 并失效 fence，再发取消信号。
- [x] 定义“提交成功、信号失败”的可重试结果。
- [x] Queue reorder 带 queue scope、expected revision、command ID。
- [x] 完整 reorder 校验 exact permutation；partial move 使用独立命令。
- [x] 统一 stale fence 错误，覆盖调度、进度和终态写入。
- [x] 定义 worker 所有权失效及 stale fence 语义；具体存活检测机制留 Host。

### P0-E：采样发布闭环（C24–C27）

- [x] 保留 Session 生命周期与 CaptureID 首次结果幂等语义。
- [x] 为草稿 insert/update/delete/move/reorder 建领域命令，稳定临时 ID。
- [x] 明确 `IdentityKey`、`NodeUUID`、`TemporaryNodeID` 的统一术语与映射。
- [x] 实现 Snapshot → SamplingPublication 的 application mapper。
- [x] 保留 CREATE/FORCE_CREATE/MERGE/REUSE mode 到审计结果，不在规范化后丢失意图。
- [x] MERGE 保留已有 Node 身份与元数据，以采样得到的 PageURL/Origin/Selectors/Fingerprint 发布完整的新 NodeVersion；不得与旧版本隐式合并 selector 或 fingerprint。
- [x] MERGE 使用 expected current version/revision CAS，并将 Workflow 引用精确重写到新建的 `(NodeID, NodeVersionID)`。
- [x] 实现临时引用递归重写（root、repeat children、validation branches）。
- [x] 校验 input temporary IDs 与 result mappings exact-set equality。
- [x] publication ID 同 payload 重放返回原 mapping；异 payload 返回 identity conflict。
- [x] 节点、版本、mapping、WorkflowVersion、outbox 同一原子事务。

### P0-F：Environment Properties 注入（C28）

- [x] 将 Environment 收敛为一组规范化 Properties，移除 Variables/Properties/CredentialReferences 的重复模型。
- [x] 不区分 property 的机敏程度，移除 credential-like key 保留字。
- [x] 移除 Core 的 CredentialReference、CredentialService、Provider resolution、Vault/rotation/reveal 等专用契约。
- [x] Run 保存所选 Environment 的 ID/revision/BaseURL/Properties，并按 C07 的快照语义执行。
- [x] 每个顶层 Entry 创建新浏览器后，将同一组 Environment Properties 注入变量上下文。
- [x] 统一使用 `${env.<name>}`，并与普通 Workflow 参数命名空间隔离。
- [x] 缺失 Environment property 返回明确变量错误，不静默使用空字符串。
- [x] 未来若需要凭据安全管理，作为独立需求重新设计，不在本次预埋抽象。

## 4. P1 调整清单

- [x] C09 补齐 ValidationGroup 的 winning BranchID、terminal reason 和 collection-valued expected/actual evidence，使结果可解释并可重建。
- [x] 定义 Heal 已拒绝候选的重现规则：同一 `NodeID + BaseNodeVersionID + CandidateHash` 在 base 仍为 current 时不重新建立候选或累计 streak；新 base 可重新观察。
- [x] 扩展 `contract/public_api_test.go`，验证 Automation、Scheduling、Execution、Sampling 的公开服务和端口可由外部 Host 正确构造与使用。
- [x] 提供最小 Host adapter conformance suites，仅证明 claim/cancel/abort/fence、terminal commit、Heal promotion 和 Sampling publication 的原子性、幂等及冲突语义。

C01 的人工 TestTask 发布边界已归入 C01/P0 前置约束；Heal original recovery 与 old-base 失效已归入 P0-C。队列公平/优先级/retry/dead-letter、Sampling 容量/保留/并发草稿 revision 和凭据子系统不在本次范围。公共 API、存储结构与调用方直接迁移到最终模型，不保留兼容 facade、shim 或双轨入口；破坏性变化在最终 PR 说明中完整列出。

## 5. 依赖顺序

```text
RunSnapshot
 ├─ LATEST resolution
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

### Gate 1：行为基线

- [x] 为所有拟调整行为先建立 RED 测试或确认保持现状。
- [x] `go test -race ./...`、`go vet ./...` 通过。
- [x] 当前公共契约和 architecture tests 固化。

### Gate 2：Domain 与 Application

- [x] Domain 无 Repository/Store/Query/Projection。
- [x] 应用端口均按用例最小化。
- [x] 所有状态转换、错误和幂等语义有表驱动测试。
- [x] 不新增第二执行入口。

### Gate 3：Host 事务证明

- [x] Run + snapshot + queue 创建全有或全无。
- [x] Cancel/claim、Abort/complete、Reorder/claim 竞态只有一个合法结果。
- [x] terminal facts/status/streak/promotion 故障注入证明原子性。
- [x] sampling publication 任意写点失败均完整回滚。
- [x] stale worker 无法写进度或终态。

### Gate 4：端到端替换

- [x] 发布资产 → 创建 Run → seal snapshot → claim → compile → execute → terminal commit。
- [x] Typed 参数和 nested binding 可复现。
- [x] APPLIED 与 BELOW_CAP 治理路径完整。
- [x] Cancel/Abort/Reorder 用户场景完整。
- [x] Sampling draft 可按四种 mode 发布。
- [x] Environment Properties 在每个 Entry 的新浏览器中正确注入并可通过 `${env.<name>}` 使用。

## 7. 审核结论

- [x] 同意总体范围。
- [x] 同意 P0/P1 划分。
- [x] 同意 Core/Host 边界。
- [x] 同意按依赖顺序实施。
- [x] 退回修改：____________________________
