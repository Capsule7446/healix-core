# 领域边界

## 总览

Core 采用四个明确边界和两个共享内核。`domain/workspace` 与 `domain/metrics` 已删除；Core 不再拥有业务 Dashboard、列表、搜索或统计投影。

| 边界 | 所有权 | 不拥有 |
|---|---|---|
| Automation | 可持久化自动化资产、不可变版本、文件夹、TestTask、采样发布、自愈候选治理 | 运行状态、浏览器会话、业务查询投影 |
| Execution | 封存执行计划、Run 与 Entry 生命周期、执行预算和图不变量 | 资产编辑、持久化端口、网络访问 |
| Evidence | 不可变执行事实、阶段事件、验证和自愈观察、原子终态提交契约 | Dashboard 聚合、资产生命周期 |
| Sampling | 临时浏览器采样、捕获和编辑状态 | 持久化资产和运行生命周期 |
| Fingerprint | 节点指纹和选择器共享值 | 聚合与编排 |
| Interpolation | 插值语法和解析共享规则 | 参数优先级和秘密解析 |

依赖方向为 `Application -> Domain`。Domain 只能依赖同一边界或共享内核；Infrastructure/消费项目实现 Application 声明的端口。

## Automation

### 聚合

- `Environment`：环境元数据、BaseURL、普通变量、普通属性和 `CredentialReference`。
- `NodeAggregate`：稳定 Node、Current NodeVersion 和不可变版本历史。
- `WorkflowAggregate`：稳定 Workflow、Current WorkflowVersion、步骤图和参数定义。
- `TestTaskAggregate`：稳定 TestTask、Current TestTaskVersion 和有序 Workflow 项。
- Folder：Automation 资产的持久化层级。
- HealCandidate：候选审批、拒绝和连续观察治理。

### 并发与版本

所有可变聚合使用独立 `Revision`：

- 持久化值从 1 开始，0 非法；
- 每次成功聚合转换只增加一次；
- 溢出失败，不回绕、复用或重置；
- Revision 与不可变发布 `VersionNumber`、时间戳相互独立；
- Application 传递 expected Revision，适配器必须以 CAS 原子保存。

NodeVersion、WorkflowVersion 和 TestTaskVersion 一旦发布即不可变。发布新版本返回新聚合值，不修改调用者持有的原对象或历史切片。

### 凭据边界

Environment 只持久化 logical name 到 `CredentialReference` 的映射，不持久化解析后的秘密。Execution Application 的 `CredentialService` 只接受带有效 `WorkerScope.ClaimToken` 的运行身份和 logical name；`CredentialAuthorizer` 从权威运行绑定解析 reference，再交给 `SecretProvider`。secret value 不得进入聚合、Plan、日志、Evidence 或读模型。普通字符串无法按内容可靠判定是否为秘密，因此 Core 不宣称能够自动保护被调用者错误写入普通 literal 的凭据。

## Execution

### 一个 Run、一个 Plan、多个 Entry

一次 TestTaskRun 生成一个封存的 `execution.Plan`。Plan 使用有序 `Entries []WorkflowEntry`：

- 每个 Entry 对应一个独立 WorkflowExecution；
- `SequenceNumber` 必须从 1 开始连续且唯一；
- 同一 Workflow 的不同固定版本可同时成为 Entry；
- `Workflows` 保存所有 Entry 和递归引用依赖的精确版本快照；
- `References` 只保存 Workflow 内部引用，被引用 Workflow 不是平级 Entry；
- 每个 Entry 独立编译为一个 Program。

Plan 只能通过 `Seal(Draft)` 创建。零值或未封存 Plan 无效；访问器返回防御性副本。

### 顺序与失败策略

Entry 严格串行，最多一个处于 RUNNING：

- `STOP_ON_FAILURE`：失败 Entry 后的 PENDING 后缀转换为 SKIPPED；
- `CONTINUE_ON_FAILURE`：失败后下一个 Entry 仍可执行；
- cancellation 和 abort 都停止后缀，并使用不同 typed skip cause；
- `RUNNING -> SKIPPED` 非法；只有 `PENDING -> SKIPPED` 合法。

Run 汇总所有 Entry：全成功为 SUCCEEDED；继续执行后的任意失败为 FAILED；停止失败及跳过后缀为 FAILED；取消为 CANCELED；活动执行中断为 ABORTED。

### 资源约束

Seal 验证依赖闭包、引用解析、环、NodeVersion 所有权、Step 结构、URL/插值语法、集合与字符串预算、图深度、重复次数、累计等待及 Run 范围的扩展执行预算。运行时宿主仍必须执行实际 CPU、网络、Evidence 和 artifact 配额。

## Evidence

Evidence 记录不可变事实，不提供业务读模型。`StepTransitionCommit` 将终态事件、最终验证、自愈观察和 selector reset 作为一个原子提交：

- `CommitID` 是幂等身份；
- `ExpectedRevision` 是终态 CAS 前置条件；
- 结果返回新 Revision 和是否首次应用；
- 相同 CommitID 的完全相同重放必须返回原结果；
- 相同 CommitID 的不同载荷和 stale Revision 必须返回显式冲突。

数据库事务、唯一约束和 CAS 由消费项目适配器验证。

## Sampling

Sampling 只拥有临时捕获和编辑状态。`SamplingPublication` 是进入 Automation 的显式发布契约；Application 通过一个原子端口发布 Node 决策与 Workflow，避免 Sampling 直接持久化 Automation 聚合。

## 消费项目职责

消费项目负责：

- Repository、事务、CAS、幂等表和 claim fencing token；
- 业务查询、Dashboard、指标和运行详情投影；
- HTTP、数据库、文件系统、浏览器和队列适配器；
- DNS/IP/端口/重定向/重绑定网络策略；
- 秘密存储，以及实现由 `CredentialService` 使用的 fenced `CredentialAuthorizer` 与 `SecretProvider`；
- 实际运行配额、日志脱敏和 artifact 生命周期。
