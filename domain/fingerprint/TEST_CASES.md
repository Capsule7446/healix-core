# domain/fingerprint 测试用例矩阵

## 范围与口径

本表记录 `domain/fingerprint` 的公开业务入口和全部顶层 Go testcase。Go 测试源码是唯一可执行事实；表驱动测试的全部子案例由其对应的测试函数统一引用。

## 公开 API 与领域入口

| 公开入口 | 定义文件 | 测试证据状态 |
|---|---|---|
| `DetectFrameworks` | [`domain/fingerprint/detection.go`](../../domain/fingerprint/detection.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Selector.Validate` | [`domain/fingerprint/fingerprint.go`](../../domain/fingerprint/fingerprint.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Fingerprint.Validate` | [`domain/fingerprint/fingerprint.go`](../../domain/fingerprint/fingerprint.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `ElementTargetSpec.Validate` | [`domain/fingerprint/fingerprint.go`](../../domain/fingerprint/fingerprint.go) | 校验元素目标的选择器、指纹描述和可选身份。 |
| `FrameworkInfo.Validate` | [`domain/fingerprint/framework.go`](../../domain/fingerprint/framework.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `FrameworkStack.Validate` | [`domain/fingerprint/framework.go`](../../domain/fingerprint/framework.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `FrameworkStack.Clone` | [`domain/fingerprint/framework.go`](../../domain/fingerprint/framework.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `SortFrameworkStack` | [`domain/fingerprint/framework.go`](../../domain/fingerprint/framework.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |

## 测试用例证据矩阵

| 测试用例 | 输入、边界或业务前置状态 | 预期契约 | 可执行证据 |
|---|---|---|---|
| `TestSelectorValidate` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/fingerprint_test.go`](../../domain/fingerprint/fingerprint_test.go) · `TestSelectorValidate` |
| `TestSelectorValidateBusinessMatrix` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/fingerprint_test.go`](../../domain/fingerprint/fingerprint_test.go) · `TestSelectorValidateBusinessMatrix` |
| `TestNodeSpecValidateInvariantMatrix` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/fingerprint_test.go`](../../domain/fingerprint/fingerprint_test.go) · `TestNodeSpecValidateInvariantMatrix` |
| `TestNodeSpecValidate` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/fingerprint_test.go`](../../domain/fingerprint/fingerprint_test.go) · `TestNodeSpecValidate` |
| `TestNodeSpecValidateOptionalUUID` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/fingerprint_test.go`](../../domain/fingerprint/fingerprint_test.go) · `TestNodeSpecValidateOptionalUUID` |
| `TestDetectFrameworksNormalizesAndOrdersMatches` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/framework_test.go`](../../domain/fingerprint/framework_test.go) · `TestDetectFrameworksNormalizesAndOrdersMatches` |
| `TestSelectorValidateStrictBoundaries` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/strict_boundary_test.go`](../../domain/fingerprint/strict_boundary_test.go) · `TestSelectorValidateStrictBoundaries` |
| `TestFingerprintValidateStrictBoundaries` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/strict_boundary_test.go`](../../domain/fingerprint/strict_boundary_test.go) · `TestFingerprintValidateStrictBoundaries` |
| `TestNodeSpecValidateAggregatesAllFailuresWithoutMutation` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/strict_boundary_test.go`](../../domain/fingerprint/strict_boundary_test.go) · `TestNodeSpecValidateAggregatesAllFailuresWithoutMutation` |
| `TestNodeSpecValidateUUIDAndCollectionBoundaries` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/strict_boundary_test.go`](../../domain/fingerprint/strict_boundary_test.go) · `TestNodeSpecValidateUUIDAndCollectionBoundaries` |
| `TestFrameworkInfoValidateEnumAndVersionMatrices` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/strict_boundary_test.go`](../../domain/fingerprint/strict_boundary_test.go) · `TestFrameworkInfoValidateEnumAndVersionMatrices` |
| `TestFrameworkStackValidateAndCloneBoundaries` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/strict_boundary_test.go`](../../domain/fingerprint/strict_boundary_test.go) · `TestFrameworkStackValidateAndCloneBoundaries` |
| `TestFrameworkInfoValidateConfidenceExtremes` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/strict_boundary_test.go`](../../domain/fingerprint/strict_boundary_test.go) · `TestFrameworkInfoValidateConfidenceExtremes` |
| `TestSortFrameworkStackIsDeterministicAndImmutable` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/strict_boundary_test.go`](../../domain/fingerprint/strict_boundary_test.go) · `TestSortFrameworkStackIsDeterministicAndImmutable` |
| `TestDetectFrameworksRejectsNilDetectors` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/strict_boundary_test.go`](../../domain/fingerprint/strict_boundary_test.go) · `TestDetectFrameworksRejectsNilDetectors` |
| `TestSortFrameworkStackOrderingAndOwnershipMatrix` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/strict_boundary_test.go`](../../domain/fingerprint/strict_boundary_test.go) · `TestSortFrameworkStackOrderingAndOwnershipMatrix` |
| `TestDetectFrameworksForwardsContextObservationAndStopsOnError` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/strict_boundary_test.go`](../../domain/fingerprint/strict_boundary_test.go) · `TestDetectFrameworksForwardsContextObservationAndStopsOnError` |
| `TestDetectFrameworksEmptyInvalidAndOrderedResults` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/strict_boundary_test.go`](../../domain/fingerprint/strict_boundary_test.go) · `TestDetectFrameworksEmptyInvalidAndOrderedResults` |
| `TestDetectFrameworksMergesDuplicatesWithoutMutatingMatches` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/strict_boundary_test.go`](../../domain/fingerprint/strict_boundary_test.go) · `TestDetectFrameworksMergesDuplicatesWithoutMutatingMatches` |
| `TestFrameworkInfoValidateBusinessBoundaries` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/public_methods_matrix_test.go`](../../domain/fingerprint/public_methods_matrix_test.go) · `TestFrameworkInfoValidateBusinessBoundaries` |
| `TestFrameworkStackValidateRejectsInvalidAndDuplicateKinds` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/public_methods_matrix_test.go`](../../domain/fingerprint/public_methods_matrix_test.go) · `TestFrameworkStackValidateRejectsInvalidAndDuplicateKinds` |
| `TestSortFrameworkStackUsesTotalOrderWithoutAliasingInput` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/public_methods_matrix_test.go`](../../domain/fingerprint/public_methods_matrix_test.go) · `TestSortFrameworkStackUsesTotalOrderWithoutAliasingInput` |
| `TestDetectFrameworksMergesDuplicatesAndPreservesDetectorContract` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/public_methods_matrix_test.go`](../../domain/fingerprint/public_methods_matrix_test.go) · `TestDetectFrameworksMergesDuplicatesAndPreservesDetectorContract` |
| `TestDetectFrameworksPropagatesFailuresAndRejectsInvalidMatches` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/public_methods_matrix_test.go`](../../domain/fingerprint/public_methods_matrix_test.go) · `TestDetectFrameworksPropagatesFailuresAndRejectsInvalidMatches` |

## 跨入口与一致性用例

同包及其子目录中名称含 `Conformance`、`Transaction`、`Race`、`Rollback`、`Replay`、`Concurrent` 或 `Fence` 的测试，属于跨入口契约；它们已在上方矩阵逐行列出。application 包的 `conformancetest/` 证据也归属此表。

## 维护规则

1. 新增或删除 `Test…` 函数时，必须同步更新本表；表驱动新增子案例要更新相应行的边界描述。
2. 新增公开 domain API 或 application use case 时，必须先添加公开入口清单行和至少一条可执行测试证据。
3. 文档不替代测试；冲突时以 Go 测试断言和领域契约为准。
