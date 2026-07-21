# 06 聚合

- **WorkflowAggregate** (`domain/workspace/assets.go:299`): workflow identity, references, steps and versions. Invariants include version/reference consistency and validated workflow structure.
- **TestTaskAggregate** (`domain/workspace/test_task_types.go:34`): task identity, items, versions and execution plans. It governs what is runnable and dependency snapshots.
- **Program** (`domain/node/runtime.go:22`): compiled executable tree plus spec index. It is an immutable execution input by convention; runtime overlays must not mutate it.
- **Runtime** (`domain/node/runtime.go:192`): run-scoped execution coordination state. It is not a durable aggregate; it owns ephemeral invariants such as overlay isolation, pacing, retries and sink usage.
- **StepExecution** (`domain/node/runtime.go:43`): phase state machine; `ValidatePhaseTransition` is its core invariant.
- **NodeSpec/Fingerprint** (`domain/fingerprint/fingerprint.go`): target identity value objects with selector and fingerprint validation.
- **Healing Decision/Assessment** (`domain/heal/heal.go`, `assessment.go`): candidate outcome and safety disposition. Assessment prevents unsafe candidates from entering overlays.
- **Evidence records** (`domain/workspace/evidence.go`): immutable facts for replay/review, but persistence ownership is external.

Aggregates are behavioral boundaries only where invariants are enforced. Runtime and evidence records should not be mistaken for database entities or 完成 durable aggregates.
