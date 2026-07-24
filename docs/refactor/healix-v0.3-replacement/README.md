# Healix v0.3 替换差异审核包

## 来源与核验基线

- 原始评估：相邻 Healix 仓库 `C:\Users\Paul\workspace\healix\docs\refactor\healix-core-v0.3.0-replacement-assessment.md`。
- Case 映射：本目录的 C01–C28 分别对应原始评估中同编号的表格行与详细章节。
- 核验方式：原始评估只作为需求输入；所有“当前判断”均以 healix-core 当前 `master` 源码重新核验，不沿用旧结论。
- 核验提交：`595a74c`。原始评估所在 Healix 仓库未在本审核包中固定 revision，因此实施前若源文件变化需重新比对。

## 总体清单

- [总体调整清单](adjustment-checklist.md)

## Case 清单

| Case | 主题 | 当前判断 | 审核文件 |
|---|---|---|---|
| C01 | 发布不可变 TestTask 版本 | 已完成 | [C01](cases/C01-publish-immutable-test-task-version.md) |
| C02 | 创建不可变 Run | 已完成 | [C02](cases/C02-create-immutable-run.md) |
| C03 | LATEST 版本冻结 | 已完成 | [C03](cases/C03-freeze-latest-at-run-creation.md) |
| C04 | Typed 参数 | 已完成 | [C04](cases/C04-typed-parameters.md) |
| C05 | 嵌套参数绑定 | 已完成 | [C05](cases/C05-nested-workflow-bindings.md) |
| C06 | 参数快照 | 已完成 | [C06](cases/C06-parameter-snapshots.md) |
| C07 | Environment/Policy 冻结 | 已完成 | [C07](cases/C07-freeze-environment-and-policies.md) |
| C08 | Workflow 编译执行 | 已完成 | [C08](cases/C08-compile-execute-workflow.md) |
| C09 | Validation | 已完成 | [C09](cases/C09-validation.md) |
| C10 | NotFound 后自愈 | 已完成 | [C10](cases/C10-heal-after-not-found.md) |
| C11 | APPLIED 当前 Run | 已完成 | [C11](cases/C11-applied-heal-current-run.md) |
| C12 | BELOW_CAP 当前 Run | 已完成 | [C12](cases/C12-below-cap-current-run.md) |
| C13 | APPLIED 自动发布 | 已完成 | [C13](cases/C13-auto-publish-applied-heal.md) |
| C14 | BELOW_CAP 等待审核 | 已完成 | [C14](cases/C14-await-approval-below-cap.md) |
| C15 | 候选批准/拒绝 | 已完成 | [C15](cases/C15-review-heal-candidate.md) |
| C16 | 原 selector 恢复重置 | 已完成 | [C16](cases/C16-reset-on-original-selector-recovery.md) |
| C17 | 顺序执行 | 已完成 | [C17](cases/C17-sequential-workflows.md) |
| C18 | 失败即停止 | 已完成 | [C18](cases/C18-stop-on-failure.md) |
| C19 | 失败后继续 | 已完成 | [C19](cases/C19-continue-on-failure.md) |
| C20 | Queue claim/reorder | 已完成 | [C20](cases/C20-queue-claim-reorder.md) |
| C21 | Cancel queued | 已完成 | [C21](cases/C21-cancel-queued-run.md) |
| C22 | Abort active | 已完成 | [C22](cases/C22-abort-active-run.md) |
| C23 | 原子终态事实 | 已完成 | [C23](cases/C23-atomic-terminal-facts.md) |
| C24 | Sampling 生命周期 | 已完成 | [C24](cases/C24-sampling-session-lifecycle.md) |
| C25 | Sampling 草稿编辑 | 已完成 | [C25](cases/C25-editable-sampling-draft.md) |
| C26 | Sampling 发布策略 | 已完成 | [C26](cases/C26-sampling-publication-modes.md) |
| C27 | 临时转正式资产 | 已完成 | [C27](cases/C27-temporary-to-formal-assets.md) |
| C28 | Environment Properties 注入 | 已完成 | [C28](cases/C28-credential-reference-boundary.md) |

## 建议审核顺序

1. 先审总体边界和 P0/P1。
2. 审 C02–C07，确认 RunSnapshot 与参数模型。
3. 审 C10–C16/C23，确认自愈治理语义。
4. 审 C17–C23，确认 Entry 浏览器隔离、串行执行、claim/fence、Cancel/Abort/Reorder 与原子终态语义。
5. 审 C24–C28，确认采样发布和 Environment Properties 注入。
6. 最后审 C01/C08/C09，确认已有能力只做增量增强。
