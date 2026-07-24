# C22 — Abort active Run

> 来源：Healix 仓库 `docs/refactor/healix-core-v0.3.0-replacement-assessment.md` 对应 Case；本清单以该评估要求为输入，并以 healix-core 当前 `master` 源码重新核验。

## 状态

**当前：状态存在；“先提交 ABORTED，再取消 context”的应用编排缺失。**

## 业务不变量

Active abort 必须先原子提交 ABORTED 并失效 worker fence，成功后才发送取消信号；提交失败不得假装 abort 成功。

## 当前证据

- `domain/execution/run.go`：Running → Aborted
- `domain/execution/status.go`：ExecutionAborted
- `application/scheduling/decision.go`：abort stops later entries
- `application/engine/coordinator.go`：普通 context cancel 当前映射 CANCELED

## 调整清单

- [x] 新增 `AbortActiveRunService`。
- [x] `CommitAbort` 端口原子更新 Run/entry/fence/revision。
- [x] commit 成功后调用 `ActiveRunSignaler`。
- [x] commit 成功、signal 失败返回可重试结果，不回滚 ABORTED。
- [x] stale worker 后续 writes 全部拒绝。
- [x] 区分 administrative abort 与普通 context cancellation。
- [x] 支持 cross-process durable signal/outbox，registry 留 Host。
- [x] command 幂等和 natural completion race 规则。

## 测试与验收

- [x] 严格验证 commit-before-cancel 调用顺序。
- [x] commit failure 不调用 cancel。
- [x] signal failure 可安全重试。
- [x] old fence 不能终态提交。
- [x] abort/complete race 仅一个 terminal winner。

## 依赖与风险

依赖 C20 fencing；存储已 ABORTED 到进程实际停止之间的窗口必须靠 fence 保证安全。

## 审核

- [x] 批准 commit-before-signal
- [x] 修改：________________
