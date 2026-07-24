# C06 — 参数快照

> 来源：Healix 仓库 `docs/refactor/healix-core-v0.3.0-replacement-assessment.md` 对应 Case；本清单以该评估要求为输入，并以 healix-core 当前 `master` 源码重新核验。

## 状态

**当前：输入 DTO 有占位，但 plan mapper 明确拒绝，未实现。**

## 业务不变量

每个 root execution 与 nested invocation 在入队前绑定不可变 typed 参数快照；retry/reclaim 不得重新解析。

## 当前证据

- `application/scheduling/plan_mapper.go`：`ParameterSnapshotInput`、`ParameterScopeInput`
- `domain/execution/plan.go`：`WorkflowEntry` 当前无 snapshot
- `domain/automation/test_task_types.go`：item parameters

## 调整清单

- [x] 新增 `ParameterSnapshot`，包含 schema version、workflow version 和 typed values。
- [x] root snapshot 关联 ExecutionID。
- [x] nested snapshot 关联 invocation scope ID。
- [x] 定义 canonical hash 与 map ordering。
- [x] Seal/clone/validation 覆盖所有 typed collections。
- [x] 校验 snapshot 与精确 WorkflowVersion schema 一致。
- [x] retry 直接加载 snapshot。
- [x] Environment Properties 使用独立的 `env.` 命名空间，不混入 Workflow ParameterSnapshot。

## 测试与验收

- [x] 同 Workflow 多 entries 可有不同 snapshots。
- [x] 修改来源 map/slice 不影响 snapshot。
- [x] map 顺序不同 hash 相同。
- [x] wrong WorkflowVersion snapshot 被拒绝。
- [x] parameterized required workflow 缺 snapshot 时不能入队。

## 依赖与风险

依赖 C02/C04/C05；主要风险是快照数量、canonical hash 稳定性和存储开销。

## 审核

- [x] 批准
- [x] 修改：________________
