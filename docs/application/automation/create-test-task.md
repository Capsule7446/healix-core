# 创建测试任务

形状 B（直接构造插入，无 CAS），共同的校验顺序、错误约定与适配器责任见 [模块 README](README.md)。以下只记录本用例独有的部分。

- 方法：`ExecutionFlowService.Create(context.Context, task domain.ExecutionFlow、initial domain.ExecutionFlowVersion)`
- 输出：`domain.ExecutionFlowAggregate`
- 领域转换：`domain.NewExecutionFlow` —— 校验条目顺序、工作流版本策略与失败策略，强制 `Revision = 1`、`VersionNumber = 1`，并把首版本的 `SourceVersionID` 清空。
- 端口：`ExecutionFlowRepository.Create`；插入失败包装为 `persist test task`。

## 源码

- [应用服务](../../../application/automation/test_task_service.go)
- [端口](../../../application/automation/test_task_repository.go)
- [领域转换](../../../domain/automation/lifecycle.go)
- [测试](../../../application/automation/test_task_service_test.go)
