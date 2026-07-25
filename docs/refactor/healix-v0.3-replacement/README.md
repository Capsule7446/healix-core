# Healix v0.3 替换差异审核包

## 来源与核验基线

- 原始评估：相邻 Healix 仓库 `C:\Users\Paul\workspace\healix\docs\refactor\healix-core-v0.3.0-replacement-assessment.md`。
- 案例映射：本目录的 C01–C28 分别对应原始评估中同编号的表格行与详细章节。
- 核验方式：原始评估只作为需求输入；所有“当前判断”均以 healix-core 当前实现（`12d1ba2`）重新核验，不沿用旧结论。
- 核验提交：`12d1ba2`。原始评估所在 Healix 仓库未在本审核包中固定修订号，因此实施前若源文件变化需重新比对。


## 当前实现模型

- 环境是普通 `Properties` 快照，通过 `${env.<name>}` 注入；核心库没有凭据、提供程序、保管库或明文读取子系统。
- 参数使用 `parameter.Value` / `parameter.Binding` 的封闭类型；根与嵌套 `LATEST` 在创建执行实例时解析并冻结。
- `Program` / `Runtime` 是一次执行内的临时执行对象，不是持久领域资产；执行领域拥有执行实例/执行计划，`domain/heal` 拥有自愈判断。
- `HealObservation` 先作为提交意图中的观察事实；只有提交结果才能报告晋升，运行时不得直接发布 NodeVersion。
- `SamplingWorkspace` 是可编辑草稿，`Session` 是捕获生命周期，二者不得混用。采样/自愈可发布其负责的资产，但 TestTaskVersion 只允许人工保存/发布。
- 测试任务的每个顶层执行项使用独立浏览器；同一顶层执行项的根工作流与嵌套工作流共享该浏览器。
- 调度是 application orchestration：负责创建/claim/推进/取消/重排执行实例，不下沉为领域聚合。
- 显式中止已闭环：`AbortRunService` 要求原子提交权威的 `execution.Aborted` 后发送取消信号；信号失败保留提交结果并返回 `ErrRunSignalRetryable`。普通执行上下文取消仍为 `CANCELED`，是不同操作。

## 总体清单

- [总体调整清单](adjustment-checklist.md)

## 案例清单

| 案例 | 主题 | 当前判断 | 审核文件 |
|---|---|---|---|
| C01 | 发布不可变测试任务版本 | 已完成 | [C01](cases/C01-publish-immutable-test-task-version.md) |
| C02 | 创建不可变执行实例 | 已完成 | [C02](cases/C02-create-immutable-run.md) |
| C03 | `LATEST` 版本冻结 | 已完成 | [C03](cases/C03-freeze-latest-at-run-creation.md) |
| C04 | 类型化参数 | 已完成 | [C04](cases/C04-typed-parameters.md) |
| C05 | 嵌套参数绑定 | 已完成 | [C05](cases/C05-nested-workflow-bindings.md) |
| C06 | 参数快照 | 已完成 | [C06](cases/C06-parameter-snapshots.md) |
| C07 | 环境与策略冻结 | 已完成 | [C07](cases/C07-freeze-environment-and-policies.md) |
| C08 | 工作流编译执行 | 已完成 | [C08](cases/C08-compile-execute-workflow.md) |
| C09 | 校验 | 已完成 | [C09](cases/C09-validation.md) |
| C10 | 未找到后自愈 | 已完成 | [C10](cases/C10-heal-after-not-found.md) |
| C11 | `APPLIED` 作用于当前执行实例 | 已完成 | [C11](cases/C11-applied-heal-current-run.md) |
| C12 | `BELOW_CAP` 作用于当前执行实例 | 已完成 | [C12](cases/C12-below-cap-current-run.md) |
| C13 | `APPLIED` 自动发布 | 已完成 | [C13](cases/C13-auto-publish-applied-heal.md) |
| C14 | `BELOW_CAP` 等待审核 | 已完成 | [C14](cases/C14-await-approval-below-cap.md) |
| C15 | 候选批准/拒绝 | 已完成 | [C15](cases/C15-review-heal-candidate.md) |
| C16 | 原选择器恢复后重置 | 已完成 | [C16](cases/C16-reset-on-original-selector-recovery.md) |
| C17 | 顺序执行 | 已完成 | [C17](cases/C17-sequential-workflows.md) |
| C18 | 失败即停止 | 已完成 | [C18](cases/C18-stop-on-failure.md) |
| C19 | 失败后继续 | 已完成 | [C19](cases/C19-continue-on-failure.md) |
| C20 | 队列领取与重排 | 已完成 | [C20](cases/C20-queue-claim-reorder.md) |
| C21 | 取消排队项 | 已完成 | [C21](cases/C21-cancel-queued-run.md) |
| C22 | 中止活动项 | 已完成 | [C22](cases/C22-abort-active-run.md) |
| C23 | 原子终态事实 | 已完成 | [C23](cases/C23-atomic-terminal-facts.md) |
| C24 | 采样生命周期 | 已完成 | [C24](cases/C24-sampling-session-lifecycle.md) |
| C25 | 采样草稿编辑 | 已完成 | [C25](cases/C25-editable-sampling-draft.md) |
| C26 | 采样发布策略 | 已完成 | [C26](cases/C26-sampling-publication-modes.md) |
| C27 | 临时转正式资产 | 已完成 | [C27](cases/C27-temporary-to-formal-assets.md) |
| C28 | 环境属性注入 | 已完成 | [C28](cases/C28-credential-reference-boundary.md) |

## 建议审核顺序

1. 先审总体边界和 P0/P1。
2. 审 C02–C07，确认 RunSnapshot 与参数模型。
3. 审 C10–C16/C23，确认自愈治理语义。
4. 审 C17–C23，确认顶层执行项浏览器隔离、串行执行、领取执行权/栅栏校验、取消/中止/重排与原子终态语义。
5. 审 C24–C28，确认采样发布和环境属性注入。
6. 最后审 C01/C08/C09，确认已有能力只做增量增强。
