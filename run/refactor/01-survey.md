# 01 Survey

## Runtime entry points

- `application/engine.RunProgram` validates run inputs, copies variables into a scratchpad, constructs `domain/node.Runtime`, owns recorder lifecycle, then invokes `Program.Root.Run`.
- `application/engine.Compiler` builds program trees and indexes fingerprint specs.
- `domain/node.Runtime` coordinates driver, healing, recording, evidence, pacing, retry, overlay, and validation.

## Domain vocabulary and contexts

- **Execution** (`domain/node`): Program, Node, StepExecution, Runtime, Driver, Element, phase transitions, retry, validation, and observations.
- **Fingerprint** (`domain/fingerprint`): NodeSpec, selectors, framework/detection metadata, normalization and matching.
- **Healing** (`domain/heal`): policy, candidate evidence/sample, framework scoring, ranking, safety and review decisions.
- **Workspace** (`domain/automation`): workflow/test task models, rules, assets, evidence, versioning, sampling, environment and persistence-facing ports.
- **Application orchestration** (`application/engine`): compilation and run lifecycle.

## Dependency graph

- `application/engine` depends on `node` and `heal`.
- `node` imports `fingerprint` and `heal`, and owns browser and execution-sink interfaces.
- `heal` uses fingerprint concepts for selector evidence and scoring.
- `workspace` supplies models and evidence but is not yet a clean execution adapter boundary.
- Tests are broad and matrix-heavy across engine, node, heal, fingerprint, and automation.

## Invariants and side effects

- RunID, Driver, and Program.Root are required.
- Step phase transitions are explicitly guarded; terminal states must not leak across repeats.
- Specs/tree remain stable while selector overlays are run-local.
- Recorder start/stop is lifecycle-managed with a detached cleanup timeout.
- Driver operations, browser navigation/snapshot, sink records, heal decisions/samples, and operation observations are side effects.
- Context cancellation and timeout behavior are business-significant.

## Behavior black boxes requiring characterization

- RunProgram validation and recorder lifecycle/error joining.
- Runtime phase/event ordering, cancellation, retry, pacing, selector overlay and repeated execution.
- Healing candidate ordering, safety thresholds, framework scoring, and audit/sample timing.
- Fingerprint detection and framework classification for supported/unknown pages.
- Evidence mapping and candidate evidence persistence.

## Bad smells

- `node.Runtime` is a large orchestration aggregate with many ports and cross-context imports.
- `node` owns healing and fingerprint-facing contracts, causing domain coupling and broad interfaces.
- `application/engine.RunProgram` both composes dependencies and owns lifecycle policy.
- Evidence concepts are shared through execution sinks rather than a narrow adapter.
- New uncommitted files indicate in-progress extraction but no explicit strangler boundary yet.

## Candidate seams

1. Runtime lifecycle/execution coordinator around `RunProgram` and `Runtime` construction.
2. Healing policy/decision seam around `domain/heal` contracts and step healing.
3. Fingerprint detection/framework seam around the newly added types.
4. Evidence projection seam from node/heal facts into evidence.

## G0 assessment

The current model explains key execution and healing behavior and identifies black boxes. Proceed to seam analysis; no production implementation should start before characterization tests are recorded.
