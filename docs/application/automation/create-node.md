# 创建节点

形状 B（直接构造插入，无 CAS），共同的校验顺序、错误约定与适配器责任见 [模块 README](README.md)。以下只记录本用例独有的部分。

- 方法：`NodeService.Create(context.Context, node domain.ElementTarget、initial domain.ElementTargetVersion)`
- 输出：`domain.ElementTargetAggregate`
- 领域转换：`domain.NewElementTarget` —— 同时校验节点与首版本，强制 `Revision = 1`、`VersionNumber = 1`，把 `CurrentVersionID` 指向首版本，并要求三个创建时间戳一致。
- 端口：`NodeRepository.Create`；插入失败包装为 `persist node`。

## 源码

- [应用服务](../../../application/automation/node_service.go)
- [端口](../../../application/automation/node_repository.go)
- [领域转换](../../../domain/automation/lifecycle.go)
- [测试](../../../application/automation/asset_service_test.go)、[用例矩阵](../../../application/automation/asset_usecase_matrix_test.go)
