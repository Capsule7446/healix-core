# C03 — `LATEST` 版本冻结

> 来源：历史替换评估中的同编号案例；本清单以该评估要求为输入，并以 healix-core 当前实现（`12d1ba2`）重新核验。

## 状态

**当前结果：已由 v0.3 替换实现覆盖；以下证据、清单与验收项按当前模型解释。**

## 业务不变量

`LATEST` 只属于创作 policy；执行实例创建时一次解析为精确版本，之后不得再次解析。

## 当前证据

- `domain/automation/test_task.go`：仅校验发布计划
- `application/scheduling/create_run_service.go`：`CreateRunService.CreateRun` 在执行实例创建事务内调用 `CreateRunTx.ResolveCreateRun`
- `application/scheduling/create_run_types.go`：`CreateRunTx.ResolveCreateRun` 契约要求从同一事务视图解析递归 workflow `LATEST`/current 指针，并以 `ResolvedCreateRun` 返回精确资产与 invocation
- `application/scheduling/create_run_builder.go`：`BuildRunSnapshot` 将已解析的精确 workflow version 写入 entry、递归执行计划与 invocation，并封存执行实例快照
- `application/scheduling/plan_mapper.go`：调度解析器一致性证据
- `domain/automation/test_task_types.go`：`ResolvedFromLatest`
- `domain/execution/validation.go`：执行引用必须精确

## 调整清单

- [x] 发布时仅验证 symbolic policy 合法，不冻结 latest target。
- [x] 执行实例创建时在一致 catalog 快照内解析根和递归引用。
- [x] 将精确 ID 写入 entry、reference 和 resolution。
- [x] 保存 `ResolvedFromLatest` 作为 provenance。
- [x] Seal 后禁止 current-version lookup。
- [x] 直接移除发布时冻结 LATEST 的旧路径，不保留双重解析语义。

## 测试与验收

- [x] Task 发布后工作流升版，随后创建执行实例应选择新版本。
- [x] 执行实例创建后再次升版不影响旧执行实例。
- [x] fixed reference 始终不变。
- [x] nested latest 在同一一致性视图中解析。
- [x] 已封存 plan 内无 symbolic latest 或空 version ID。

## 依赖与风险

依赖 C02；需重定义 `TestTaskVersionPlan` 的发布与 run-lock 职责。

## 审核

- [x] 批准
- [x] 修改：________________
