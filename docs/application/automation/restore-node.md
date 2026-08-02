# 恢复节点

形状 A（先读后写，带 Revision CAS），共同的校验顺序、错误约定与适配器责任见 [模块 README](README.md)。以下只记录本用例独有的部分。

- 方法：`NodeService.Restore(context.Context, id、expected、at)`
- 输出：`domain.ElementTargetAggregate`
- 领域转换：`ElementTargetAggregate.Restore` —— 清除 `DeletedAt`，不重写版本历史；对未删除节点恢复是空转，领域以 `AUTOMATION_AGGREGATE_TRANSITION_INVALID` 拒绝。
- 端口：`NodeRepository.Load` / `SaveAggregate`；读取失败包装为 `load node %q`，写入失败包装为 `persist node %q`。

## 源码

- [应用服务](../../../application/automation/node_service.go)
- [端口](../../../application/automation/node_repository.go)
- [领域转换](../../../domain/automation/lifecycle.go)
- [测试](../../../application/automation/asset_service_test.go)、[用例矩阵](../../../application/automation/asset_usecase_matrix_test.go)
