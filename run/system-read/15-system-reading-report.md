# 15 系统阅读报告

## 当前边界

Healix Core 当前由以下领域组成：

- `domain/automation`：Environment、Node/NodeVersion、Workflow/WorkflowVersion、TestTask/TestTaskVersion、Folder forest、heal candidate governance 与聚合 Revision。
- `domain/execution`：sealed multi-entry Plan、Run、WorkflowExecution、顺序与失败策略快照。
- `domain/evidence`：不可变 progress facts、terminal step commit、validation 与 healing observations。
- `domain/sampling`：瞬态浏览器采样与编辑状态。
- `domain/fingerprint`、`domain/interpolation`：共享内核。

`domain/workspace` 与 `domain/metrics` 已物理删除，不再是公开或内部契约。

## Application 编排

- `application/automation` 加载聚合、检查 expected Revision、调用 immutable Domain transition，并通过窄 repository 端口执行 CAS。
- `application/scheduling` 构造一个包含多个有序 peer Entries 的 sealed Plan，并由 Coordinator 调用 `DecideAdvance` 统一决定下一个 Entry、typed skip suffix 与 Run 终态。
- `application/execution` 定义 Worker-fenced Evidence ports 与 credential resolution boundary。
- `application/engine` 为每个 Entry 独立编译 Program 并运行；不解析 LATEST，不访问 Automation persistence。

## 执行语义

一次 TestTaskRun 生成一个 Plan。`Entries` 是有显式顺序的平级 Workflow 入口；内部 Workflow references 只是依赖，不是 Entry。Entry 串行执行：`STOP_ON_FAILURE` 会将后缀标记为 SKIPPED，`CONTINUE_ON_FAILURE` 允许后续 Entry 继续。Run 状态聚合全部 Entry 结果。

## 写侧一致性

- Automation 聚合使用非零、单调且不可复用的 Revision；adapter 必须原子比较 expected Revision。
- Folder 删除同时比较 forest Revision、目标 Folder identity 与 occupancy Revision。
- Heal candidate 审批由 Application 验证 candidate identity、当前 base version 和 node Revision，再原子提交 candidate + promoted Node aggregate。
- Worker-originated scheduling/Evidence 写入必须校验 claim fencing token。
- terminal step phase、final validations、healing facts 与 resets 只能通过 `StepTransitionCommit` 的 CAS/idempotency 事务提交。
- progress ports 只接受无法表达 terminal 状态的专用类型。

## 凭据边界

Automation 只持久化 logical name 到 `CredentialReference` 的映射。调用者只能使用 run identity 与 logical name 请求凭据；Application authorizer 从权威、不可变运行绑定解析 reference，再交给 SecretProvider。secret value 不得进入聚合、Plan、日志、Evidence 或业务投影。

## 读侧所有权

Core 不提供 `NodeQueryResult`、`WorkflowQueryResult`、`TestTaskQueryResult`、`Dashboard`、`ExecutionDetail` 或 Metrics projection。消费项目基于 Automation 资产和 immutable Evidence/Execution facts 建立自己的查询 DTO、缓存、刷新策略、API 与 UI。

最终契约见 `docs/architecture/domains.md` 与 `docs/architecture/application-orchestration.md`。
