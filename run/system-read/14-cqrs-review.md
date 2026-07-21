# 14 CQRS 审查

## G3 结果

代码库在概念上清晰区分了写侧与读侧，并提供了多个有用的查询端口，但 Core 中没有具体的 CQRS 读模型存储。 Field origins can be traced for the main views, while freshness, ordering, projection ownership and UI DTO boundaries remain adapter concerns.

## 优点

- `metrics` is a clean read-side example: immutable `ObservationFact` input, pure `Project`, derived rates, and a narrow Reader contract.
- Workspace readers distinguish list/detail/dashboard/evidence queries from writer ports.
- Terminal execution facts have an atomic commit boundary; non-terminal progress is separate.
- Query fields such as `RefCount`, latest run status/time, dashboard counts and quality rates are identifiable as derived values.

## 发现

- Query results embed write-side aggregates (`NodeQueryResult`, `WorkflowQueryResult`, `TestTaskQueryResult`), creating entity-backed API risk.
- `TestTaskRun` mixes lifecycle facts with presentation/progress fields such as queue position, environment display, and current step labels.
- Environment contracts contain credentials and must not be reused as UI responses.
- Evidence/review records are mixed into the broad workspace package.
- No explicit freshness/cursor/idempotency/projection-version contract exists.
- `OperationObserver` is best-effort and must not be treated as the atomic fact stream.

## G3 结论

通过，但存在已记录的限制。 The current repository is a domain core with adapter-facing query contracts, not a 完成 read-model service.
