# domain/heal 测试用例矩阵

## 范围与口径

本表记录 `domain/heal` 的公开业务入口和全部顶层 Go testcase。Go 测试源码是唯一可执行事实；表驱动测试的全部子案例由其对应的测试函数统一引用。

## 公开 API 与领域入口

| 公开入口 | 定义文件 | 测试证据状态 |
|---|---|---|
| `Assess` | [`domain/heal/assessment.go`](../../domain/heal/assessment.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `EvidenceFor` | [`domain/heal/evidence.go`](../../domain/heal/evidence.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Decision.Validate` | [`domain/heal/heal.go`](../../domain/heal/heal.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `DefaultThresholds` | [`domain/heal/healer.go`](../../domain/heal/healer.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Thresholds.Validate` | [`domain/heal/healer.go`](../../domain/heal/healer.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `NewDefaultHealer` | [`domain/heal/healer.go`](../../domain/heal/healer.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `NewDefaultHealerWithPolicy` | [`domain/heal/healer.go`](../../domain/heal/healer.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `DefaultHealer.Validate` | [`domain/heal/healer.go`](../../domain/heal/healer.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `DefaultHealer.Heal` | [`domain/heal/healer.go`](../../domain/heal/healer.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `DefaultPolicyV1` | [`domain/heal/policy.go`](../../domain/heal/policy.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `PolicyV1.Validate` | [`domain/heal/policy.go`](../../domain/heal/policy.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Decision.Samples` | [`domain/heal/sample.go`](../../domain/heal/sample.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `CandidateHash` | [`domain/heal/sample.go`](../../domain/heal/sample.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `ValidateSamples` | [`domain/heal/sample.go`](../../domain/heal/sample.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `SortSamples` | [`domain/heal/sample.go`](../../domain/heal/sample.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `DefaultWeights` | [`domain/heal/scorer.go`](../../domain/heal/scorer.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Weights.Validate` | [`domain/heal/scorer.go`](../../domain/heal/scorer.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |

## 测试用例证据矩阵

| 测试用例 | 输入、边界或业务前置状态 | 预期契约 | 可执行证据 |
|---|---|---|---|
| `TestAssessDispositionBusinessMatrix` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/assessment_business_matrix_test.go`](../../domain/heal/assessment_business_matrix_test.go) · `TestAssessDispositionBusinessMatrix` |
| `TestAssessBlocksOriginMismatch` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/assessment_test.go`](../../domain/heal/assessment_test.go) · `TestAssessBlocksOriginMismatch` |
| `TestAssessReviewsAmbiguousCandidates` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/assessment_test.go`](../../domain/heal/assessment_test.go) · `TestAssessReviewsAmbiguousCandidates` |
| `TestAssessAllowsDistinctHighConfidenceCandidate` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/assessment_test.go`](../../domain/heal/assessment_test.go) · `TestAssessAllowsDistinctHighConfidenceCandidate` |
| `TestEvidenceForUsesStableDimensions` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/evidence_test.go`](../../domain/heal/evidence_test.go) · `TestEvidenceForUsesStableDimensions` |
| `TestThresholdsValidateDirectBoundaries` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/exported_direct_test.go`](../../domain/heal/exported_direct_test.go) · `TestThresholdsValidateDirectBoundaries` |
| `TestWeightsValidateDirectBoundariesAndPrecedence` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/exported_direct_test.go`](../../domain/heal/exported_direct_test.go) · `TestWeightsValidateDirectBoundariesAndPrecedence` |
| `TestPolicyV1ValidateDirectPrecedence` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/exported_direct_test.go`](../../domain/heal/exported_direct_test.go) · `TestPolicyV1ValidateDirectPrecedence` |
| `TestDecisionValidateDirectMalformedShapesAndScores` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/exported_direct_test.go`](../../domain/heal/exported_direct_test.go) · `TestDecisionValidateDirectMalformedShapesAndScores` |
| `TestAssessDirectMarginBoundariesAndErrorPrecedence` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/exported_direct_test.go`](../../domain/heal/exported_direct_test.go) · `TestAssessDirectMarginBoundariesAndErrorPrecedence` |
| `TestDefaultHealerHealDirectErrorPrecedenceAndCollections` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/exported_direct_test.go`](../../domain/heal/exported_direct_test.go) · `TestDefaultHealerHealDirectErrorPrecedenceAndCollections` |
| `TestCandidateHashDirectStabilityAndFieldSensitivity` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/exported_direct_test.go`](../../domain/heal/exported_direct_test.go) · `TestCandidateHashDirectStabilityAndFieldSensitivity` |
| `TestValidateSamplesDirectMalformedAndDuplicateCollections` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/exported_direct_test.go`](../../domain/heal/exported_direct_test.go) · `TestValidateSamplesDirectMalformedAndDuplicateCollections` |
| `TestSortSamplesDirectStableOrderingAndDeepOwnership` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/exported_direct_test.go`](../../domain/heal/exported_direct_test.go) · `TestSortSamplesDirectStableOrderingAndDeepOwnership` |
| `TestFrameworkWeightIsOptional` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/framework_score_test.go`](../../domain/heal/framework_score_test.go) · `TestFrameworkWeightIsOptional` |
| `TestThresholdsValidate` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestThresholdsValidate` |
| `TestPolicyV1DefaultsAndValidation` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestPolicyV1DefaultsAndValidation` |
| `TestNewDefaultHealerWithPolicyCopiesAndValidates` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestNewDefaultHealerWithPolicyCopiesAndValidates` |
| `TestDecisionValidate` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestDecisionValidate` |
| `TestDecisionRejectsEveryInvalidSelectorShape` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestDecisionRejectsEveryInvalidSelectorShape` |
| `TestDefaultHealerRejectsInvalidConfigurationBeforeSnapshot` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestDefaultHealerRejectsInvalidConfigurationBeforeSnapshot` |
| `TestDefaultHealerRejectsNilAndPropagatesSnapshotFailures` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestDefaultHealerRejectsNilAndPropagatesSnapshotFailures` |
| `TestHealThresholdBoundariesAreInclusive` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestHealThresholdBoundariesAreInclusive` |
| `TestHealRejectsInvalidCandidateSelectorFromSnapshot` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestHealRejectsInvalidCandidateSelectorFromSnapshot` |
| `TestHeal_ExactCloneAppliesHighConfidence` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestHeal_ExactCloneAppliesHighConfidence` |
| `TestHeal_PartialMatchGoesToReview` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestHeal_PartialMatchGoesToReview` |
| `TestHeal_UnrelatedElementIsNoCandidate` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestHeal_UnrelatedElementIsNoCandidate` |
| `TestHeal_EmptySnapshotIsNoCandidate` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestHeal_EmptySnapshotIsNoCandidate` |
| `TestHeal_PicksHighestScoringOfMultipleCandidates` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestHeal_PicksHighestScoringOfMultipleCandidates` |
| `TestHeal_TiedCandidatesUseStableTotalOrder` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestHeal_TiedCandidatesUseStableTotalOrder` |
| `TestHeal_TiedCandidatesUseSelectorPriorityAsDeterministicKey` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestHeal_TiedCandidatesUseSelectorPriorityAsDeterministicKey` |
| `TestDecisionValidateRequiresOrderedCandidatesAndBestAtFront` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestDecisionValidateRequiresOrderedCandidatesAndBestAtFront` |
| `TestLCSLengthMatchesMatrixOracle` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/lcs_test.go`](../../domain/heal/lcs_test.go) · `TestLCSLengthMatchesMatrixOracle` |
| `TestNarrowByPathLCSMatchesMatrixOracle` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/lcs_test.go`](../../domain/heal/lcs_test.go) · `TestNarrowByPathLCSMatchesMatrixOracle` |
| `TestDecisionSamplesRetainEligibleCandidatesAndSelection` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/sample_test.go`](../../domain/heal/sample_test.go) · `TestDecisionSamplesRetainEligibleCandidatesAndSelection` |
| `TestScoreMatchesLegacyOracle` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/scorer_optimization_test.go`](../../domain/heal/scorer_optimization_test.go) · `TestScoreMatchesLegacyOracle` |
| `TestPreparedTargetScorerMatchesLegacyDecision` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/scorer_optimization_test.go`](../../domain/heal/scorer_optimization_test.go) · `TestPreparedTargetScorerMatchesLegacyDecision` |
| `TestSimTextUsesRuneLengthForUnicode` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/scorer_test.go`](../../domain/heal/scorer_test.go) · `TestSimTextUsesRuneLengthForUnicode` |
| `TestSimIndexStaysBoundedAtIntegerExtremes` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/scorer_test.go`](../../domain/heal/scorer_test.go) · `TestSimIndexStaysBoundedAtIntegerExtremes` |
| `TestWeightsValidate` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/scorer_test.go`](../../domain/heal/scorer_test.go) · `TestWeightsValidate` |
| `TestScore_LabelTextMatchIncreasesScore` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/scorer_test.go`](../../domain/heal/scorer_test.go) · `TestScore_LabelTextMatchIncreasesScore` |
| `TestScore_LabelTextDimensionSkippedWhenTargetHasNone` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/scorer_test.go`](../../domain/heal/scorer_test.go) · `TestScore_LabelTextDimensionSkippedWhenTargetHasNone` |
| `TestScore_ContainerMatchIncreasesScore` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/scorer_test.go`](../../domain/heal/scorer_test.go) · `TestScore_ContainerMatchIncreasesScore` |
| `TestScore_DynamicDataAttributeContributesToAttrsDimension` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/scorer_test.go`](../../domain/heal/scorer_test.go) · `TestScore_DynamicDataAttributeContributesToAttrsDimension` |
| `TestFingerprintCanonicalKeyIgnoresAttributeInsertionOrder` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/scorer_test.go`](../../domain/heal/scorer_test.go) · `TestFingerprintCanonicalKeyIgnoresAttributeInsertionOrder` |
| `TestHealDecisionNeverAliasesTheCallersSnapshot` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/decision_aliasing_test.go`](../../domain/heal/decision_aliasing_test.go) · `TestHealDecisionNeverAliasesTheCallersSnapshot` |
| `TestHealDecisionBestDoesNotShareAFingerprintWithCandidatesZero` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/decision_aliasing_test.go`](../../domain/heal/decision_aliasing_test.go) · `TestHealDecisionBestDoesNotShareAFingerprintWithCandidatesZero` |
| `TestDecisionValidateOutcomeAndOrderingMatrix` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/public_methods_matrix_test.go`](../../domain/heal/public_methods_matrix_test.go) · `TestDecisionValidateOutcomeAndOrderingMatrix` |
| `TestAssessCoversSafetyRulesAndURLNormalization` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/public_methods_matrix_test.go`](../../domain/heal/public_methods_matrix_test.go) · `TestAssessCoversSafetyRulesAndURLNormalization` |
| `TestAssessAmbiguityMarginBusinessBoundaryAndPrecedence` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/public_methods_matrix_test.go`](../../domain/heal/public_methods_matrix_test.go) · `TestAssessAmbiguityMarginBusinessBoundaryAndPrecedence` |
| `TestPolicyV1ValidationPropagatesThresholdAndWeightRules` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/public_methods_matrix_test.go`](../../domain/heal/public_methods_matrix_test.go) · `TestPolicyV1ValidationPropagatesThresholdAndWeightRules` |
| `TestWeightsValidateRejectsFiniteValuesWhoseSumOverflows` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/public_methods_matrix_test.go`](../../domain/heal/public_methods_matrix_test.go) · `TestWeightsValidateRejectsFiniteValuesWhoseSumOverflows` |
| `TestDecisionSamplesReviewCapBoundaryAndInvalidInputs` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/public_methods_matrix_test.go`](../../domain/heal/public_methods_matrix_test.go) · `TestDecisionSamplesReviewCapBoundaryAndInvalidInputs` |
| `TestValidateSamplesRejectsMalformedRanksScoresAndSelection` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/public_methods_matrix_test.go`](../../domain/heal/public_methods_matrix_test.go) · `TestValidateSamplesRejectsMalformedRanksScoresAndSelection` |
| `TestValidateSamplesRejectsStatusFlagContradictions` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/public_methods_matrix_test.go`](../../domain/heal/public_methods_matrix_test.go) · `TestValidateSamplesRejectsStatusFlagContradictions` |
| `TestSortSamplesOrdersAndDeepCopiesEvidence` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/public_methods_matrix_test.go`](../../domain/heal/public_methods_matrix_test.go) · `TestSortSamplesOrdersAndDeepCopiesEvidence` |

## 跨入口与一致性用例

同包及其子目录中名称含 `Conformance`、`Transaction`、`Race`、`Rollback`、`Replay`、`Concurrent` 或 `Fence` 的测试，属于跨入口契约；它们已在上方矩阵逐行列出。application 包的 `conformancetest/` 证据也归属此表。

## 维护规则

1. 新增或删除 `Test…` 函数时，必须同步更新本表；表驱动新增子案例要更新相应行的边界描述。
2. 新增公开 domain API 或 application use case 时，必须先添加公开入口清单行和至少一条可执行测试证据。
3. 文档不替代测试；冲突时以 Go 测试断言和领域契约为准。
