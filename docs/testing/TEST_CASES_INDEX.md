# 测试用例矩阵索引

本索引汇总每个拥有生产代码的一级 domain/application 包的 testcase 矩阵。每个矩阵同时记录公开入口清单和全部顶层 Go `Test…` 函数的可执行证据。表驱动测试的子案例在对应函数源码中维护。

## Domain

| Package | Matrix | Coverage focus |
|---|---|---|
| `domain/automation` | [`TEST_CASES.md`](../../domain/automation/TEST_CASES.md) | 值对象、验证、状态机、算法、生命周期、所有权与边界。 |
| `domain/evidence` | [`TEST_CASES.md`](../../domain/evidence/TEST_CASES.md) | 值对象、验证、状态机、算法、生命周期、所有权与边界。 |
| `domain/execution` | [`TEST_CASES.md`](../../domain/execution/TEST_CASES.md) | 值对象、验证、状态机、算法、生命周期、所有权与边界。 |
| `domain/fault` | [`TEST_CASES.md`](../../domain/fault/TEST_CASES.md) | 错误内核：安全公开文本、`Kind`/`Code` 分类穿透包装与 `errors.Join`、params/violations 所有权、`MaxViolations` 截断、封闭的违规原因词表。 |
| `domain/fingerprint` | [`TEST_CASES.md`](../../domain/fingerprint/TEST_CASES.md) | 值对象、验证、状态机、算法、生命周期、所有权与边界。 |
| `domain/heal` | [`TEST_CASES.md`](../../domain/heal/TEST_CASES.md) | 值对象、验证、状态机、算法、生命周期、所有权与边界。 |
| `domain/interpolation` | [`TEST_CASES.md`](../../domain/interpolation/TEST_CASES.md) | 值对象、验证、状态机、算法、生命周期、所有权与边界。 |
| `domain/node` | [`TEST_CASES.md`](../../domain/node/TEST_CASES.md) | 值对象、验证、状态机、算法、生命周期、所有权与边界。 |
| `domain/parameter` | [`TEST_CASES.md`](../../domain/parameter/TEST_CASES.md) | 值对象、验证、状态机、算法、生命周期、所有权与边界。 |
| `domain/sampling` | [`TEST_CASES.md`](../../domain/sampling/TEST_CASES.md) | 值对象、验证、状态机、算法、生命周期、所有权与边界。 |
| `domain/weburl` | [`TEST_CASES.md`](../../domain/weburl/TEST_CASES.md) | 共享内核：绝对 HTTP(S) URL 规则的唯一判定处——scheme、host、userinfo、控制字符，以及封闭的拒绝原因词表。 |

## Application

| Package | Matrix | Coverage focus |
|---|---|---|
| `application/automation` | [`TEST_CASES.md`](../../application/automation/TEST_CASES.md) | Use case、端口错误、事务、回滚、幂等、并发和跨层契约。 |
| `application/engine` | [`TEST_CASES.md`](../../application/engine/TEST_CASES.md) | Use case、端口错误、事务、回滚、幂等、并发和跨层契约。 |
| `application/execution` | [`TEST_CASES.md`](../../application/execution/TEST_CASES.md) | Use case、端口错误、事务、回滚、幂等、并发和跨层契约。 |
| `application/scheduling` | [`TEST_CASES.md`](../../application/scheduling/TEST_CASES.md) | Use case、端口错误、事务、回滚、幂等、并发和跨层契约。 |

## 使用与维护

- 从对应包的 `TEST_CASES.md` 查找某个公开入口或 testcase 的证据；链接定位到可执行 Go 测试。
- 新增测试时先更新代码断言，再在对应矩阵新增或扩展表格行。
- `application/*/conformancetest` 不单独拥有矩阵；其案例归属上层 application 包。
