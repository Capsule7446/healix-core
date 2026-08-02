# 发布节点版本

形状 A（先读后写，带 Revision CAS），共同的校验顺序、错误约定与适配器责任见 [模块 README](README.md)。以下只记录本用例独有的部分。

- 方法：`NodeService.PublishVersion(context.Context, id、versionID、pageURL、origin、selectors []fingerprint.Selector、fingerprint.Fingerprint、source domain.VersionSource、expected、at)`
- 输出：`domain.ElementTargetAggregate`
- 领域转换：`ElementTargetAggregate.PublishVersion` —— 追加一个不可变版本并切换 `CurrentVersionID`；版本号由 `nextNodeVersion` 推进，耗尽时返回 `AUTOMATION_VERSION_NUMBER_EXHAUSTED`，Revision 耗尽时返回 `AUTOMATION_REVISION_EXHAUSTED`。
- 端口：`NodeRepository.Load` / `SaveAggregate`；读取失败包装为 `load node %q`，写入失败包装为 `persist node %q`。

## 源码

- [应用服务](../../../application/automation/node_service.go)
- [端口](../../../application/automation/node_repository.go)
- [领域转换](../../../domain/automation/versioning.go)
- [测试](../../../application/automation/asset_service_test.go)、[用例矩阵](../../../application/automation/asset_usecase_matrix_test.go)
