# domain/evidence 测试用例矩阵

## 范围与口径

本表记录 `domain/evidence` 的公开业务入口和全部顶层 Go testcase。Go 测试源码是唯一可执行事实；表驱动测试的全部子案例由其对应的测试函数统一引用。

## 公开 API 与领域入口

| 公开入口 | 定义文件 | 测试证据状态 |
|---|---|---|
| `StepTransitionCommit.Validate` | [`domain/evidence/commits.go`](../../domain/evidence/commits.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `StepProgressEvent.Validate` | [`domain/evidence/events.go`](../../domain/evidence/events.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Phase.IsTerminal` | [`domain/evidence/facts.go`](../../domain/evidence/facts.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `StepFact.Validate` | [`domain/evidence/facts.go`](../../domain/evidence/facts.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `ValidateDecisionBand` | [`domain/evidence/observations.go`](../../domain/evidence/observations.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `ValidateConfidence` | [`domain/evidence/observations.go`](../../domain/evidence/observations.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `HealObservation.Validate` | [`domain/evidence/observations.go`](../../domain/evidence/observations.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `AbsentValidationValue` | [`domain/evidence/observations.go`](../../domain/evidence/observations.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `ScalarValidationValue` | [`domain/evidence/observations.go`](../../domain/evidence/observations.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `CollectionValidationValue` | [`domain/evidence/observations.go`](../../domain/evidence/observations.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `RedactedValidationValue` | [`domain/evidence/observations.go`](../../domain/evidence/observations.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `ValidationValue.Validate` | [`domain/evidence/observations.go`](../../domain/evidence/observations.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `ValidationValue.Equal` | [`domain/evidence/observations.go`](../../domain/evidence/observations.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `ValidationValue.CollectionValue` | [`domain/evidence/observations.go`](../../domain/evidence/observations.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `ValidationBranchDisposition.Validate` | [`domain/evidence/observations.go`](../../domain/evidence/observations.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `NewValidationGroupTerminalObservation` | [`domain/evidence/observations.go`](../../domain/evidence/observations.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `ValidationGroupTerminalObservation.ExpectedMembers` | [`domain/evidence/observations.go`](../../domain/evidence/observations.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `ValidationGroupTerminalObservation.Validate` | [`domain/evidence/observations.go`](../../domain/evidence/observations.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `ValidationProgressObservation.Validate` | [`domain/evidence/observations.go`](../../domain/evidence/observations.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `ValidationObservation.Validate` | [`domain/evidence/observations.go`](../../domain/evidence/observations.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |

## 测试用例证据矩阵

| Test case | 输入、边界或业务前置状态 | 预期契约 | 可执行证据 |
|---|---|---|---|
| `TestStepTransitionCommitValidatesGroupTerminalFactsAndFinalMemberTopology` | `Step Transition Commit Validates Group Terminal Facts And Final Member Topology`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/evidence/commits_test.go`](../../domain/evidence/commits_test.go) · `TestStepTransitionCommitValidatesGroupTerminalFactsAndFinalMemberTopology` |
| `TestStepTransitionCommitRejectsContradictoryGroupOutcomes` | `Step Transition Commit Rejects Contradictory Group Outcomes`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/evidence/commits_test.go`](../../domain/evidence/commits_test.go) · `TestStepTransitionCommitRejectsContradictoryGroupOutcomes` |
| `TestStepTransitionCommitValidatesAtomicFactIdentity` | `Step Transition Commit Validates Atomic Fact Identity`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/evidence/commits_test.go`](../../domain/evidence/commits_test.go) · `TestStepTransitionCommitValidatesAtomicFactIdentity` |
| `TestStepTransitionCommitRejectsDuplicateHealAndResetIdentities` | `Step Transition Commit Rejects Duplicate Heal And Reset Identities`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/evidence/commits_test.go`](../../domain/evidence/commits_test.go) · `TestStepTransitionCommitRejectsDuplicateHealAndResetIdentities` |
| `TestStepProgressEventAcceptsOnlyRuntimeNonTerminalPhases` | `Step Progress Event Accepts Only Runtime Non Terminal Phases`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/evidence/events_test.go`](../../domain/evidence/events_test.go) · `TestStepProgressEventAcceptsOnlyRuntimeNonTerminalPhases` |
| `TestStepProgressEventRejectsMissingIdentityAndRuntimeCoordinates` | `Step Progress Event Rejects Missing Identity And Runtime Coordinates`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/evidence/events_test.go`](../../domain/evidence/events_test.go) · `TestStepProgressEventRejectsMissingIdentityAndRuntimeCoordinates` |
| `TestStepFactRequiresTerminalIdentity` | `Step Fact Requires Terminal Identity`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/evidence/facts_test.go`](../../domain/evidence/facts_test.go) · `TestStepFactRequiresTerminalIdentity` |
| `TestHealObservationUsesEvidenceOwnedDecisionBand` | `Heal Observation Uses Evidence Owned Decision Band`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/evidence/observations_test.go`](../../domain/evidence/observations_test.go) · `TestHealObservationUsesEvidenceOwnedDecisionBand` |
| `TestValidationValueDistinguishesAbsentScalarCollectionAndRedacted` | `Validation Value Distinguishes Absent Scalar Collection And Redacted`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/evidence/observations_test.go`](../../domain/evidence/observations_test.go) · `TestValidationValueDistinguishesAbsentScalarCollectionAndRedacted` |
| `TestValidationValueRejectsInvalidKindFieldCombinations` | `Validation Value Rejects Invalid Kind Field Combinations`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/evidence/observations_test.go`](../../domain/evidence/observations_test.go) · `TestValidationValueRejectsInvalidKindFieldCombinations` |
| `TestValidationGroupTerminalObservationRequiresConsistentWinnerAndReason` | `Validation Group Terminal Observation Requires Consistent Winner And Reason`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/evidence/observations_test.go`](../../domain/evidence/observations_test.go) · `TestValidationGroupTerminalObservationRequiresConsistentWinnerAndReason` |
| `TestValidationObservationRejectsUnknownReviewStatus` | `Validation Observation Rejects Unknown Review Status`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/evidence/observations_test.go`](../../domain/evidence/observations_test.go) · `TestValidationObservationRejectsUnknownReviewStatus` |
| `TestExportedValidatorsStrictBoundaries` | `Exported Validators Strict Boundaries`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/evidence/strict_boundary_test.go`](../../domain/evidence/strict_boundary_test.go) · `TestExportedValidatorsStrictBoundaries` |
| `TestValidationValuesNilEmptyDuplicatesAndImmutability` | `Validation Values Nil Empty Duplicates And Immutability`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/evidence/strict_boundary_test.go`](../../domain/evidence/strict_boundary_test.go) · `TestValidationValuesNilEmptyDuplicatesAndImmutability` |
| `TestValidationGroupExpectedMembersImmutable` | `Validation Group Expected Members Immutable`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/evidence/strict_boundary_test.go`](../../domain/evidence/strict_boundary_test.go) · `TestValidationGroupExpectedMembersImmutable` |
| `TestStepFactRejectsWhitespaceIdentity` | `Step Fact Rejects Whitespace Identity`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/evidence/strict_boundary_test.go`](../../domain/evidence/strict_boundary_test.go) · `TestStepFactRejectsWhitespaceIdentity` |

| `TestValidationGroupTopologyReportsLeftoverMembersDeterministically` | 同一 commit 含两个不同类别的剩余 group 成员，两种排列各跑 200 次。 | 同一输入必得同一错误；遍历源切片而非 map，排除随机 map 迭代顺序。 | [`domain/evidence/commits_test.go`](../../domain/evidence/commits_test.go) · `TestValidationGroupTopologyReportsLeftoverMembersDeterministically` |
| `TestStepTransitionCommitOrdersViolationsDeterministically` | 同时违反 commitId / expectedRevision / event 相位、Occurrence、时间戳五条规则。 | 标量违规按声明顺序位于集合遍历之前；100 次重复运行顺序不变。 | [`domain/evidence/commits_test.go`](../../domain/evidence/commits_test.go) · `TestStepTransitionCommitOrdersViolationsDeterministically` |
| `TestStepTransitionCommitTruncatesViolationsAtCap` | 40 个全部非法的 heal observation（远超 fault.MaxViolations）。 | violation 数恰为上限，且截断前缀在重复运行中确定。 | [`domain/evidence/commits_test.go`](../../domain/evidence/commits_test.go) · `TestStepTransitionCommitTruncatesViolationsAtCap` |
| `TestValidationProgressObservationValidateRuleMatrix` | `Validation Progress Observation Validate Rule Matrix`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/evidence/public_methods_coverage_test.go`](../../domain/evidence/public_methods_coverage_test.go) · `TestValidationProgressObservationValidateRuleMatrix` |
| `TestHealObservationValidateBusinessBoundaryMatrix` | `Heal Observation Validate Business Boundary Matrix`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/evidence/public_methods_coverage_test.go`](../../domain/evidence/public_methods_coverage_test.go) · `TestHealObservationValidateBusinessBoundaryMatrix` |
| `TestValidationValueEqualityKindsAndCollectionOwnership` | `Validation Value Equality Kinds And Collection Ownership`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/evidence/public_methods_coverage_test.go`](../../domain/evidence/public_methods_coverage_test.go) · `TestValidationValueEqualityKindsAndCollectionOwnership` |
| `TestValidationObservationFinalDispositionStateMatrix` | `Validation Observation Final Disposition State Matrix`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/evidence/public_methods_coverage_test.go`](../../domain/evidence/public_methods_coverage_test.go) · `TestValidationObservationFinalDispositionStateMatrix` |
| `TestStepFactTerminalPhaseAndBoundaryMatrix` | `Step Fact Terminal Phase And Boundary Matrix`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/evidence/public_methods_coverage_test.go`](../../domain/evidence/public_methods_coverage_test.go) · `TestStepFactTerminalPhaseAndBoundaryMatrix` |
| `TestStepTransitionCommitRejectsCombinedFactLimitAndCrossStepHeal` | `Step Transition Commit Rejects Combined Fact Limit And Cross Step Heal`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/evidence/public_methods_coverage_test.go`](../../domain/evidence/public_methods_coverage_test.go) · `TestStepTransitionCommitRejectsCombinedFactLimitAndCrossStepHeal` |
| `TestValidationGroupTerminalObservationRejectsIdentityAndMemberDuplicates` | `Validation Group Terminal Observation Rejects Identity And Member Duplicates`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/evidence/public_methods_coverage_test.go`](../../domain/evidence/public_methods_coverage_test.go) · `TestValidationGroupTerminalObservationRejectsIdentityAndMemberDuplicates` |
| `TestEveryValidatingCoordinateCarrierRejectsNonPositiveOccurrence` | 六个带 `Validate` 的坐标载体逐一把 `Occurrence` 置为 `0` 与 `-1`。 | 合法值 `1` 通过，`0` 与 `-1` 均被拒。`StepPhaseEvent` 与 `HealCandidateReset` 不在表内：二者无 `Validate`，由 `StepTransitionCommit.Validate` 覆盖。 | [`domain/evidence/occurrence_test.go`](../../domain/evidence/occurrence_test.go) · `TestEveryValidatingCoordinateCarrierRejectsNonPositiveOccurrence` |

## 跨入口与一致性用例

同包及其子目录中名称含 `Conformance`、`Transaction`、`Race`、`Rollback`、`Replay`、`Concurrent` 或 `Fence` 的测试，属于跨入口契约；它们已在上方矩阵逐行列出。application 包的 `conformancetest/` 证据也归属此表。

## 维护规则

1. 新增或删除 `Test…` 函数时，必须同步更新本表；表驱动新增子案例要更新相应行的边界描述。
2. 新增公开 domain API 或 application use case 时，必须先添加公开入口清单行和至少一条可执行测试证据。
3. 文档不替代测试；冲突时以 Go 测试断言和领域契约为准。
