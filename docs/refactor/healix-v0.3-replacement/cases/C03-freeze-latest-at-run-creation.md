# C03 — `LATEST` 版本冻结

> 来源：Healix 仓库 `docs/refactor/healix-core-v0.3.0-replacement-assessment.md` 对应 Case；本清单以该评估要求为输入，并以 healix-core 当前 `master` 源码重新核验。

## 状态

**当前：存在冲突。当前更接近发布时解析，而非 Run 创建时解析。**

## 业务不变量

`LATEST` 只属于 authoring policy；Run 创建时一次解析为精确版本，之后不得再次解析。

## 当前证据

- `domain/automation/test_task.go`：publication plan 的 latest resolution
- `domain/automation/test_task_types.go`：`ResolvedFromLatest`
- `application/scheduling/plan_mapper.go`：publication → execution 映射
- `domain/execution/validation.go`：执行引用必须精确

## 调整清单

- [x] 发布时仅验证 symbolic policy 合法，不冻结 latest target。
- [x] Run 创建时在一致 catalog snapshot 内解析根和递归引用。
- [x] 将精确 ID 写入 entry、reference 和 resolution。
- [x] 保存 `ResolvedFromLatest` 作为 provenance。
- [x] Seal 后禁止 current-version lookup。
- [x] 直接移除发布时冻结 LATEST 的旧路径，不保留双重解析语义。

## 测试与验收

- [x] Task 发布后 Workflow 升版，随后创建 Run 应选择新版本。
- [x] Run 创建后再次升版不影响旧 Run。
- [x] fixed reference 始终不变。
- [x] nested latest 在同一一致性视图中解析。
- [x] sealed plan 内无 symbolic latest 或空 version ID。

## 依赖与风险

依赖 C02；需重定义 `TestTaskVersionPlan` 的 publication 与 run-lock 职责。

## 审核

- [x] 批准
- [x] 修改：________________
