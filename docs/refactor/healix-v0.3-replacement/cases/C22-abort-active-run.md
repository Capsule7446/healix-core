# C22 — 中止活动执行实例

> 来源：历史替换评估中的同编号案例；本清单以该评估要求为输入，并以 healix-core 当前实现（`12d1ba2`）重新核验。

## 状态

**当前结果：已实现；显式中止与普通执行上下文取消具有明确且独立的终态语义。**

## 业务不变量

活动执行中止必须先由宿主事务原子提交权威的 `execution.Aborted` 并失效工作器栅栏，成功后才发送取消信号；提交失败不得发送信号或假装中止成功。信号失败不回滚已提交结果，而是返回 `ErrRunSignalRetryable`。普通执行上下文取消仍映射为 `CANCELED`，属于不同操作。

## 当前证据

- `domain/execution/run.go`：Running → Aborted
- `domain/execution/status.go`：ExecutionAborted
- `application/scheduling/run_command_services.go`：`AbortRunService` 校验权威 `execution.Aborted` 结果，提交后发送取消信号，并以 `ErrRunSignalRetryable` 表示可重试的信号失败
- `application/engine/coordinator.go`：普通执行上下文取消映射为 `CANCELED`
- `application/scheduling/run_command_services_test.go`：服务顺序、结果校验与信号失败测试
- `application/scheduling/run_command_transaction_conformance_test.go`：宿主事务原子性、竞态与栅栏一致性验收

## 调整清单

- [x] `AbortRunService` 明确承载显式中止用例。
- [x] `RunCommandStore.Abort` 原子更新 Run/entry/fence/revision，并返回权威提交结果。
- [x] 提交成功后调用 `RunCancellationSignaler`。
- [x] 提交成功、信号失败返回含已提交结果的 `ErrRunSignalRetryable`，不回滚 `ABORTED`。
- [x] 持有失效租约的工作器后续写入全部拒绝。
- [x] 管理性显式中止为 `ABORTED`，普通执行上下文取消为 `CANCELED`。
- [ ] 跨进程持久信号/发件箱由宿主按部署需要提供。
- [x] 命令幂等和自然完成竞态规则。

## 测试与验收

- [x] 严格验证先提交后取消调用顺序。
- [x] 提交失败不调用取消。
- [x] 信号失败保留提交结果并可安全重试。
- [x] 旧栅栏不能终态提交。
- [x] 中止/完成竞态仅一个终态胜者。

## 依赖与风险

依赖 C20 栅栏校验；存储已 ABORTED 到进程实际停止之间的窗口必须靠栅栏保证安全。

## 审核

- [x] 批准先提交后发信号
- [x] 修改：________________
