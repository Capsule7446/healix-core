# 10 业务视图

| 视图 | 消费者 | 字段/来源 | 刷新/同步状态 |
|---|---|---|---|
| Node list/detail | authoring UI/API | `NodeQueryResult` (`NodeAggregate` + `RefCount`) | Reader port; concrete adapter owns joins/cache |
| Workflow list/detail | authoring UI/API | `WorkflowQueryResult` plus `LastRunStatus/LastRunAt` | Reader port; latest-run derivation external |
| Test task list/detail | task UI/API | `TestTaskQueryResult` plus latest-run fields | Reader port; concrete query path external |
| Run dashboard | monitoring UI | `Dashboard`: status counts, recent runs, queue, task projections | Reader port; freshness/materialization unspecified |
| Execution detail/timeline | monitor/replay UI | `ExecutionDetail`: execution, steps, requests, heals, validations | Facts synchronize through execution writer/committer contracts |
| Healing review | review UI | `HealCandidateRecord`, `HealObservationDetail`, candidate evidence and samples | Review/read record exists; query and review-state persistence external |
| Heal quality report | analytics UI | `metrics.Query` → immutable `ObservationFact` → `metrics.Report/Bucket` | Pure read projection; no writer in metrics package |
| Framework diagnostics | debugging UI | `PageObservation`, `FrameworkStack`, NodeSpec/Fingerprint | Host supplies sanitized observations; persistence/query external |

本仓库没有页面或 Controller 实现。 Workspace reader ports in `domain/workspace/ports.go` and metrics reader contracts define the integration surface; host adapters own API DTOs, storage and refresh jobs.

## Read-side pollution visible today

- Query results embed full write-side aggregates (`NodeQueryResult`, `WorkflowQueryResult`, `TestTaskQueryResult`).
- `TestTaskRun` mixes lifecycle facts with display/progress fields such as queue position and current step.
- `Environment`/`EnvironmentSnapshot` contain credentials and must not be reused directly as UI responses.
