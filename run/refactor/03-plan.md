# 03 Strangler Plan

## Ordered slices

### Slice 1 — Execution lifecycle coordinator

- Scope: extract construction, validation, variable snapshot, recorder lifecycle and root delegation from `RunProgram` into an application coordinator while retaining the public function.
- Characterization: engine validation matrix, recorder start/stop ordering, stop-error joining, variable copy isolation, root error propagation, and cancellation cleanup.
- Contract: existing `RunProgram(ctx, program, Config)` behavior and errors remain compatible.
- Rollback: public `RunProgram` can switch back to the current inline implementation; coordinator is additive and has no external state.

### Slice 2 — Healing port at execution boundary

- Scope: hide concrete healer/policy coupling behind a node-local port and adapter.
- Characterization: healing disabled, no candidate, safety rejection, successful healing, delayed success audit, repeat overlay, and cancellation.
- Rollback: adapter delegates to current `heal.Healer`.

### Slice 3 — Fingerprint classification service

- Scope: isolate framework detection/classification and keep stable fingerprint value objects.
- Characterization: known frameworks, unknown framework, malformed evidence, and deterministic confidence/order.
- Rollback: old detection path remains default.

### Slice 4 — Workspace evidence adapter

- Scope: translate execution/healing facts to workspace evidence without importing workspace into node/heal.
- Characterization: node/spec ID mapping, candidate evidence, sample retention, and duplicate/idempotency behavior.
- Rollback: existing sinks remain supported.

## First-slice implementation guardrails

- Do not modify node execution semantics or browser adapters.
- Keep `RunProgram` public and the legacy path available.
- Add characterization tests before implementation (G2).
- Run `go test -race ./...` and `go vet ./...` after each implementation increment.
- Stop and re-plan if recorder lifecycle or cancellation behavior differs.

## G1 assessment

Slice order, rollback paths, and characterization scopes are defined. The first slice is independently deliverable and keeps the legacy path runnable. Proceed to G2 characterization only; do not implement until tests are added and passing against current behavior.
