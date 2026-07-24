# C08 — Workflow 编译执行

> 来源：Healix 仓库 `docs/refactor/healix-core-v0.3.0-replacement-assessment.md` 对应 Case；本清单以该评估要求为输入，并以 healix-core 当前 `master` 源码重新核验。

## 状态

**当前：核心能力已实现；需接入完整 RunSnapshot 与 typed scopes。**

## 业务不变量

Action、Wait、Repeat、WorkflowRef 只从 sealed exact-version graph 编译；Compiler/Worker 不查询 current assets。

## 当前证据

- `domain/execution/plan.go`：exact snapshots 与 seal
- `domain/execution/validation.go`：graph/budget 校验
- `application/engine/compiler.go`：全部 step 编译
- `domain/node`：program/runtime 执行

## 调整清单

- [x] exact workflow/node/reference IDs。
- [x] recursive WorkflowRef/Repeat 编译。
- [x] graph depth/count/wait/expansion budget。
- [x] deterministic invocation IDs。
- [x] 输入改为 durable RunSnapshot。
- [x] C03 materialize nested latest exact IDs。
- [x] C04–C06 typed scopes 接入 compiler。
- [x] 明确无 repository lookup 的契约测试。
- [x] 提供跨 browser adapter conformance suite。

## 测试与验收

- [x] 所有 action/wait kind 通过矩阵。
- [x] missing/mismatched exact dependency 在浏览器调用前失败。
- [x] 同 child version 两次调用 runtime IDs 唯一且稳定。
- [x] source mutation 不影响 sealed/compiled program。
- [x] 同一 frozen Run 重编译结构等价。

## 依赖与风险

依赖 C02–C06；runtime ID 若已持久化，格式变化需迁移。

## 审核

- [x] 批准保留现有设计并扩展
- [x] 修改：________________
