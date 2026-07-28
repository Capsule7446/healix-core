# domain/interpolation Test Case Matrix

## 范围与口径

本表记录 `domain/interpolation` 的公开业务入口和全部顶层 Go testcase。Go 测试源码是唯一可执行事实；表驱动测试的全部子案例由其对应的测试函数统一引用。

## Public API / Use-case Inventory

| 公开入口 | 定义文件 | 测试证据状态 |
|---|---|---|
| `Expand` | [`domain/interpolation/variables.go`](../../domain/interpolation/variables.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Names` | [`domain/interpolation/variables.go`](../../domain/interpolation/variables.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |

## Test Case Evidence Matrix

| Test case | 输入、边界或业务前置状态 | 预期契约 | 可执行证据 |
|---|---|---|---|
| `TestExpandStrictInputs` | `Expand Strict Inputs`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/interpolation/strict_boundary_test.go`](../../domain/interpolation/strict_boundary_test.go) · `TestExpandStrictInputs` |
| `TestNamesStrictSyntaxAndDuplicates` | `Names Strict Syntax And Duplicates`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/interpolation/strict_boundary_test.go`](../../domain/interpolation/strict_boundary_test.go) · `TestNamesStrictSyntaxAndDuplicates` |
| `TestNamesAndExpandExpressionMatrix` | `Names And Expand Expression Matrix`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/interpolation/variables_test.go`](../../domain/interpolation/variables_test.go) · `TestNamesAndExpandExpressionMatrix` |
| `TestExpandRejectsNilResolverOnlyWhenNeeded` | `Expand Rejects Nil Resolver Only When Needed`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/interpolation/variables_test.go`](../../domain/interpolation/variables_test.go) · `TestExpandRejectsNilResolverOnlyWhenNeeded` |
| `TestNamesAndExpandShareOneCaseSensitiveGrammar` | `Names And Expand Share One Case Sensitive Grammar`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/interpolation/variables_test.go`](../../domain/interpolation/variables_test.go) · `TestNamesAndExpandShareOneCaseSensitiveGrammar` |

## Cross-cutting / Conformance Cases

同包及其子目录中名称含 `Conformance`、`Transaction`、`Race`、`Rollback`、`Replay`、`Concurrent` 或 `Fence` 的测试，属于跨入口契约；它们已在上方矩阵逐行列出。application 包的 `conformancetest/` 证据也归属此表。

## 维护规则

1. 新增或删除 `Test…` 函数时，必须同步更新本表；表驱动新增子案例要更新相应行的边界描述。
2. 新增公开 domain API 或 application use case 时，必须先添加公开入口清单行和至少一条可执行测试证据。
3. 文档不替代测试；冲突时以 Go 测试断言和领域契约为准。
