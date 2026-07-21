# 08 DDD 模型审查

## 优点

- Domain invariants are explicit for phase transitions, fingerprints, selectors, workflow status, and healing confidence.
- Browser SDK concerns are kept behind ports.
- Healing is deterministic and safety-assessed rather than directly mutating targets.
- Tests cover business matrices and cancellation/error behavior.

## 发现

- `node.Runtime` is a high-centrality orchestration object and imports both fingerprint and heal concepts.
- Workspace evidence types sit beside authoring models, making the evidence/read boundary less explicit.
- `Program` is conventionally immutable but uses maps and slices; callers must honor copy/overlay discipline.
- The event-like facts have no explicit durable event identity or transaction contract in Core.
- Concrete read models are absent; `WorkflowQueryResult` and similar structs are contracts, not proven projections.

## G2 结论

该模型解释了当前写侧行为。 The principal model risk is confusing ephemeral Runtime/facts with durable aggregates and assuming evidence sinks imply a 完成 event store.
