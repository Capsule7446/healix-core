# C18 — 失败即停止

> 来源：Healix 仓库 `docs/refactor/healix-core-v0.3.0-replacement-assessment.md` 对应 Case；本清单以该评估要求为输入，并以 healix-core 当前 `master` 源码重新核验。

## 状态

**当前：决策层已实现；skip/final 原子提交需 Host 证明。**

## 业务不变量

STOP_ON_FAILURE 下首个失败使所有后续 pending entry 以 PRIOR_FAILURE 跳过，Run 同事务终态 FAILED。

## 当前证据

- `application/scheduling/decision.go`：`stopFor`、`stopAfter`
- `domain/execution/plan.go`：FailurePolicy
- `application/scheduling/coordinator.go`：ApplyDecision port

## 调整清单

- [x] failure/cancel/abort 各有 distinct skip cause。
- [x] 后续 entry transitions 一次性由 decision 返回。
- [x] skips + final run status 同事务。
- [x] CAS run/entry revisions。
- [x] decision replay 幂等。
- [x] stop commit 期间不得 claim later entry。
- [x] stale worker 不得覆盖 skipped 状态。
- [x] adapter crash/fault injection tests。

## 测试与验收

- [x] first/middle failure 只 skip 正确后继。
- [x] 同事务设置 run FAILED。
- [x] concurrent claim 与 stop commit 不启动后继。
- [x] rollback 保持原状态。
- [x] CONTINUE_ON_FAILURE 不受影响。

## 依赖与风险

依赖 C17；分开写 skips 与 Run final 会暴露可被 claim 的非法中间态。

## 审核

- [x] 批准
- [x] 修改：________________
