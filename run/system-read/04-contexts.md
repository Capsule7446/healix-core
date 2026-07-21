# 04 上下文

## 规划上下文

`domain/workspace` owns workflow/test-task definitions, versions, dependency snapshots, run plans, and lifecycle transition rules. It is the source of what should run.

## 执行上下文

`domain/node` owns executable tree behavior, runtime state, phase transitions, browser ports, retries, validation, and run-local selector overlays. It is the source of what happened during a run at execution time.

## 指纹上下文

`domain/fingerprint` owns NodeSpec, Selector, Fingerprint and framework metadata. It is the shared language for identifying a browser target, not a browser adapter.

## 自愈上下文

`domain/heal` owns candidate evaluation, deterministic scoring, sample ordering, safety assessment, and review decisions. It consumes fingerprint values and execution context but should not know persistence.

## 证据上下文

`domain/workspace/evidence.go` and `execution_facts.go` define projections for validation/healing/run evidence. Conceptually this is a reporting/audit context; physically it currently shares workspace package types and ports.

## 应用上下文

`application/engine` coordinates compilation and run lifecycle. It is the composition root between planning, execution, and host-provided ports.

## Boundary observations

The strongest current boundaries are interfaces: `node.Driver`, `node.ExecutionSink`, `heal.Healer`, workspace readers/writers, and fingerprint detector interfaces. Package imports still couple node to heal and fingerprint, so physical boundaries are weaker than conceptual boundaries.
