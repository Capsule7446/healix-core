# 06 边界决策更新

## 已确认共识

- 调度属于 Application，不属于 Workspace Domain。
- Core 不维护聚合视图、业务列表、仪表盘或页面读模型。
- 实际开发项目根据具体 UI/API 需求实现业务读模型。
- Core 只保留资产领域规则、执行领域、不变量、浏览器执行端口、自愈决策和执行事实契约。

## 已落地

- `application/scheduling` 拥有运行队列和运行生命周期端口。
- `domain/workspace.WorkspaceWriter` 不再包含 TestRunWriter。
- `application/readmodel` 已删除。
- `domain/evidence` 拥有独立执行事实和终态提交契约。
- Workspace 的旧执行事实提交端口已删除。

## Core 最终职责

```text
domain/workspace  -> 资产、版本、发布不变量
 domain/execution  -> 运行状态、执行计划和执行快照
 domain/evidence   -> 执行事实、验证、自愈、网络证据
 domain/node       -> 可执行节点树和浏览器执行状态
 domain/heal       -> 自愈候选评分与安全决策
 application/engine     -> 编译和单次运行编排
 application/scheduling -> 调度和运行生命周期用例
```

## 尚未完成

- 执行计划类型仍需从 Workspace 物理迁移到 `domain/execution`。
- Workspace `evidence.go` 仍因依赖 TestTaskRunPlan 而保留组合模型。
- Node 内部 ValidationObservation 需要通过 Application mapper 映射到 domain/evidence。
- `domain/workspace.test_task_types.go` 仍同时承载资产版本类型与执行快照类型，需要先拆出共享资产快照值对象，再删除执行计划。
