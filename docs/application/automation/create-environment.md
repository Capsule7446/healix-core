# 创建环境

形状 B（直接构造插入，无 CAS），共同的校验顺序、错误约定与适配器责任见 [模块 README](README.md)。以下只记录本用例独有的部分。

- 方法：`EnvironmentService.Create(context.Context, value domain.Environment)`
- 输出：`domain.Environment`
- 领域转换：`domain.NewEnvironment` —— 强制 `Revision = 1`，要求 `CreatedAt > 0` 且 `UpdatedAt == CreatedAt`（否则 `AUTOMATION_AGGREGATE_TRANSITION_INVALID`），并深拷贝 `Variables`。
- 端口：`EnvironmentRepository.Create`；插入失败包装为 `persist environment`。

## 源码

- [应用服务](../../../application/automation/environment_service.go)
- [端口](../../../application/automation/environment_repository.go)
- [领域转换](../../../domain/automation/lifecycle.go)
- [测试](../../../application/automation/environment_service_test.go)
