# 更新环境

形状 A（先读后写，带 Revision CAS），共同的校验顺序、错误约定与适配器责任见 [模块 README](README.md)。以下只记录本用例独有的部分。

- 方法：`EnvironmentService.Update(context.Context, id、displayName、baseURL、variables domain.EnvironmentVariables、expected、at)`
- 输出：`domain.Environment`
- 领域转换：`Environment.UpdateMetadata` —— 只改元数据；已软删除的环境返回 `DeletedAggregateError()`，Revision 由领域自己推进一次。
- 端口：`EnvironmentRepository.Load` / `Update`；读取失败包装为 `load environment %q`，写入失败包装为 `persist environment %q`。

## 源码

- [应用服务](../../../application/automation/environment_service.go)
- [端口](../../../application/automation/environment_repository.go)
- [领域转换](../../../domain/automation/lifecycle.go)
- [测试](../../../application/automation/environment_service_test.go)
