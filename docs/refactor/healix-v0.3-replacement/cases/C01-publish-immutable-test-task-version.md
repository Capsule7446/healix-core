# C01 — 发布不可变测试任务版本

> 来源：历史替换评估中的同编号案例；本清单以该评估要求为输入，并以 healix-core 当前实现（`12d1ba2`）重新核验。

## 状态

**当前结果：已由 v0.3 替换实现覆盖；以下证据、清单与验收项按当前模型解释。**

## 业务不变量

测试任务是稳定身份；TestTaskVersion 是只追加、不可变的可执行配置。发布只能新增版本，历史版本不得被改写。

## 当前证据

- `domain/automation/test_task_types.go`：`TestTask`、`TestTaskVersion`、`TestTaskAggregate`
- `domain/automation/test_task.go`：版本历史、连续编号、lineage/current 校验
- `application/automation/test_task_service.go`：人工保存已发布 TestTaskVersion 与修订号校验
- `domain/automation/lifecycle.go`：聚合 clone

## 调整清单

- [x] 若需要领域发布入口，增加 `TestTaskAggregate.PublishVersion`：仅接收人工编写并已校验的测试任务草稿内容，由聚合分配连续 `VersionNumber`、记录上一版 `SourceVersionID` 血缘，并推进 `CurrentVersionID`/`Revision`。
- [x] TestTaskVersion 不接受采样或 自愈产物；采样只发布工作流、WorkflowVersion、节点、NodeVersion，自愈只晋升 NodeVersion。
- [x] 禁止调用方组装整份“新聚合”绕过发布命令。
- [x] 类型化 values 落地后，对嵌套 map/slice 做真正深拷贝。
- [x] Repository 契约声明历史版本 append-only。
- [x] 并发发布使用 expected revision/CAS。

## 测试与验收

- [x] 发布后修改输入对象不影响任何版本。
- [x] 每次仅新增一个连续版本。
- [x] 两个同修订号并发发布仅一个成功。
- [x] current 指针始终指向最高版本。

## 依赖与风险

依赖 typed value 模型和适配器 CAS；现有整聚合构造调用方需要迁移。

## 审核

- [x] 批准
- [x] 修改：________________
