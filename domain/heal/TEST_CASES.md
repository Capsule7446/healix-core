# domain/heal Test Case Matrix

## 范围与口径

本表记录 `domain/heal` 的公开业务入口和全部顶层 Go testcase。Go 测试源码是唯一可执行事实；表驱动测试的全部子案例由其对应的测试函数统一引用。

## Public API / Use-case Inventory

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

## Test Case Evidence Matrix

| Test case | 输入、边界或业务前置状态 | 预期契约 | 可执行证据 |
|---|---|---|---|
| `TestAssessDispositionBusinessMatrix` | `Assess Disposition Business Matrix`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/assessment_business_matrix_test.go`](../../domain/heal/assessment_business_matrix_test.go) · `TestAssessDispositionBusinessMatrix` |
| `TestAssessBlocksOriginMismatch` | `Assess Blocks Origin Mismatch`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/assessment_test.go`](../../domain/heal/assessment_test.go) · `TestAssessBlocksOriginMismatch` |
| `TestAssessReviewsAmbiguousCandidates` | `Assess Reviews Ambiguous Candidates`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/assessment_test.go`](../../domain/heal/assessment_test.go) · `TestAssessReviewsAmbiguousCandidates` |
| `TestAssessAllowsDistinctHighConfidenceCandidate` | `Assess Allows Distinct High Confidence Candidate`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/assessment_test.go`](../../domain/heal/assessment_test.go) · `TestAssessAllowsDistinctHighConfidenceCandidate` |
| `TestEvidenceForUsesStableDimensions` | `Evidence For Uses Stable Dimensions`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/evidence_test.go`](../../domain/heal/evidence_test.go) · `TestEvidenceForUsesStableDimensions` |
| `TestThresholdsValidateDirectBoundaries` | `Thresholds Validate Direct Boundaries`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/exported_direct_test.go`](../../domain/heal/exported_direct_test.go) · `TestThresholdsValidateDirectBoundaries` |
| `TestWeightsValidateDirectBoundariesAndPrecedence` | `Weights Validate Direct Boundaries And Precedence`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/exported_direct_test.go`](../../domain/heal/exported_direct_test.go) · `TestWeightsValidateDirectBoundariesAndPrecedence` |
| `TestPolicyV1ValidateDirectPrecedence` | `Policy V1 Validate Direct Precedence`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/exported_direct_test.go`](../../domain/heal/exported_direct_test.go) · `TestPolicyV1ValidateDirectPrecedence` |
| `TestDecisionValidateDirectMalformedShapesAndScores` | `Decision Validate Direct Malformed Shapes And Scores`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/exported_direct_test.go`](../../domain/heal/exported_direct_test.go) · `TestDecisionValidateDirectMalformedShapesAndScores` |
| `TestAssessDirectMarginBoundariesAndErrorPrecedence` | `Assess Direct Margin Boundaries And Error Precedence`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/exported_direct_test.go`](../../domain/heal/exported_direct_test.go) · `TestAssessDirectMarginBoundariesAndErrorPrecedence` |
| `TestDefaultHealerHealDirectErrorPrecedenceAndCollections` | `Default Healer Heal Direct Error Precedence And Collections`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/exported_direct_test.go`](../../domain/heal/exported_direct_test.go) · `TestDefaultHealerHealDirectErrorPrecedenceAndCollections` |
| `TestCandidateHashDirectStabilityAndFieldSensitivity` | `Candidate Hash Direct Stability And Field Sensitivity`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/exported_direct_test.go`](../../domain/heal/exported_direct_test.go) · `TestCandidateHashDirectStabilityAndFieldSensitivity` |
| `TestValidateSamplesDirectMalformedAndDuplicateCollections` | `Validate Samples Direct Malformed And Duplicate Collections`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/exported_direct_test.go`](../../domain/heal/exported_direct_test.go) · `TestValidateSamplesDirectMalformedAndDuplicateCollections` |
| `TestSortSamplesDirectStableOrderingAndDeepOwnership` | `Sort Samples Direct Stable Ordering And Deep Ownership`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/exported_direct_test.go`](../../domain/heal/exported_direct_test.go) · `TestSortSamplesDirectStableOrderingAndDeepOwnership` |
| `TestFrameworkWeightIsOptional` | `Framework Weight Is Optional`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/framework_score_test.go`](../../domain/heal/framework_score_test.go) · `TestFrameworkWeightIsOptional` |
| `TestThresholdsValidate` | `Thresholds Validate`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestThresholdsValidate` |
| `TestPolicyV1DefaultsAndValidation` | `Policy V1 Defaults And Validation`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestPolicyV1DefaultsAndValidation` |
| `TestNewDefaultHealerWithPolicyCopiesAndValidates` | `New Default Healer With Policy Copies And Validates`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestNewDefaultHealerWithPolicyCopiesAndValidates` |
| `TestDecisionValidate` | `Decision Validate`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestDecisionValidate` |
| `TestDecisionRejectsEveryInvalidSelectorShape` | `Decision Rejects Every Invalid Selector Shape`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestDecisionRejectsEveryInvalidSelectorShape` |
| `TestDefaultHealerRejectsInvalidConfigurationBeforeSnapshot` | `Default Healer Rejects Invalid Configuration Before Snapshot`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestDefaultHealerRejectsInvalidConfigurationBeforeSnapshot` |
| `TestDefaultHealerRejectsNilAndPropagatesSnapshotFailures` | `Default Healer Rejects Nil And Propagates Snapshot Failures`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestDefaultHealerRejectsNilAndPropagatesSnapshotFailures` |
| `TestHealThresholdBoundariesAreInclusive` | `Heal Threshold Boundaries Are Inclusive`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestHealThresholdBoundariesAreInclusive` |
| `TestHealRejectsInvalidCandidateSelectorFromSnapshot` | `Heal Rejects Invalid Candidate Selector From Snapshot`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestHealRejectsInvalidCandidateSelectorFromSnapshot` |
| `TestHeal_ExactCloneAppliesHighConfidence` | `Heal Exact Clone Applies High Confidence`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestHeal_ExactCloneAppliesHighConfidence` |
| `TestHeal_PartialMatchGoesToReview` | `Heal Partial Match Goes To Review`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestHeal_PartialMatchGoesToReview` |
| `TestHeal_UnrelatedElementIsNoCandidate` | `Heal Unrelated Element Is No Candidate`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestHeal_UnrelatedElementIsNoCandidate` |
| `TestHeal_EmptySnapshotIsNoCandidate` | `Heal Empty Snapshot Is No Candidate`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestHeal_EmptySnapshotIsNoCandidate` |
| `TestHeal_PicksHighestScoringOfMultipleCandidates` | `Heal Picks Highest Scoring Of Multiple Candidates`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestHeal_PicksHighestScoringOfMultipleCandidates` |
| `TestHeal_TiedCandidatesUseStableTotalOrder` | `Heal Tied Candidates Use Stable Total Order`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestHeal_TiedCandidatesUseStableTotalOrder` |
| `TestHeal_TiedCandidatesUseSelectorPriorityAsDeterministicKey` | `Heal Tied Candidates Use Selector Priority As Deterministic Key`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestHeal_TiedCandidatesUseSelectorPriorityAsDeterministicKey` |
| `TestDecisionValidateRequiresOrderedCandidatesAndBestAtFront` | `Decision Validate Requires Ordered Candidates And Best At Front`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/healer_test.go`](../../domain/heal/healer_test.go) · `TestDecisionValidateRequiresOrderedCandidatesAndBestAtFront` |
| `TestLCSLengthMatchesMatrixOracle` | `LCSLength Matches Matrix Oracle`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/lcs_test.go`](../../domain/heal/lcs_test.go) · `TestLCSLengthMatchesMatrixOracle` |
| `TestNarrowByPathLCSMatchesMatrixOracle` | `Narrow By Path LCSMatches Matrix Oracle`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/lcs_test.go`](../../domain/heal/lcs_test.go) · `TestNarrowByPathLCSMatchesMatrixOracle` |
| `TestDecisionSamplesRetainEligibleCandidatesAndSelection` | `Decision Samples Retain Eligible Candidates And Selection`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/sample_test.go`](../../domain/heal/sample_test.go) · `TestDecisionSamplesRetainEligibleCandidatesAndSelection` |
| `TestScoreMatchesLegacyOracle` | `Score Matches Legacy Oracle`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/scorer_optimization_test.go`](../../domain/heal/scorer_optimization_test.go) · `TestScoreMatchesLegacyOracle` |
| `TestPreparedTargetScorerMatchesLegacyDecision` | `Prepared Target Scorer Matches Legacy Decision`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/scorer_optimization_test.go`](../../domain/heal/scorer_optimization_test.go) · `TestPreparedTargetScorerMatchesLegacyDecision` |
| `TestSimTextUsesRuneLengthForUnicode` | `Sim Text Uses Rune Length For Unicode`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/scorer_test.go`](../../domain/heal/scorer_test.go) · `TestSimTextUsesRuneLengthForUnicode` |
| `TestSimIndexStaysBoundedAtIntegerExtremes` | `Sim Index Stays Bounded At Integer Extremes`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/scorer_test.go`](../../domain/heal/scorer_test.go) · `TestSimIndexStaysBoundedAtIntegerExtremes` |
| `TestWeightsValidate` | `Weights Validate`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/scorer_test.go`](../../domain/heal/scorer_test.go) · `TestWeightsValidate` |
| `TestScore_LabelTextMatchIncreasesScore` | `Score Label Text Match Increases Score`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/scorer_test.go`](../../domain/heal/scorer_test.go) · `TestScore_LabelTextMatchIncreasesScore` |
| `TestScore_LabelTextDimensionSkippedWhenTargetHasNone` | `Score Label Text Dimension Skipped When Target Has None`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/scorer_test.go`](../../domain/heal/scorer_test.go) · `TestScore_LabelTextDimensionSkippedWhenTargetHasNone` |
| `TestScore_ContainerMatchIncreasesScore` | `Score Container Match Increases Score`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/scorer_test.go`](../../domain/heal/scorer_test.go) · `TestScore_ContainerMatchIncreasesScore` |
| `TestScore_DynamicDataAttributeContributesToAttrsDimension` | `Score Dynamic Data Attribute Contributes To Attrs Dimension`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/scorer_test.go`](../../domain/heal/scorer_test.go) · `TestScore_DynamicDataAttributeContributesToAttrsDimension` |
| `TestFingerprintCanonicalKeyIgnoresAttributeInsertionOrder` | `Fingerprint Canonical Key Ignores Attribute Insertion Order`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/scorer_test.go`](../../domain/heal/scorer_test.go) · `TestFingerprintCanonicalKeyIgnoresAttributeInsertionOrder` |
| `TestHealDecisionNeverAliasesTheCallersSnapshot` | `Heal Decision Never Aliases The Callers Snapshot`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/decision_aliasing_test.go`](../../domain/heal/decision_aliasing_test.go) · `TestHealDecisionNeverAliasesTheCallersSnapshot` |
| `TestHealDecisionBestDoesNotShareAFingerprintWithCandidatesZero` | `Heal Decision Best Does Not Share AFingerprint With Candidates Zero`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/decision_aliasing_test.go`](../../domain/heal/decision_aliasing_test.go) · `TestHealDecisionBestDoesNotShareAFingerprintWithCandidatesZero` |
| `TestDecisionValidateOutcomeAndOrderingMatrix` | `Decision Validate Outcome And Ordering Matrix`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/public_methods_matrix_test.go`](../../domain/heal/public_methods_matrix_test.go) · `TestDecisionValidateOutcomeAndOrderingMatrix` |
| `TestAssessCoversSafetyRulesAndURLNormalization` | `Assess Covers Safety Rules And URLNormalization`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/public_methods_matrix_test.go`](../../domain/heal/public_methods_matrix_test.go) · `TestAssessCoversSafetyRulesAndURLNormalization` |
| `TestAssessAmbiguityMarginBusinessBoundaryAndPrecedence` | `Assess Ambiguity Margin Business Boundary And Precedence`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/public_methods_matrix_test.go`](../../domain/heal/public_methods_matrix_test.go) · `TestAssessAmbiguityMarginBusinessBoundaryAndPrecedence` |
| `TestPolicyV1ValidationPropagatesThresholdAndWeightRules` | `Policy V1 Validation Propagates Threshold And Weight Rules`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/public_methods_matrix_test.go`](../../domain/heal/public_methods_matrix_test.go) · `TestPolicyV1ValidationPropagatesThresholdAndWeightRules` |
| `TestWeightsValidateRejectsFiniteValuesWhoseSumOverflows` | `Weights Validate Rejects Finite Values Whose Sum Overflows`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/public_methods_matrix_test.go`](../../domain/heal/public_methods_matrix_test.go) · `TestWeightsValidateRejectsFiniteValuesWhoseSumOverflows` |
| `TestDecisionSamplesReviewCapBoundaryAndInvalidInputs` | `Decision Samples Review Cap Boundary And Invalid Inputs`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/public_methods_matrix_test.go`](../../domain/heal/public_methods_matrix_test.go) · `TestDecisionSamplesReviewCapBoundaryAndInvalidInputs` |
| `TestValidateSamplesRejectsMalformedRanksScoresAndSelection` | `Validate Samples Rejects Malformed Ranks Scores And Selection`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/public_methods_matrix_test.go`](../../domain/heal/public_methods_matrix_test.go) · `TestValidateSamplesRejectsMalformedRanksScoresAndSelection` |
| `TestValidateSamplesRejectsStatusFlagContradictions` | `Validate Samples Rejects Status Flag Contradictions`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/public_methods_matrix_test.go`](../../domain/heal/public_methods_matrix_test.go) · `TestValidateSamplesRejectsStatusFlagContradictions` |
| `TestSortSamplesOrdersAndDeepCopiesEvidence` | `Sort Samples Orders And Deep Copies Evidence`；表驱动子案例（如存在）覆盖该函数中声明的输入、状态与边界。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/heal/public_methods_matrix_test.go`](../../domain/heal/public_methods_matrix_test.go) · `TestSortSamplesOrdersAndDeepCopiesEvidence` |

## Cross-cutting / Conformance Cases

同包及其子目录中名称含 `Conformance`、`Transaction`、`Race`、`Rollback`、`Replay`、`Concurrent` 或 `Fence` 的测试，属于跨入口契约；它们已在上方矩阵逐行列出。application 包的 `conformancetest/` 证据也归属此表。

## 维护规则

1. 新增或删除 `Test…` 函数时，必须同步更新本表；表驱动新增子案例要更新相应行的边界描述。
2. 新增公开 domain API 或 application use case 时，必须先添加公开入口清单行和至少一条可执行测试证据。
3. 文档不替代测试；冲突时以 Go 测试断言和领域契约为准。
