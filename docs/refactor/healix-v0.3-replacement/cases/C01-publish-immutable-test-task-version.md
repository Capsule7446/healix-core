# C01 — 发布不可变 TestTask 版本

> 来源：Healix 仓库 `docs/refactor/healix-core-v0.3.0-replacement-assessment.md` 对应 Case；本清单以该评估要求为输入，并以 healix-core 当前 `master` 源码重新核验。

## 状态

**当前：大体已实现；发布封装和深拷贝仍需增强。**

## 业务不变量

TestTask 是稳定身份；TestTaskVersion 是只追加、不可变的可执行配置。发布只能新增版本，历史版本不得被改写。

## 当前证据

- `domain/automation/test_task_types.go`：`TestTask`、`TestTaskVersion`、`TestTaskAggregate`
- `domain/automation/test_task.go`：版本历史、连续编号、lineage/current 校验
- `application/automation/test_task_service.go`：`SavePublished` 与 revision 校验
- `domain/automation/lifecycle.go`：聚合 clone

## 调整清单

- [x] 若需要领域发布入口，增加 `TestTaskAggregate.PublishVersion`：仅接收人工编写并已校验的 TestTask 草稿内容，由聚合分配连续 `VersionNumber`、记录上一版 `SourceVersionID` 血缘，并推进 `CurrentVersionID`/`Revision`。
- [x] TestTaskVersion 不接受 Sampling 或 Heal 产物；Sampling 只发布 Workflow、WorkflowVersion、Node、NodeVersion，Heal 只晋升 NodeVersion。
- [x] 禁止调用方组装整份“新聚合”绕过发布命令。
- [x] Typed values 落地后，对嵌套 map/slice 做真正深拷贝。
- [x] Repository 契约声明历史版本 append-only。
- [x] 并发发布使用 expected revision/CAS。

## 测试与验收

- [x] 发布后修改输入对象不影响任何版本。
- [x] 每次仅新增一个连续版本。
- [x] 两个同 revision 并发发布仅一个成功。
- [x] current 指针始终指向最高版本。

## 依赖与风险

依赖 typed value 模型和适配器 CAS；现有整聚合构造调用方需要迁移。

## 审核

- [x] 批准
- [x] 修改：________________
