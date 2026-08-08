# domain/fingerprint 测试用例矩阵

## 范围与口径

本表记录 `domain/fingerprint` 的公开业务入口和全部顶层 Go testcase。Go 测试源码是唯一可执行事实；表驱动测试的全部子案例由其对应的测试函数统一引用。

## 公开 API 与领域入口

| 公开入口 | 定义文件 | 测试证据状态 |
|---|---|---|
| `DetectFrameworks` | [`domain/fingerprint/detection.go`](../../domain/fingerprint/detection.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Selector.Validate` | [`domain/fingerprint/fingerprint.go`](../../domain/fingerprint/fingerprint.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Fingerprint.Validate` | [`domain/fingerprint/fingerprint.go`](../../domain/fingerprint/fingerprint.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `NodeSpec.Validate` | [`domain/fingerprint/fingerprint.go`](../../domain/fingerprint/fingerprint.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `FrameworkInfo.Validate` | [`domain/fingerprint/framework.go`](../../domain/fingerprint/framework.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `FrameworkStack.Validate` | [`domain/fingerprint/framework.go`](../../domain/fingerprint/framework.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `FrameworkStack.Clone` | [`domain/fingerprint/framework.go`](../../domain/fingerprint/framework.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `SortFrameworkStack` | [`domain/fingerprint/framework.go`](../../domain/fingerprint/framework.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |

## 测试用例证据矩阵

| Test case | 输入、边界或业务前置状态 | 预期契约 | 可执行证据 |
|---|---|---|---|
| `TestSelectorValidate` | `Selector Validate`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/fingerprint_test.go`](../../domain/fingerprint/fingerprint_test.go) · `TestSelectorValidate` |
| `TestSelectorValidateBusinessMatrix` | `Selector Validate Business Matrix`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/fingerprint_test.go`](../../domain/fingerprint/fingerprint_test.go) · `TestSelectorValidateBusinessMatrix` |
| `TestNodeSpecValidateInvariantMatrix` | `Node Spec Validate Invariant Matrix`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/fingerprint_test.go`](../../domain/fingerprint/fingerprint_test.go) · `TestNodeSpecValidateInvariantMatrix` |
| `TestNodeSpecValidate` | `Node Spec Validate`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/fingerprint_test.go`](../../domain/fingerprint/fingerprint_test.go) · `TestNodeSpecValidate` |
| `TestNodeSpecValidateOptionalUUID` | `Node Spec Validate Optional UUID`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/fingerprint_test.go`](../../domain/fingerprint/fingerprint_test.go) · `TestNodeSpecValidateOptionalUUID` |
| `TestDetectFrameworksNormalizesAndOrdersMatches` | `Detect Frameworks Normalizes And Orders Matches`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/framework_test.go`](../../domain/fingerprint/framework_test.go) · `TestDetectFrameworksNormalizesAndOrdersMatches` |
| `TestSelectorValidateStrictBoundaries` | `Selector Validate Strict Boundaries`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/strict_boundary_test.go`](../../domain/fingerprint/strict_boundary_test.go) · `TestSelectorValidateStrictBoundaries` |
| `TestFingerprintValidateStrictBoundaries` | `Fingerprint Validate Strict Boundaries`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/strict_boundary_test.go`](../../domain/fingerprint/strict_boundary_test.go) · `TestFingerprintValidateStrictBoundaries` |
| `TestNodeSpecValidateAggregatesAllFailuresWithoutMutation` | `Node Spec Validate Aggregates All Failures Without Mutation`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/strict_boundary_test.go`](../../domain/fingerprint/strict_boundary_test.go) · `TestNodeSpecValidateAggregatesAllFailuresWithoutMutation` |
| `TestNodeSpecValidateUUIDAndCollectionBoundaries` | `Node Spec Validate UUIDAnd Collection Boundaries`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/strict_boundary_test.go`](../../domain/fingerprint/strict_boundary_test.go) · `TestNodeSpecValidateUUIDAndCollectionBoundaries` |
| `TestFrameworkInfoValidateEnumAndVersionMatrices` | `Framework Info Validate Enum And Version Matrices`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/strict_boundary_test.go`](../../domain/fingerprint/strict_boundary_test.go) · `TestFrameworkInfoValidateEnumAndVersionMatrices` |
| `TestFrameworkStackValidateAndCloneBoundaries` | `Framework Stack Validate And Clone Boundaries`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/strict_boundary_test.go`](../../domain/fingerprint/strict_boundary_test.go) · `TestFrameworkStackValidateAndCloneBoundaries` |
| `TestFrameworkInfoValidateConfidenceExtremes` | `Framework Info Validate Confidence Extremes`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/strict_boundary_test.go`](../../domain/fingerprint/strict_boundary_test.go) · `TestFrameworkInfoValidateConfidenceExtremes` |
| `TestSortFrameworkStackIsDeterministicAndImmutable` | `Sort Framework Stack Is Deterministic And Immutable`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/strict_boundary_test.go`](../../domain/fingerprint/strict_boundary_test.go) · `TestSortFrameworkStackIsDeterministicAndImmutable` |
| `TestDetectFrameworksRejectsNilDetectors` | `Detect Frameworks Rejects Nil Detectors`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/strict_boundary_test.go`](../../domain/fingerprint/strict_boundary_test.go) · `TestDetectFrameworksRejectsNilDetectors` |
| `TestSortFrameworkStackOrderingAndOwnershipMatrix` | `Sort Framework Stack Ordering And Ownership Matrix`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/strict_boundary_test.go`](../../domain/fingerprint/strict_boundary_test.go) · `TestSortFrameworkStackOrderingAndOwnershipMatrix` |
| `TestDetectFrameworksForwardsContextObservationAndStopsOnError` | `Detect Frameworks Forwards Context Observation And Stops On Error`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/strict_boundary_test.go`](../../domain/fingerprint/strict_boundary_test.go) · `TestDetectFrameworksForwardsContextObservationAndStopsOnError` |
| `TestDetectFrameworksEmptyInvalidAndOrderedResults` | `Detect Frameworks Empty Invalid And Ordered Results`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/strict_boundary_test.go`](../../domain/fingerprint/strict_boundary_test.go) · `TestDetectFrameworksEmptyInvalidAndOrderedResults` |
| `TestDetectFrameworksMergesDuplicatesWithoutMutatingMatches` | `Detect Frameworks Merges Duplicates Without Mutating Matches`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/strict_boundary_test.go`](../../domain/fingerprint/strict_boundary_test.go) · `TestDetectFrameworksMergesDuplicatesWithoutMutatingMatches` |
| `TestFrameworkInfoValidateBusinessBoundaries` | `Framework Info Validate Business Boundaries`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/public_methods_matrix_test.go`](../../domain/fingerprint/public_methods_matrix_test.go) · `TestFrameworkInfoValidateBusinessBoundaries` |
| `TestFrameworkStackValidateRejectsInvalidAndDuplicateKinds` | `Framework Stack Validate Rejects Invalid And Duplicate Kinds`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/public_methods_matrix_test.go`](../../domain/fingerprint/public_methods_matrix_test.go) · `TestFrameworkStackValidateRejectsInvalidAndDuplicateKinds` |
| `TestSortFrameworkStackUsesTotalOrderWithoutAliasingInput` | `Sort Framework Stack Uses Total Order Without Aliasing Input`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/public_methods_matrix_test.go`](../../domain/fingerprint/public_methods_matrix_test.go) · `TestSortFrameworkStackUsesTotalOrderWithoutAliasingInput` |
| `TestDetectFrameworksMergesDuplicatesAndPreservesDetectorContract` | `Detect Frameworks Merges Duplicates And Preserves Detector Contract`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/public_methods_matrix_test.go`](../../domain/fingerprint/public_methods_matrix_test.go) · `TestDetectFrameworksMergesDuplicatesAndPreservesDetectorContract` |
| `TestDetectFrameworksPropagatesFailuresAndRejectsInvalidMatches` | `Detect Frameworks Propagates Failures And Rejects Invalid Matches`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/fingerprint/public_methods_matrix_test.go`](../../domain/fingerprint/public_methods_matrix_test.go) · `TestDetectFrameworksPropagatesFailuresAndRejectsInvalidMatches` |

## 跨入口与一致性用例

同包及其子目录中名称含 `Conformance`、`Transaction`、`Race`、`Rollback`、`Replay`、`Concurrent` 或 `Fence` 的测试，属于跨入口契约；它们已在上方矩阵逐行列出。application 包的 `conformancetest/` 证据也归属此表。

## 维护规则

1. 新增或删除 `Test…` 函数时，必须同步更新本表；表驱动新增子案例要更新相应行的边界描述。
2. 新增公开 domain API 或 application use case 时，必须先添加公开入口清单行和至少一条可执行测试证据。
3. 文档不替代测试；冲突时以 Go 测试断言和领域契约为准。
