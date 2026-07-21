# 02 Seams

## Seam A — Execution lifecycle composition (first slice)

- Current boundary: `application/engine.RunProgram` → `node.Runtime` → `Program.Root.Run`.
- Extraction candidate: application `RuntimeFactory`/`RunCoordinator` that validates config, copies variables, starts/stops recorder, and delegates execution.
- Testability gain: lifecycle and construction tests can use a fake root and recorder without browser nodes.
- Rollback: retain `RunProgram` as a thin adapter delegating to the old construction path; no tree or Driver changes.
- Risk: recorder stop error joining and detached timeout semantics must remain identical.

## Seam B — Healing decision service

- Current boundary: `domain/node.StepNode` reaches `heal.Healer`, `heal.SafetyPolicy`, selectors and evidence.
- Extraction candidate: execution-local healing port returning a decision plus audit/sample facts; adapter maps to existing `heal.Healer`.
- Rollback: default adapter calls existing healer; selector overlay and event order remain in node.
- Risk: preserve cancellation, attempt counts, old selector identity, and delayed success audit.

## Seam C — Fingerprint detection/classification

- Current boundary: fingerprint types are consumed by compiler, node Driver, and heal scoring.
- Extraction candidate: detection/classification service with framework-neutral result; keep NodeSpec and Selector as stable value objects.
- Rollback: old parser/detection remains default while new service is optional.
- Risk: framework confidence and unknown fallback can alter healing ranking.

## Seam D — Evidence projection

- Current boundary: node `ExecutionSink` and heal sample observer emit facts consumed by workspace.
- Extraction candidate: workspace adapter translating execution facts into candidate/evidence records.
- Rollback: preserve existing sink interfaces and add adapter beside them.
- Risk: ID-space confusion between nodeID and specID and persistence timing.

## Recommendation

Start with Seam A. It is cross-cutting but behavior-preserving, has a narrow application boundary, and reduces composition/lifecycle coupling before changing domain behavior. Defer B-D until G2 tests and slice acceptance exist.
