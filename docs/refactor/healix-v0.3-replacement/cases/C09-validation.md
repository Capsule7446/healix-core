# C09 — 校验

> 来源：历史替换评估中的同编号案例；本清单以该评估要求为输入，并以 healix-core 当前实现（`12d1ba2`）重新核验。

## 状态

**当前结果：已由 v0.3 替换实现覆盖；以下证据、清单与验收项按当前模型解释。**

## 业务不变量

校验必须在 MaxWait 内连续满足 Stability；ValidationGroup 为 branch 间 OR、branch 内 AND，并产生可重建结果的稳定证据。

## 当前证据

- `domain/automation/workflow_validation.go`：定义与约束
- `domain/node/validation.go`：stable polling、group 逻辑、observations
- `application/engine/compiler.go`：校验与分组编译
- `domain/evidence/observations.go`：过程证据与最终证据

## 调整清单

- [x] 连续稳定窗口。
- [x] OR 分支 / AND 成员。
- [x] 过程证据与最终证据。
- [x] collection-valued expected/actual evidence 保留值边界和顺序语义。
- [x] group-level 终态观察事实显式记录 winning BranchID 与终态 reason。
- [x] 定义 未胜出分支的最终证据 语义。

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
