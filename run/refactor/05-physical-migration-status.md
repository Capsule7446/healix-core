# 05 物理迁移状态

## 已完成

- 读模型已从 Workspace 移至 `application/readmodel`，旧查询模型已删除。
- 应用执行端口已建立于 `application/execution`。
- 运行生命周期已建立于 `domain/execution`，并通过独立状态机测试。
- 执行凭据已建立 `CredentialReference`/`SecretResolver` 边界。
- Node 已通过 `HealingPort` 隔离 Heal 调用。
- `domain/evidence` 已建立独立事实内核。
- `HealObservation`、`ValidationObservation`、`StepPhaseEvent` 和终态提交契约已在 `domain/evidence` 建立。
- 应用执行端口已切换为依赖 `domain/evidence`。
- Workspace 已删除 EvidenceWriter、ExecutionFactCommitter 和 ExecutionProgressWriter。

## 尚未完成

- Workspace 中仍保留旧证据类型，尚未物理删除。
- `TestTaskRunPlan`、依赖快照和工作流执行计划仍位于 Workspace。
- 旧 `EnvironmentSnapshot` 已移除明文凭据字段，但执行计划仍未使用 `domain/execution.EnvironmentDescriptor`。

## 下一物理迁移

将执行计划类型真正迁移到 `domain/execution`，再处理 Workspace 中剩余的证据类型删除和所有内部消费者切换。每批迁移必须更新所有测试和架构边界后通过 race/vet。
