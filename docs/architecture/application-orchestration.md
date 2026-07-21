# Application 编排

## 原则

Application 负责用例顺序、事务意图、expected Revision、执行顺序和失败策略；Domain 负责状态转换和不变量；适配器只实现 Application 在使用处声明的小端口，不能自行决定业务推进。

## Automation 命令

| 能力 | Application 服务 | Domain 行为 | 持久化端口 |
|---|---|---|---|
| Environment 新增 | `EnvironmentService.Create` | `NewEnvironment` | `EnvironmentRepository.Create` |
| Environment 修改/删除/恢复 | `EnvironmentService` | `UpdateMetadata/Delete/Restore` | `Load + Update(expected Revision)` |
| Node 新增 | `NodeService.Create` | `NewNode` | `NodeRepository.Create` |
| Node 元数据/删除/恢复 | `NodeService` | immutable transition | `Load + SaveAggregate(expected Revision)` |
| Node 发布新版本 | `NodeService.PublishVersion` | `NodeAggregate.PublishVersion` | 同上 |
| Workflow 新增 | `WorkflowService.Create` | `NewWorkflow` | `WorkflowRepository.Create` |
| Workflow 元数据/删除/恢复 | `WorkflowService` | immutable transition | `Load + SaveAggregate(expected Revision)` |
| Workflow 发布新版本 | `WorkflowService.PublishVersion` | `WorkflowAggregate.PublishVersion` | 同上 |
| TestTask 新增 | `TestTaskService.Create` | `NewTestTask` | `TestTaskRepository.Create` |
| TestTask 发布保存 | `TestTaskService.SavePublished` | 聚合 Validation + 单次 Revision 推进 | `Load + SaveAggregate(expected Revision)` |
| Sampling 发布 | `SamplingPublicationService.Publish` | `SamplingPublication.Validate` | 单个原子 `Publish` 端口 |
| Folder 保存/删除/移动 | `FolderService` | `FolderForest` invariants | 所有读写携带 FolderKind；删除原子比较 FolderID、forest Revision 与 occupancy Revision |
| Heal candidate 批准/拒绝 | `HealReviewService` | verified candidate identity/base-current + `NodeAggregate.PublishVersion` | reviewer/时间来自授权端口与可信 Clock；原子 review commit 比较 candidate/node Revisions |

所有更新流程固定为：

1. Load 当前聚合；
2. 对比命令 expected Revision；
3. 调用返回新值的 Domain transition；
4. 以 expected Revision 执行 CAS save；
5. 返回适配器持久化后的 struct。

适配器仍必须在数据库层执行 CAS；Application 的预检查用于稳定错误和避免无效转换，不能替代原子条件更新。

## Plan 构造

`BuildExecutionPlan` 接收已发布 `TestTaskVersionPlan` 和每个 Entry 的运行身份：

1. 验证 Automation publication 依赖闭包；
2. 按 `(WorkflowID, WorkflowVersionID)` 解析每个 Entry，因此同一 Workflow 的多个固定版本不会互相覆盖；
3. 保持 TestTaskItemID、ExecutionID 和 SequenceNumber；
4. 映射全部 Workflow/Node/Reference 快照；
5. 映射 FailurePolicy；
6. 调用 `execution.Seal`。

尚未定义可无损映射的参数来源会被显式拒绝，不会静默丢弃。参数优先级必须在 TestTask item、run scope、Workflow defaults、reference bindings、Environment variables 和 CredentialReference 之间形成单独的业务决策后再开放。

## Worker 调度

### 端口

- `RunCommands`：创建、取消、删除 Run；
- `QueueOrderWriter`：队列重排；
- `ClaimSource`：按 worker 身份领取一个 Plan，返回不透明 fencing token，并在每个已领取路径显式 Release；
- `EntryStateReader`：在 Claim 下读取全部 Entry 状态；
- `DecisionWriter`：在同一个 Claim 下原子应用 Decision。

旧的大 `Scheduler` 接口已删除。适配器不再直接拥有 Start/Finish/Finalize 的独立调用权限。

### Coordinator

`Coordinator.ProcessNext` 的当前闭环：

```text
ClaimNext(worker, now)
  -> 验证 sealed Plan 与非空 fencing token
  -> LoadEntryStates(claim)
  -> DecideAdvance(plan, states)
  -> ApplyDecision(claim, decision, now)
```

`DecideAdvance` 是顺序和失败策略的唯一生产决策点：

- 返回 `NextExecutionID` 表示原子打开下一个 Entry；
- 返回 `Transitions` 表示按序持久化 typed skipped suffix；
- 返回 `FinalStatus` 表示 Run 可以终态化；
- Entry 正在 RUNNING 时返回空 Decision，不产生写入。

`DecisionWriter` 必须在一个事务中验证 fencing token、当前状态和 transition from-status，并保持 transition 顺序。相同 token 与相同 Decision 的重放应幂等；stale token 必须失败。

### 执行与 Evidence

被打开的 Entry 由 Engine 独立编译并执行。非终态进度通过 `ProgressWriter` 写入；终态步骤通过 `FactCommitter.CommitStepTransition` 原子写入：

```text
Expected step revision
  + terminal phase event
  + final validation facts
  + heal observations/resets
  -> one transaction
  -> new revision + idempotency result
```

Entry 终态持久化后，Worker 再次运行 Coordinator；Coordinator 根据最新状态打开后继 Entry、跳过后缀或终态化 Run。

## 失败、取消和恢复

- STOP_ON_FAILURE 与 CONTINUE_ON_FAILURE 由封存 Plan 决定，适配器不能覆盖；
- cancel 与 abort 使用不同 Entry 终态和 skip cause；
- 运行错误不能仅凭 `context.Context` 自动推断为 canceled 或 aborted；
- claim token 是所有 Worker 写入的 fencing 条件；
- lease 时长、heartbeat、过期和 interrupted recovery 尚未由 Domain 语义定义，因此 Core 不猜测这些策略；消费项目必须在增加恢复行为前明确合法状态转换和幂等规则。

## 查询职责

Core 不提供业务查询服务、Dashboard、Metrics、列表或统计投影。消费项目从 Automation 资产和 Evidence 事实建立自己的读库；这些查询不能反向进入 Domain 或影响命令聚合。

## 事务边界

必须由生产适配器集成测试验证：

- Aggregate expected Revision CAS；
- Sampling publication 全有或全无；
- Claim fencing token；
- Decision 状态 CAS 和有序批量 transition；
- StepTransitionCommit Revision CAS、CommitID 唯一性和相同重放；
- 失败时 rollback；
- 不可变版本号与稳定 ID 唯一约束。
