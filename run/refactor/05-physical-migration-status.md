# 05 物理迁移状态

## 已完成

- 读模型已从 Workspace 移至 `application/readmodel`，旧查询模型已删除。
- 应用执行端口已建立于 `application/execution`。
- 运行生命周期已建立于 `domain/execution`，并通过独立状态机测试。
- 执行凭据已建立 `CredentialReference`/`SecretResolver` 边界。
- Node 已通过 `HealingPort` 隔离 Heal 调用。
- `domain/evidence` 已建立独立事实内核，确认新的证据上下文边界。

## 尚未完成

- 现有 `domain/workspace/evidence.go` 和 `execution_facts.go` 仍包含旧证据定义。
- `TestTaskRunPlan`、依赖快照和工作流执行计划仍位于 Workspace。
- `EnvironmentSnapshot` 仍有旧的明文 Username/Password 字段。
- Application ports 仍引用 workspace 事实类型，需切换到 evidence 类型。

## 下一物理迁移

将完整证据类型迁移到 `domain/evidence`，先迁移无 Workspace 聚合依赖的观察/网络/步骤记录，再迁移提交契约；之后迁移执行计划类型，并删除 Workspace 中的旧定义。每批迁移必须更新所有测试和架构边界后通过 race/vet。
