# 删除工作流

形状 A（先读后写，带 Revision CAS），共同的校验顺序、错误约定与适配器责任见 [模块 README](README.md)。以下只记录本用例独有的部分。

- 方法：`FlowFragmentService.Delete(context.Context, id、expected、at)`
- 输出：`domain.FlowFragmentAggregate`
- 领域转换：`FlowFragmentAggregate.Delete` —— 软删除聚合，保留定义版本；对已删除聚合再删是空转，领域以 `AUTOMATION_AGGREGATE_TRANSITION_INVALID` 拒绝。
- 端口：`FlowFragmentRepository.Load` / `SaveAggregate`；读取失败包装为 `load workflow %q`，写入失败包装为 `persist workflow %q`。

## 源码

- [应用服务](../../../application/automation/workflow_service.go)
- [端口](../../../application/automation/workflow_repository.go)
- [领域转换](../../../domain/automation/lifecycle.go)
- [测试](../../../application/automation/asset_service_test.go)、[契约矩阵](../../../domain/automation/workflow_contract_matrix_test.go)
