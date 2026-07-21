# 09 领域与读侧解耦

当前 Core 具有写侧/执行模型以及面向读取的结果结构和端口，但没有已实现的 CQRS 读模型存储。

## 真源

- Workflow and task definitions/plans are planning-context truth.
- Runtime phase, validation, healing and operation facts are execution-time truth while a run is active.
- Workspace evidence records are durable candidates only when a host adapter persists them.
- Framework stacks and fingerprints are target identity metadata supplied by the browser adapter and stored only if the host chooses to persist them.

## 对读侧的含义

Queries should consume projected execution/workspace facts rather than Runtime or mutable Program state. Core currently exposes contracts (`WorkflowReader`, `TestTaskReader`, `WorkflowQueryResult`, `TestTaskQueryResult`) but does not specify projection ownership or freshness.
