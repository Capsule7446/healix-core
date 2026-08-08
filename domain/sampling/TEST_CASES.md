# domain/sampling 测试用例矩阵

## 范围与口径

本表记录 `domain/sampling` 的公开业务入口和全部顶层 Go testcase。Go 测试源码是唯一可执行事实；表驱动测试的全部子案例由其对应的测试函数统一引用。

## 公开 API 与领域入口

| 公开入口 | 定义文件 | 测试证据状态 |
|---|---|---|
| `InsertUnpublishedFlowFragmentStep` | [`domain/sampling/draft.go`](../../domain/sampling/draft.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `UpdateUnpublishedFlowFragmentStep` | [`domain/sampling/draft.go`](../../domain/sampling/draft.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `DeleteUnpublishedFlowFragmentStep` | [`domain/sampling/draft.go`](../../domain/sampling/draft.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `MoveUnpublishedFlowFragmentStep` | [`domain/sampling/draft.go`](../../domain/sampling/draft.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `ReorderUnpublishedFlowFragmentSteps` | [`domain/sampling/draft.go`](../../domain/sampling/draft.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `DeleteUnpublishedElementTarget` | [`domain/sampling/draft.go`](../../domain/sampling/draft.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Match` | [`domain/sampling/matching.go`](../../domain/sampling/matching.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `RewriteUnpublishedElementTargetReferences` | [`domain/sampling/rewrite.go`](../../domain/sampling/rewrite.go) | 按临时到正式节点映射递归重写步骤引用，并保持输入不变。 |
| `NewSession` | [`domain/sampling/session.go`](../../domain/sampling/session.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Session.ID` | [`domain/sampling/session.go`](../../domain/sampling/session.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Session.Start` | [`domain/sampling/session.go`](../../domain/sampling/session.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Session.Record` | [`domain/sampling/session.go`](../../domain/sampling/session.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Session.Pause` | [`domain/sampling/session.go`](../../domain/sampling/session.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Session.Resume` | [`domain/sampling/session.go`](../../domain/sampling/session.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Session.End` | [`domain/sampling/session.go`](../../domain/sampling/session.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Session.Interrupt` | [`domain/sampling/session.go`](../../domain/sampling/session.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `Session.Snapshot` | [`domain/sampling/session.go`](../../domain/sampling/session.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `NewUUID` | [`domain/sampling/session.go`](../../domain/sampling/session.go) | 在下方 testcase 矩阵中提供直接行为证据；无业务分支的辅助 accessor 以调用方契约覆盖。 |
| `RebuildUnpublishedElementTargetReferences` | [`domain/sampling/workspace.go`](../../domain/sampling/workspace.go) | 从工作区步骤重建节点引用投影，要求节点集合与步骤引用一致。 |

## 测试用例证据矩阵

| 测试用例 | 输入、边界或业务前置状态 | 预期契约 | 可执行证据 |
|---|---|---|---|
| `TestUpdateUnpublishedFlowFragmentStepDirectNestedReplacementAndRejection` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/direct_gap_test.go`](../../domain/sampling/direct_gap_test.go) · `TestUpdateUnpublishedFlowFragmentStepDirectNestedReplacementAndRejection` |
| `TestRebuildUnpublishedElementTargetReferencesDirectNestedStaleUnknownAndAtomicity` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/direct_gap_test.go`](../../domain/sampling/direct_gap_test.go) · `TestRebuildUnpublishedElementTargetReferencesDirectNestedStaleUnknownAndAtomicity` |
| `TestDraftCommandsInsertMoveReorderAndDeleteImmutably` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/draft_test.go`](../../domain/sampling/draft_test.go) · `TestDraftCommandsInsertMoveReorderAndDeleteImmutably` |
| `TestMoveUnpublishedFlowFragmentStepUsesFinalPositionWithinContainer` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/draft_test.go`](../../domain/sampling/draft_test.go) · `TestMoveUnpublishedFlowFragmentStepUsesFinalPositionWithinContainer` |
| `TestDraftCommandsRejectInvalidIdentitiesAndReferences` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/draft_test.go`](../../domain/sampling/draft_test.go) · `TestDraftCommandsRejectInvalidIdentitiesAndReferences` |
| `TestDeleteUnpublishedElementTargetRemovesUnreferencedNode` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/draft_test.go`](../../domain/sampling/draft_test.go) · `TestDeleteUnpublishedElementTargetRemovesUnreferencedNode` |
| `TestUpdateUnpublishedFlowFragmentStepCoversEveryNestedContainerAndRebuildsReferences` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/draft_test.go`](../../domain/sampling/draft_test.go) · `TestUpdateUnpublishedFlowFragmentStepCoversEveryNestedContainerAndRebuildsReferences` |
| `TestUpdateUnpublishedFlowFragmentStepRejectsInvalidIdentityAndReferenceWithoutMutation` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/draft_test.go`](../../domain/sampling/draft_test.go) · `TestUpdateUnpublishedFlowFragmentStepRejectsInvalidIdentityAndReferenceWithoutMutation` |
| `TestReorderUnpublishedFlowFragmentStepsAcceptsEveryExactPermutation` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/draft_test.go`](../../domain/sampling/draft_test.go) · `TestReorderUnpublishedFlowFragmentStepsAcceptsEveryExactPermutation` |
| `TestReorderUnpublishedFlowFragmentStepsRejectsNonPermutations` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/draft_test.go`](../../domain/sampling/draft_test.go) · `TestReorderUnpublishedFlowFragmentStepsRejectsNonPermutations` |
| `TestMatchCombinesSelectorAndStableFingerprintSignals` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/matching_test.go`](../../domain/sampling/matching_test.go) · `TestMatchCombinesSelectorAndStableFingerprintSignals` |
| `TestMatchDoesNotRewardMissingOptionalSignals` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/matching_test.go`](../../domain/sampling/matching_test.go) · `TestMatchDoesNotRewardMissingOptionalSignals` |
| `TestMatchCountsDuplicateSelectorsOnce` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/matching_test.go`](../../domain/sampling/matching_test.go) · `TestMatchCountsDuplicateSelectorsOnce` |
| `TestRewriteUnpublishedElementTargetReferencesRecursesWithoutMutatingInput` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/rewrite_test.go`](../../domain/sampling/rewrite_test.go) · `TestRewriteUnpublishedElementTargetReferencesRecursesWithoutMutatingInput` |
| `TestRewriteUnpublishedElementTargetReferencesRequiresExactMappingSet` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/rewrite_test.go`](../../domain/sampling/rewrite_test.go) · `TestRewriteUnpublishedElementTargetReferencesRequiresExactMappingSet` |
| `TestRewriteUnpublishedElementTargetReferencesAllowsStepsWithoutNodes` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/rewrite_test.go`](../../domain/sampling/rewrite_test.go) · `TestRewriteUnpublishedElementTargetReferencesAllowsStepsWithoutNodes` |
| `TestNewSessionRejectsMissingBusinessIdentity` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/session_matrix_test.go`](../../domain/sampling/session_matrix_test.go) · `TestNewSessionRejectsMissingBusinessIdentity` |
| `TestSessionLifecycleTransitionMatrix` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/session_matrix_test.go`](../../domain/sampling/session_matrix_test.go) · `TestSessionLifecycleTransitionMatrix` |
| `TestSessionPauseResumePreservesIdentityAndSequence` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/session_matrix_test.go`](../../domain/sampling/session_matrix_test.go) · `TestSessionPauseResumePreservesIdentityAndSequence` |
| `TestSessionInterruptIsTerminalAndIdempotent` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/session_matrix_test.go`](../../domain/sampling/session_matrix_test.go) · `TestSessionInterruptIsTerminalAndIdempotent` |
| `TestSessionRecordActionContractMatrix` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/session_matrix_test.go`](../../domain/sampling/session_matrix_test.go) · `TestSessionRecordActionContractMatrix` |
| `TestSessionCaptureIDMakesRetriesIdempotentAcrossPayloadChanges` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/session_matrix_test.go`](../../domain/sampling/session_matrix_test.go) · `TestSessionCaptureIDMakesRetriesIdempotentAcrossPayloadChanges` |
| `TestSessionRejectsInvalidNewAndUpdatedNodeSpecifications` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/session_matrix_test.go`](../../domain/sampling/session_matrix_test.go) · `TestSessionRejectsInvalidNewAndUpdatedNodeSpecifications` |
| `TestSessionIdentityAccessorsAndOriginNormalization` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/session_matrix_test.go`](../../domain/sampling/session_matrix_test.go) · `TestSessionIdentityAccessorsAndOriginNormalization` |
| `TestSessionSnapshotOwnsNestedData` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/session_matrix_test.go`](../../domain/sampling/session_matrix_test.go) · `TestSessionSnapshotOwnsNestedData` |
| `TestSessionAssignsStableNodeUUIDAndIdempotentCapture` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/session_test.go`](../../domain/sampling/session_test.go) · `TestSessionAssignsStableNodeUUIDAndIdempotentCapture` |
| `TestNewUUIDUsesVersion7` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/session_test.go`](../../domain/sampling/session_test.go) · `TestNewUUIDUsesVersion7` |
| `TestSessionRejectsCaptureAfterCompletion` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/session_test.go`](../../domain/sampling/session_test.go) · `TestSessionRejectsCaptureAfterCompletion` |
| `TestNewUUIDFormatAndUniqueness` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/session_test.go`](../../domain/sampling/session_test.go) · `TestNewUUIDFormatAndUniqueness` |
| `TestNewSessionURLContract` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/strict_contract_test.go`](../../domain/sampling/strict_contract_test.go) · `TestNewSessionURLContract` |
| `TestSessionPublicMethodsAreNilSafe` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/strict_contract_test.go`](../../domain/sampling/strict_contract_test.go) · `TestSessionPublicMethodsAreNilSafe` |
| `TestDraftIndexBoundariesDoNotAllocateFromExtremeValues` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/strict_contract_test.go`](../../domain/sampling/strict_contract_test.go) · `TestDraftIndexBoundariesDoNotAllocateFromExtremeValues` |
| `TestTemporaryWorkflowTimestampBoundaryValuesRemainLossless` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/strict_contract_test.go`](../../domain/sampling/strict_contract_test.go) · `TestTemporaryWorkflowTimestampBoundaryValuesRemainLossless` |
| `TestDraftEditingNeverAliasesNodeFingerprintWithItsSource` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/draft_aliasing_test.go`](../../domain/sampling/draft_aliasing_test.go) · `TestDraftEditingNeverAliasesNodeFingerprintWithItsSource` |
| `TestUpdateUnpublishedFlowFragmentStepNeverAliasesNodeContent` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/draft_aliasing_test.go`](../../domain/sampling/draft_aliasing_test.go) · `TestUpdateUnpublishedFlowFragmentStepNeverAliasesNodeContent` |
| `TestRecordReportsEveryCaptureShapeFailureAtOnce` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/fault_envelope_test.go`](../../domain/sampling/fault_envelope_test.go) · `TestRecordReportsEveryCaptureShapeFailureAtOnce` |
| `TestDraftIdentityReportsEveryBadStepAtOnce` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/fault_envelope_test.go`](../../domain/sampling/fault_envelope_test.go) · `TestDraftIdentityReportsEveryBadStepAtOnce` |
| `TestContainerShapeMismatchIsNotReportedAsAMissingStep` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/fault_envelope_test.go`](../../domain/sampling/fault_envelope_test.go) · `TestContainerShapeMismatchIsNotReportedAsAMissingStep` |
| `TestReferenceWalksReportEveryUnresolvableReferenceAtOnce` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/fault_envelope_test.go`](../../domain/sampling/fault_envelope_test.go) · `TestReferenceWalksReportEveryUnresolvableReferenceAtOnce` |
| `TestDraftEditingNeverAliasesStepContentWithItsSource` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/fault_envelope_test.go`](../../domain/sampling/fault_envelope_test.go) · `TestDraftEditingNeverAliasesStepContentWithItsSource` |
| `TestDraftCommandsRejectBoundaryIndexesAndMissingTargetsWithoutMutation` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/public_methods_matrix_test.go`](../../domain/sampling/public_methods_matrix_test.go) · `TestDraftCommandsRejectBoundaryIndexesAndMissingTargetsWithoutMutation` |
| `TestDeleteUnpublishedFlowFragmentStepRemovesValidationBranchMember` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/public_methods_matrix_test.go`](../../domain/sampling/public_methods_matrix_test.go) · `TestDeleteUnpublishedFlowFragmentStepRemovesValidationBranchMember` |
| `TestDraftContainerSelectionRejectsImpossibleBusinessShapes` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/public_methods_matrix_test.go`](../../domain/sampling/public_methods_matrix_test.go) · `TestDraftContainerSelectionRejectsImpossibleBusinessShapes` |
| `TestDraftCommandsRejectMalformedWorkflowIdentity` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/public_methods_matrix_test.go`](../../domain/sampling/public_methods_matrix_test.go) · `TestDraftCommandsRejectMalformedWorkflowIdentity` |
| `TestRebuildUnpublishedElementTargetReferencesDerivesNestedProjectionInEncounterOrder` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/public_methods_matrix_test.go`](../../domain/sampling/public_methods_matrix_test.go) · `TestRebuildUnpublishedElementTargetReferencesDerivesNestedProjectionInEncounterOrder` |
| `TestRebuildUnpublishedElementTargetReferencesRejectsNilAndUnknownNode` | 测试函数覆盖源码声明的输入、状态与边界；表驱动子案例由该函数统一维护。 | 由测试断言验证返回值、错误分类、状态变更、所有权或副作用。 | [`domain/sampling/public_methods_matrix_test.go`](../../domain/sampling/public_methods_matrix_test.go) · `TestRebuildUnpublishedElementTargetReferencesRejectsNilAndUnknownNode` |

## 跨入口与一致性用例

同包及其子目录中名称含 `Conformance`、`Transaction`、`Race`、`Rollback`、`Replay`、`Concurrent` 或 `Fence` 的测试，属于跨入口契约；它们已在上方矩阵逐行列出。application 包的 `conformancetest/` 证据也归属此表。

## 维护规则

1. 新增或删除 `Test…` 函数时，必须同步更新本表；表驱动新增子案例要更新相应行的边界描述。
2. 新增公开 domain API 或 application use case 时，必须先添加公开入口清单行和至少一条可执行测试证据。
3. 文档不替代测试；冲突时以 Go 测试断言和领域契约为准。
