# 错误码边界说明

本页补充[业务错误码注册表](error-code-registry.md)中按上下文归属和公开边界使用错误码的规则。注册表是逐行校验的权威清单，本页只解释当前代码的产出位置和消费方式。

## 统一错误封套

- 公开业务失败使用 `fault.Error`，由 `fault.Kind`、稳定 `fault.Code`、安全 message、受限 params 和有序 `fault.Violation` 组成。
- `fault.CodeOf`、`fault.KindOf` 和 `fault.Describe` 报告边界故障的单一分类；`fault.IsCode` 检查整条错误链是否包含某个 code。调用方不得解析 `Error()` 文本。
- `Violation` 的 code 只能来自 `VALIDATION_FIELD_*` 共享词表，不能作为顶层错误码。一个聚合校验失败返回一个封套，最多保留 `fault.MaxViolations` 条。
- cause 只通过 `Unwrap` 保留给诊断；公共 message 和 params 不得包含身份、selector、URL、环境/参数值、堆栈或适配器细节。

## 产出边界

| 产出包 | 当前错误码前缀 | 公开边界 |
|---|---|---|
| `domain/automation` | `AUTOMATION_*` | 资产、版本、生命周期、参数和依赖校验。 |
| `application/automation` | `AUTOMATION_*` 与三个 `SAMPLING_*` | 仓储 CAS、采样发布事务和修复审核事务。三个采样发布码按消费上下文归入 `SAMPLING_*`。 |
| `domain/sampling` | `SAMPLING_*` | 会话、捕获和未发布工作区边界。 |
| `domain/execution`、`domain/node`、`application/engine`、`application/scheduling`、`application/execution` | `EXECUTION_*` | 执行实例、节点运行、编译、调度、领取、事实提交和恢复决策。 |
| `domain/evidence` | `EVIDENCE_*` | 进度、终态事实、验证/修复观察和原子提交值。 |
| `domain/fingerprint` | `FINGERPRINT_*` | selector、指纹和框架检测输入。 |
| `domain/interpolation` | `INTERPOLATION_*` | 变量表达式、解析器和展开预算。 |
| `domain/parameter` | `PARAMETER_*` | 参数名称、封闭值类型、约束和绑定。 |

`domain/heal` 不直接拥有错误码家族。它的失败只经 `domain/node` 的 `classifyNodeFault` 到达公开边界；已有 code 原样透传，其他失败映射到 `EXECUTION_*`。

## 维护与验证

新增或修改 code 时必须同时更新注册表、生产构造点和外部包契约测试。以下守卫在 `go test ./...` 中执行：

- [`architecture/fault_contract_guard_test.go`](../../architecture/fault_contract_guard_test.go)：注册表与 `fault.Code` 常量、Kind、message、前缀所有权和无哨兵 error 约束。
- [`contract/fault_public_api_test.go`](../../contract/fault_public_api_test.go)：外部包可见的错误面。
- 各上下文 `fault_codes.go` 和对应 TEST_CASES 矩阵：构造点、参数安全和错误链语义。

错误码只能新增或墓碑化，不能改名、复用或改变既有含义。持久化映射和数据库迁移属于宿主；Core 只提供当前 `Kind + Code` 契约。
