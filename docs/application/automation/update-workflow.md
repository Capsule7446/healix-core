# 更新工作流

形状 A（先读后写，带 Revision CAS），共同的校验顺序、错误约定与适配器责任见 [模块 README](README.md)。以下只记录本用例独有的部分。

- 方法：`FlowFragmentService.Update(context.Context, id、displayName、folderID、properties domain.Properties、expected、at)`
- 输出：`domain.FlowFragmentAggregate`
- 领域转换：`FlowFragmentAggregate.UpdateMetadata` —— 只改元数据，不改定义也不切换当前版本。
- 端口：`FlowFragmentRepository.Load` / `SaveAggregate`；读取失败包装为 `load workflow %q`，写入失败包装为 `persist workflow %q`。

## 源码

- [应用服务](../../../application/automation/workflow_service.go)
- [端口](../../../application/automation/workflow_repository.go)
- [领域转换](../../../domain/automation/lifecycle.go)
- [测试](../../../application/automation/asset_service_test.go)、[契约矩阵](../../../domain/automation/workflow_contract_matrix_test.go)
