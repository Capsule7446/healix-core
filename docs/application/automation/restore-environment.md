# 恢复环境

形状 A（先读后写，带 Revision CAS），共同的校验顺序、错误约定与适配器责任见 [模块 README](README.md)。以下只记录本用例独有的部分。

- 方法：`EnvironmentService.Restore(context.Context, id、expected、at)`
- 输出：`domain.Environment`
- 领域转换：`Environment.Restore` —— 清除 `DeletedAt`；对未删除对象恢复是空转，领域以 `AUTOMATION_AGGREGATE_TRANSITION_INVALID` 拒绝。
- 端口：`EnvironmentRepository.Load` / `Update`；读取失败包装为 `load environment %q`，写入失败包装为 `persist environment %q`。

## 源码

- [应用服务](../../../application/automation/environment_service.go)
- [端口](../../../application/automation/environment_repository.go)
- [领域转换](../../../domain/automation/lifecycle.go)
- [测试](../../../application/automation/environment_service_test.go)
