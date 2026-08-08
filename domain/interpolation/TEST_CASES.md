# domain/interpolation 测试用例矩阵

## 范围与口径

本表记录 `domain/interpolation` 的公开业务入口和全部顶层 Go testcase。Go 测试源码是唯一可执行事实；表驱动测试的全部子案例由其对应的测试函数统一引用。

## 公开 API 与领域入口

| 公开入口 | 定义文件 | 测试证据状态 |
|---|---|---|
| `Expand` | [`domain/interpolation/variables.go`](../../domain/interpolation/variables.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Names` | [`domain/interpolation/variables.go`](../../domain/interpolation/variables.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |

## 测试用例证据矩阵

| Test case | 输入、边界或业务前置状态 | 预期契约 | 可执行证据 |
|---|---|---|---|
| `TestExpandStrictInputs` | `Expand Strict Inputs`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/interpolation/strict_boundary_test.go`](../../domain/interpolation/strict_boundary_test.go) · `TestExpandStrictInputs` |
| `TestNamesStrictSyntaxAndDuplicates` | `Names Strict Syntax And Duplicates`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/interpolation/strict_boundary_test.go`](../../domain/interpolation/strict_boundary_test.go) · `TestNamesStrictSyntaxAndDuplicates` |
| `TestNamesAndExpandExpressionMatrix` | `Names And Expand Expression Matrix`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/interpolation/variables_test.go`](../../domain/interpolation/variables_test.go) · `TestNamesAndExpandExpressionMatrix` |
| `TestExpandRejectsNilResolverOnlyWhenNeeded` | `Expand Rejects Nil Resolver Only When Needed`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/interpolation/variables_test.go`](../../domain/interpolation/variables_test.go) · `TestExpandRejectsNilResolverOnlyWhenNeeded` |
| `TestNamesAndExpandShareOneCaseSensitiveGrammar` | `Names And Expand Share One Case Sensitive Grammar`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/interpolation/variables_test.go`](../../domain/interpolation/variables_test.go) · `TestNamesAndExpandShareOneCaseSensitiveGrammar` |
| `TestExpandRefusesTheAmplificationTemplate` | 64 KiB 的 `${x}` 重复模板配合 64 KiB 变量值——两侧单独看都不异常，乘积恰好 1 GiB。 | 以 `INTERPOLATION_EXPANSION_TOO_LARGE` / `RESOURCE_EXHAUSTED` 拒绝，返回空串。 | [`domain/interpolation/expansion_budget_test.go`](../../domain/interpolation/expansion_budget_test.go) · `TestExpandRefusesTheAmplificationTemplate` |
| `TestExpandRefusalNeverMaterializesTheOutput` | 同一放大模板，测量拒绝路径的堆分配。 | 分配不超过 8×`MaxExpansionBytes`——先构造出 1 GiB 再拒绝，等于用尽了它本要保护的内存。 | [`domain/interpolation/expansion_budget_test.go`](../../domain/interpolation/expansion_budget_test.go) · `TestExpandRefusalNeverMaterializesTheOutput` |
| `TestExpandOutputBudgetBoundary` | 输出恰好 `MaxExpansionBytes-1`、`MaxExpansionBytes`、`MaxExpansionBytes+1` 字节。 | 前两者接受且长度精确，第三者拒绝；预算含上界。 | [`domain/interpolation/expansion_budget_test.go`](../../domain/interpolation/expansion_budget_test.go) · `TestExpandOutputBudgetBoundary` |
| `TestExpandBudgetCountsLiteralsAndValuesTogether` | 把体积压在模板字面量而非解析值里的模板。 | 同样拒绝——预算算的是输出总量，不能靠换个位置放字节规避。 | [`domain/interpolation/expansion_budget_test.go`](../../domain/interpolation/expansion_budget_test.go) · `TestExpandBudgetCountsLiteralsAndValuesTogether` |
| `TestExpandBudgetSurvivesRemainderOverflow` | 4096 个 `${x}` × 4096 字节值。 | 拒绝且不泄露部分输出；`written+len(chunk) > limit` 写法会溢出成负数并放行，剩余额度写法不会。 | [`domain/interpolation/expansion_budget_test.go`](../../domain/interpolation/expansion_budget_test.go) · `TestExpandBudgetSurvivesRemainderOverflow` |

## 跨入口与一致性用例

同包及其子目录中名称含 `Conformance`、`Transaction`、`Race`、`Rollback`、`Replay`、`Concurrent` 或 `Fence` 的测试，属于跨入口契约；它们已在上方矩阵逐行列出。application 包的 `conformancetest/` 证据也归属此表。

## 维护规则

1. 新增或删除 `Test…` 函数时，必须同步更新本表；表驱动新增子案例要更新相应行的边界描述。
2. 新增公开 domain API 或 application use case 时，必须先添加公开入口清单行和至少一条可执行测试证据。
3. 文档不替代测试；冲突时以 Go 测试断言和领域契约为准。
