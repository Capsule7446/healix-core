# 发布测试任务版本

形状 A（先读后写，带 Revision CAS），共同的校验顺序、错误约定与适配器责任见 [模块 README](README.md)。以下只记录本用例独有的部分。

- 方法：`ExecutionFlowService.PublishVersion(context.Context, taskID、expected、publication domain.ExecutionFlowVersionPublication)`
- 输出：`domain.ExecutionFlowAggregate`
- 领域转换：`ExecutionFlowAggregate.PublishVersion` —— 调用方提交的是一份 `ExecutionFlowVersionPublication`，由聚合追加版本、把 `SourceVersionID` 指向原当前版本并推进 Revision，而不是由调用方构造整个已发布聚合。版本 ID 重复或 `CreatedAt` 非单调时返回 `AUTOMATION_AGGREGATE_TRANSITION_INVALID`。
- 端口：`ExecutionFlowRepository.Load` / `SaveAggregate`；读取失败包装为 `load test task %q`，写入失败包装为 `persist test task %q`。
- 独有的前置校验：本方法在服务中直接展开，不调用共用的 `transition` 辅助函数。除 taskID 外还要求 `publication.ID` 非空，并在调用 `Load` 前返回未分类的校验错误。

## 源码

- [应用服务](../../../application/automation/test_task_service.go)
- [端口](../../../application/automation/test_task_repository.go)
- [领域转换](../../../domain/automation/lifecycle.go)
- [测试](../../../application/automation/test_task_service_test.go)
