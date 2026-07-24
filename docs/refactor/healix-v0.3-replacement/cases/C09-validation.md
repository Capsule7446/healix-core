# C09 — Validation

> 来源：Healix 仓库 `docs/refactor/healix-core-v0.3.0-replacement-assessment.md` 对应 Case；本清单以该评估要求为输入，并以 healix-core 当前 `master` 源码重新核验。

## 状态

**当前：稳定窗口和 OR-of-AND 已实现；group evidence 可增强。**

## 业务不变量

Validation 必须在 MaxWait 内连续满足 Stability；ValidationGroup 为 branch 间 OR、branch 内 AND，并产生可重建结果的稳定证据。

## 当前证据

- `domain/automation/workflow_validation.go`：定义与约束
- `domain/node/validation.go`：stable polling、group 逻辑、observations
- `application/engine/compiler.go`：validation/group compile
- `domain/evidence/observations.go`：progress/final evidence

## 调整清单

- [x] continuous stability window。
- [x] OR branches / AND members。
- [x] progress/final evidence。
- [x] collection-valued expected/actual evidence 保留值边界和顺序语义。
- [x] group-level terminal observation 显式记录 winning BranchID 与 terminal reason。
- [x] 定义 non-winning branch final evidence 语义。

## 测试与验收

- [x] 失败 poll 会重置 stability。
- [x] 不同 poll 的成员交错成功不能误判 AND。
- [x] 不同 branch 交错成功不能误判稳定 branch。
- [x] evidence 可重建与 runtime 相同的 group outcome。
- [x] timeout/cancel/system error 有终态解释。

## 依赖与风险

显式 group evidence 是 schema change；集合值必须保持类型与元素边界，避免退化为拼接字符串。

## 审核

- [x] 批准增量增强
- [x] 修改：________________
