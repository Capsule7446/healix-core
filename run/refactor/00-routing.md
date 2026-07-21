# 00 Routing

## Route

- Existing runnable Go code: yes.
- Target is decoupling, module splitting, and testability: yes.
- Selected workflow: brownfield DDD strangler refactor.

## Constraints

- Preserve current uncommitted browser-execution refactor as baseline.
- Do not rewrite the system wholesale.
- Keep legacy entry paths runnable and rollbackable.
- Use Go and existing test suite.

## Initial slice candidates

1. Separate execution orchestration from runtime construction and lifecycle.
2. Isolate healing decision/scoring from node execution.
3. Isolate fingerprint detection/framework classification from healing.
4. Isolate workspace evidence collection from execution.

## Workflow state

- G0: pending survey.
- G1: pending seam and strangler plan.
- G2/G3: no implementation slice started.
