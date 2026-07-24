# C18 — 失败即停止

> 来源：历史替换评估中的同编号案例；本清单以该评估要求为输入，并以 healix-core 当前实现（`12d1ba2`）重新核验。

## 状态

**当前结果：已由 v0.3 替换实现覆盖；以下证据、清单与验收项按当前模型解释。**

## 业务不变量

STOP_ON_FAILURE 下首个失败使所有后续 pending entry 以 PRIOR_FAILURE 跳过，执行实例同事务终态 FAILED。

## 当前证据

- `application/scheduling/decision.go`：`stopFor`、`stopAfter`
- `domain/execution/plan.go`：FailurePolicy
- `application/scheduling/coordinator.go`：ApplyDecision port

## 调整清单

- [x] failure/cancel/abort 各有 distinct skip cause。
- [x] 后续 entry transitions 一次性由 decision 返回。
- [x] skips + final run status 同事务。
- [x] 对执行实例与执行项修订号执行 CAS。
- [x] decision replay 幂等。
- [x] stop 提交期间不得领取执行权 later entry。
- [x] 持有失效租约的工作器不得覆盖 skipped 状态。
- [x] 适配器崩溃与故障注入测试。

## 测试与验收

- [x] first/middle failure 只 skip 正确后继。
- [x] 同事务设置 run FAILED。
- [x] concurrent 领取执行权与 stop 提交不启动后继。
- [x] rollback 保持原状态。
- [x] CONTINUE_ON_FAILURE 不受影响。

## 依赖与风险

依赖 C17；分开写 skips 与执行实例 final 会暴露可被领取执行权的非法中间态。

## 审核

- [x] 批准
- [x] 修改：________________
