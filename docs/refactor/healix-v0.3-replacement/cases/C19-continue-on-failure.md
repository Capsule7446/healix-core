# C19 — 失败后继续

> 来源：历史替换评估中的同编号案例；本清单以该评估要求为输入，并以 healix-core 当前实现（`12d1ba2`）重新核验。

## 状态

**当前结果：已由 v0.3 替换实现覆盖；以下证据、清单与验收项按当前模型解释。**

## 业务不变量

CONTINUE_ON_FAILURE 下失败后继续下一个 pending entry；但最终执行实例若有失败仍为 FAILED。Cancel/Abort 无论 policy 都停止。

## 当前证据

- `domain/execution/plan.go`：ContinueOnFailure
- `application/scheduling/decision.go`：policy matrix 与 final status
- `application/scheduling/decision_test.go`

## 调整清单

- [x] failure 可继续。
- [x] cancel/abort 始终停止。
- [x] 有失败的最终执行实例为 FAILED。
- [x] 固定新模型默认 policy 为 STOP_ON_FAILURE。
- [x] 不为旧测试任务保留特殊映射或兼容分支。
- [x] ApplyDecision 事务与幂等 conformance test。
- [x] UI/宿主读模型展示“部分失败但继续”的状态事实。

## 测试与验收

- [x] first failure 后选择 second。
- [x] final failure 后执行实例 FAILED。
- [x] cancel/abort 不继续。
- [x] 同一 decision 重放不重复 transition。
- [x] 默认值为 STOP_ON_FAILURE，不存在 legacy-only 行为分支。

## 依赖与风险

属于明确的新产品语义；直接以新模型和 STOP_ON_FAILURE 默认值替换旧行为，不维护历史数据兼容分支。

## 审核

- [x] 批准支持，默认仍 STOP
- [x] 其他默认：________________
