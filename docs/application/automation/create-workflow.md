# 创建工作流

形状 B（直接构造插入，无 CAS），共同的校验顺序、错误约定与适配器责任见 [模块 README](README.md)。以下只记录本用例独有的部分。

- 方法：`FlowFragmentService.Create(context.Context, workflow domain.FlowFragment、initial domain.FlowFragmentVersion)`
- 输出：`domain.FlowFragmentAggregate`
- 领域转换：`domain.NewFlowFragment` —— 校验首版定义及其步骤，强制 `Revision = 1`、`VersionNumber = 1`，把 `CurrentVersionID` 指向首版本。
- 端口：`FlowFragmentRepository.Create`；插入失败包装为 `persist workflow`。

## 源码

- [应用服务](../../../application/automation/workflow_service.go)
- [端口](../../../application/automation/workflow_repository.go)
- [领域转换](../../../domain/automation/lifecycle.go)
- [测试](../../../application/automation/asset_service_test.go)、[契约矩阵](../../../domain/automation/workflow_contract_matrix_test.go)
