# 12 读模型

当前代码已经隐含了这些视图契约，但它们在物理上混杂于 `domain/automation` 而不是独立的查询命名空间:

- `NodeListItem`: stable node identity/display/version fields plus derived `RefCount`.
- `WorkflowListItem`: workflow identity/version fields plus derived latest run status/time.
- `TestTaskListItem`: task identity/version fields plus derived latest run status/time.
- `RunListItem`: safe run status, queue position, current workflow/step labels, environment display, and timestamps.
- `ExecutionDetail视图`: execution record plus step, request, heal and validation facts.
- `HealingReview视图`: observation IDs, run/node/spec IDs, old selectors, ordered candidates, score/rank/eligibility/selected status, safety disposition and explanation.
- `Dashboard视图`: status counts, 30-day total, running run, queue, recent runs, and task summaries.
- `HealQualityReport`: `metrics.Query` over immutable `ObservationFact`, producing buckets and derived rates.
- `FrameworkDiagnostic视图`: sanitized observation identity, framework name/version/confidence, evidence categories, and stable hash.

These should be projections over workspace aggregates and immutable execution facts. Runtime, Driver, Program mutation, raw DOM, and framework SDK objects must not be query sources. Credentials in `Environment`/`EnvironmentSnapshot` require a separate execution-only path and should be excluded from UI-safe views.
