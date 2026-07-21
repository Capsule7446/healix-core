# 01 代码盘点

## 一句话系统定位

Healix Core models and executes browser workflows from immutable workspace plans, attempts safe selector healing when UI drift breaks execution, and emits replay/audit evidence for host adapters.

## 入口地图

- `application/engine.CompileExecution` (`application/engine/compiler.go:40`) transforms workspace workflow/test-task plans into an executable `node.Program` plus indexed `fingerprint.NodeSpec` values.
- `application/engine.RunProgram` (`application/engine/engine.go:31`) is the execution entry point: validates inputs, snapshots variables, constructs `node.Runtime`, manages Recorder lifecycle, then runs the root node.
- `node.Program.Root.Run` dispatches the composite execution tree; `WorkflowNode` and `WorkflowCallNode` model workflow composition (`domain/node/composite.go`).
- Host browser adapters implement `node.Driver` and `node.Element`; Core has no browser SDK dependency.
- `domain/fingerprint.DetectFrameworks` (`domain/fingerprint/detection.go:27`) consumes sanitized `PageObservation` supplied by the host adapter.
- Workspace ports (`domain/workspace/ports.go`) define persistence-facing readers/writers; this repository contains contracts and models, not a concrete API/controller.

## Core behavior

- A run is a sequential stateful execution over a compiled tree. `Runtime` carries run-local variables, selector overlays, retry/pacing policy, browser ports, healing ports, and fact sinks.
- Each step uses selectors first, then optional healing from a browser snapshot. Healing is assessed for safety before a selector overlay is installed.
- Validation nodes can poll and emit validation observations; cancellation, timeout, system error, and sensitive actual values are distinct behaviors.
- Execution emits phase events, healing decisions, operation observations, and deterministic candidate samples through ports.

## 主要行为黑盒

- Host adapter mapping from browser DOM/framework signals to `PageObservation`, `NodeSpec`, `Element`, and `DOMSnapshot`.
- Concrete persistence and API projections behind workspace ports.
- Whether emitted facts are stored transactionally, streamed, or queried through a separate read model.

## G0 门禁

Entrypoints, core structures, invariants, side effects, tests, and unknown adapters are identified. The repository is a domain/application core rather than a 完成 HTTP application.
