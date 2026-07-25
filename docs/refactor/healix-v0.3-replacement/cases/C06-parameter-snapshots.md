# C06 — 参数快照

> 来源：历史替换评估中的同编号案例；本清单以该评估要求为输入，并以 healix-core 当前实现（`12d1ba2`）重新核验。

## 状态

**当前结果：已由 v0.3 替换实现覆盖；以下证据、清单与验收项按当前模型解释。**

## 业务不变量

每个 root execution 与 nested invocation 在入队前绑定不可变 typed 参数快照；retry/reclaim 不得重新解析。

## 当前证据

- `application/scheduling/plan_mapper.go`：`ParameterSnapshotInput`、`ParameterScopeInput`
- `domain/execution/plan.go`：`WorkflowEntry` 当前无快照
- `domain/automation/test_task_types.go`：执行项参数

## 调整清单

- [x] 新增 `ParameterSnapshot`，包含 schema version、workflow version 和 typed values。
- [x] root 快照关联 ExecutionID。
- [x] nested 快照关联 invocation scope ID。
- [x] 定义 canonical hash 与 map ordering。
- [x] Seal/clone/validation 覆盖所有 typed collections。
- [x] 校验快照与精确 WorkflowVersion schema 一致。
- [x] retry 直接加载快照。
- [x] 环境属性使用独立的 `env.` 命名空间，不混入工作流 ParameterSnapshot。

## 测试与验收

- [x] 同工作流多 entries 可有不同 snapshots。
- [x] 修改来源 map/slice 不影响快照。
- [x] map 顺序不同 hash 相同。
- [x] wrong WorkflowVersion 快照被拒绝。
- [x] parameterized required workflow 缺快照时不能入队。

## 依赖与风险

依赖 C02/C04/C05；主要风险是快照数量、canonical hash 稳定性和存储开销。

## 审核

- [x] 批准
- [x] 修改：________________
