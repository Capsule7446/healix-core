# 13 读模型同步

- Workspace reader ports (`FolderReader`, `EnvironmentReader`, `NodeReader`, `WorkflowReader`, `TestTaskReader`, `ExecutionEvidenceReader`, `MaintenanceReader`, `HealCandidateReader`, `DashboardReader`) are adapter-facing query boundaries.
- Execution lifecycle writers explicitly claim/start/finish/fail/finalize/cancel/recover runs; these are the source transitions for dashboard and run views.
- Terminal step facts synchronize atomically through `ExecutionFactCommitter.CommitStepTransition`; non-terminal progress uses `ExecutionProgressWriter`.
- `ExecutionDetail` fields derive from workflow execution, step, network, healing and validation records. They are not read from active Runtime.
- Metrics is a pure projection: immutable `ObservationFact` is filtered by `metrics.Query`, then buckets/rates are derived. It has no writer/repository contract.
- `OperationObserver` is best-effort with detached timeout and therefore is not equivalent to the atomic terminal fact commit.
- Host owns database schema, Wails/UI requests, materialized-view invalidation, refresh cadence, and retention.

Missing explicit contract: projection freshness/consistency, stable query cursors, event/idempotency keys, and whether latest-run/ref-count/dashboard fields are live joins, persisted denormalizations, or caches.
