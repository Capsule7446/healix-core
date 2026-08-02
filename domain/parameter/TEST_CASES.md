# domain/parameter Test Case Matrix

## 范围与口径

本表记录 `domain/parameter` 的公开业务入口和全部顶层 Go testcase。Go 测试源码是唯一可执行事实；表驱动测试的全部子案例由其对应的测试函数统一引用。

## Public API / Use-case Inventory

| 公开入口 | 定义文件 | 测试证据状态 |
|---|---|---|
| `Constraint.Validate` | [`domain/parameter/binding.go`](../../domain/parameter/binding.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `LiteralBinding` | [`domain/parameter/binding.go`](../../domain/parameter/binding.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `ParentReferenceBinding` | [`domain/parameter/binding.go`](../../domain/parameter/binding.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Binding.Kind` | [`domain/parameter/binding.go`](../../domain/parameter/binding.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Binding.Literal` | [`domain/parameter/binding.go`](../../domain/parameter/binding.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Binding.ParentName` | [`domain/parameter/binding.go`](../../domain/parameter/binding.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Binding.Clone` | [`domain/parameter/binding.go`](../../domain/parameter/binding.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Binding.Resolve` | [`domain/parameter/binding.go`](../../domain/parameter/binding.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `TextValue` | [`domain/parameter/value.go`](../../domain/parameter/value.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `BooleanValue` | [`domain/parameter/value.go`](../../domain/parameter/value.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `SingleSelectValue` | [`domain/parameter/value.go`](../../domain/parameter/value.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `MultiSelectValue` | [`domain/parameter/value.go`](../../domain/parameter/value.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `NewNumberValue` | [`domain/parameter/value.go`](../../domain/parameter/value.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `PresentValue` | [`domain/parameter/value.go`](../../domain/parameter/value.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `OptionalValue.IsPresent` | [`domain/parameter/value.go`](../../domain/parameter/value.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `OptionalValue.Value` | [`domain/parameter/value.go`](../../domain/parameter/value.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Value.Type` | [`domain/parameter/value.go`](../../domain/parameter/value.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Value.Text` | [`domain/parameter/value.go`](../../domain/parameter/value.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Value.Number` | [`domain/parameter/value.go`](../../domain/parameter/value.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Value.Boolean` | [`domain/parameter/value.go`](../../domain/parameter/value.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Value.SingleSelect` | [`domain/parameter/value.go`](../../domain/parameter/value.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Value.MultiSelect` | [`domain/parameter/value.go`](../../domain/parameter/value.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Value.MultiSelectMetrics` | [`domain/parameter/value.go`](../../domain/parameter/value.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Value.Clone` | [`domain/parameter/value.go`](../../domain/parameter/value.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Value.Equal` | [`domain/parameter/value.go`](../../domain/parameter/value.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Value.Validate` | [`domain/parameter/value.go`](../../domain/parameter/value.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |

## Test Case Evidence Matrix

| Test case | 输入、边界或业务前置状态 | 预期契约 | 可执行证据 |
|---|---|---|---|
| `TestLiteralBindingPreservesTypedValueAndClonesCollections` | `Literal Binding Preserves Typed Value And Clones Collections`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/parameter/binding_test.go`](../../domain/parameter/binding_test.go) · `TestLiteralBindingPreservesTypedValueAndClonesCollections` |
| `TestParentReferenceBindingResolvesOnlyNamedTypedParent` | `Parent Reference Binding Resolves Only Named Typed Parent`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/parameter/binding_test.go`](../../domain/parameter/binding_test.go) · `TestParentReferenceBindingResolvesOnlyNamedTypedParent` |
| `TestBindingRejectsInvalidVariantsAndMissingParents` | `Binding Rejects Invalid Variants And Missing Parents`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/parameter/binding_test.go`](../../domain/parameter/binding_test.go) · `TestBindingRejectsInvalidVariantsAndMissingParents` |
| `TestConstraintValidatesExactTypeAndSelectMembership` | `Constraint Validates Exact Type And Select Membership`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/parameter/binding_test.go`](../../domain/parameter/binding_test.go) · `TestConstraintValidatesExactTypeAndSelectMembership` |
| `TestValueStrictBoundariesAndImmutability` | `Value Strict Boundaries And Immutability`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/parameter/strict_boundary_test.go`](../../domain/parameter/strict_boundary_test.go) · `TestValueStrictBoundariesAndImmutability` |
| `TestConstraintStrictTypesOptionsAndBinding` | `Constraint Strict Types Options And Binding`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/parameter/strict_boundary_test.go`](../../domain/parameter/strict_boundary_test.go) · `TestConstraintStrictTypesOptionsAndBinding` |
| `TestMultiSelectMetricsInspectWithoutExposingPayload` | `Multi Select Metrics Inspect Without Exposing Payload`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/parameter/value_test.go`](../../domain/parameter/value_test.go) · `TestMultiSelectMetricsInspectWithoutExposingPayload` |
| `TestValueAccessorsPreserveEachPublicType` | `Value Accessors Preserve Each Public Type`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/parameter/value_test.go`](../../domain/parameter/value_test.go) · `TestValueAccessorsPreserveEachPublicType` |
| `TestOptionalValueDistinguishesAbsentFromPresentZeroValues` | `Optional Value Distinguishes Absent From Present Zero Values`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/parameter/value_test.go`](../../domain/parameter/value_test.go) · `TestOptionalValueDistinguishesAbsentFromPresentZeroValues` |
| `TestValueValidateAcceptsEveryKindAndRejectsInvalidValues` | `Value Validate Accepts Every Kind And Rejects Invalid Values`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/parameter/value_test.go`](../../domain/parameter/value_test.go) · `TestValueValidateAcceptsEveryKindAndRejectsInvalidValues` |
| `TestValueEqualDetectsSemanticDifferences` | `Value Equal Detects Semantic Differences`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/parameter/value_test.go`](../../domain/parameter/value_test.go) · `TestValueEqualDetectsSemanticDifferences` |
| `TestNumberCanonicalization` | `Number Canonicalization`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/parameter/value_test.go`](../../domain/parameter/value_test.go) · `TestNumberCanonicalization` |
| `TestNumberRejectsOversizedInputBeforeCanonicalization` | `Number Rejects Oversized Input Before Canonicalization`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/parameter/value_test.go`](../../domain/parameter/value_test.go) · `TestNumberRejectsOversizedInputBeforeCanonicalization` |
| `TestValueAcceptsStringsAtExactLimit` | `Value Accepts Strings At Exact Limit`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/parameter/value_test.go`](../../domain/parameter/value_test.go) · `TestValueAcceptsStringsAtExactLimit` |
| `TestValueRejectsOversizedStrings` | `Value Rejects Oversized Strings`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/parameter/value_test.go`](../../domain/parameter/value_test.go) · `TestValueRejectsOversizedStrings` |
| `TestClosedValuesCloneAndValidate` | `Closed Values Clone And Validate`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/parameter/value_test.go`](../../domain/parameter/value_test.go) · `TestClosedValuesCloneAndValidate` |

| `TestNewNumberValueRejectsWithoutEchoingInput` | 导出构造器收到非法数字输入（含哨兵值）。 | 返回 PARAMETER_VALUE_INVALID；公共文本与私有 cause 均不含被拒输入。 | [`domain/parameter/fault_contract_test.go`](../../domain/parameter/fault_contract_test.go) · `TestNewNumberValueRejectsWithoutEchoingInput` |
| `TestNewNumberValueRejectsOversizedInputWithSameCode` | 超出 MaxValueStringBytes 的数字输入。 | 与其他值非法情形共用同一 code，不单独设 OUT_OF_RANGE。 | [`domain/parameter/fault_contract_test.go`](../../domain/parameter/fault_contract_test.go) · `TestNewNumberValueRejectsOversizedInputWithSameCode` |
| `TestValidateNameRejectsWithoutEchoingName` | 空白、控制字符、格式字符、非法 UTF-8、超字节上限五类名称。 | 一律返回 PARAMETER_NAME_INVALID；被拒名称不进公共文本。 | [`domain/parameter/fault_contract_test.go`](../../domain/parameter/fault_contract_test.go) · `TestValidateNameRejectsWithoutEchoingName` |
| `TestValueValidateRejectsUnsupportedTypeWithoutEchoingIt` | 闭集之外的值类型。 | 返回 PARAMETER_VALUE_INVALID；类型取值不回显。 | [`domain/parameter/fault_contract_test.go`](../../domain/parameter/fault_contract_test.go) · `TestValueValidateRejectsUnsupportedTypeWithoutEchoingIt` |
| `TestConstraintValidateRejectsWithoutEchoingTypesOrOptions` | 选项不匹配与类型不匹配两类约束失败。 | 返回 PARAMETER_CONSTRAINT_UNSATISFIED；约束类型、值类型、选项值均不回显。 | [`domain/parameter/fault_contract_test.go`](../../domain/parameter/fault_contract_test.go) · `TestConstraintValidateRejectsWithoutEchoingTypesOrOptions` |
| `TestConstraintValidatePropagatesValueCodeUnchanged` | 值自身非法时经由约束校验返回。 | 保持 PARAMETER_VALUE_INVALID，不被改标为约束失败。 | [`domain/parameter/fault_contract_test.go`](../../domain/parameter/fault_contract_test.go) · `TestConstraintValidatePropagatesValueCodeUnchanged` |
| `TestNewNumberValueNeverReturnsAValueItsOwnValidatorRejects` | `New Number Value Never Returns AValue Its Own Validator Rejects`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/parameter/fault_contract_test.go`](../../domain/parameter/fault_contract_test.go) · `TestNewNumberValueNeverReturnsAValueItsOwnValidatorRejects` |
| `TestValidateNameContract` | `Validate Name Contract`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/parameter/name_test.go`](../../domain/parameter/name_test.go) · `TestValidateNameContract` |
| `TestLiteralBindingRejectsInvalidClosedValueBeforeResolution` | `Literal Binding Rejects Invalid Closed Value Before Resolution`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/parameter/public_methods_matrix_test.go`](../../domain/parameter/public_methods_matrix_test.go) · `TestLiteralBindingRejectsInvalidClosedValueBeforeResolution` |

## Cross-cutting / Conformance Cases

同包及其子目录中名称含 `Conformance`、`Transaction`、`Race`、`Rollback`、`Replay`、`Concurrent` 或 `Fence` 的测试，属于跨入口契约；它们已在上方矩阵逐行列出。application 包的 `conformancetest/` 证据也归属此表。

## 维护规则

1. 新增或删除 `Test…` 函数时，必须同步更新本表；表驱动新增子案例要更新相应行的边界描述。
2. 新增公开 domain API 或 application use case 时，必须先添加公开入口清单行和至少一条可执行测试证据。
3. 文档不替代测试；冲突时以 Go 测试断言和领域契约为准。
