# C21 — 取消排队中的执行实例

> 来源：历史替换评估中的同编号案例；本清单以该评估要求为输入，并以 healix-core 当前实现（`12d1ba2`）重新核验。

## 状态

**当前结果：已由 v0.3 替换实现覆盖；以下证据、清单与验收项按当前模型解释。**

## 业务不变量

`CancelRunService` 处理 `QUEUED` 与 `RUNNING` 两个分支。`CancelRunCommand.ExpectedStatus` 只接受这两个状态：取消 `QUEUED` 执行实例时，宿主事务必须原子提交 `CANCELED` 并移除队列/领取资格，不发送活动取消信号；取消 `RUNNING` 执行实例时，宿主事务必须先提交 `CANCELED` 并失效栅栏，再要求发送活动取消信号。取消与并发领取执行权只能有一个赢家。

## 当前证据

- `application/scheduling/run_command_services.go`：`CancelRunCommand`、`RunCommandStore`、`RunCancellationSignaler` 与 `CancelRunService`
- `application/scheduling/run_command_services_test.go`：排队与运行中取消分支、命令校验、幂等重放及信号语义
- `application/scheduling/run_command_transaction_conformance_test.go`：取消/领取竞争及宿主原子事务契约
- `domain/execution/run.go`：`QUEUED → CANCELED` 与 `RUNNING → CANCELED`

## 调整清单

- [x] 使用通用 `CancelRunService`，不另设 `CancelQueuedRunService`。
- [x] 命令包含执行实例 ID、预期修订号/状态、命令 ID 与可信时间；不存在 actor/reason 字段。
- [x] `ExpectedStatus` 仅接受 `QUEUED` 或 `RUNNING`。
- [x] 排队分支原子更新状态、队列成员关系与领取资格，且不要求活动取消信号。
- [x] 运行中分支原子提交 `CANCELED` 并失效栅栏，随后要求活动取消信号。
- [x] 定义幂等重放、状态冲突与修订冲突。
- [x] 适配器通过 cancel/claim race conformance test。

## 测试与验收

- [x] 已取消的排队执行实例永不被 `ClaimNext` 返回。
- [x] cancel/claim race 仅一个成功。
- [x] `QUEUED` 取消提交后不发送活动信号。
- [x] `RUNNING` 取消提交后必须发送活动信号；信号失败返回包含已提交结果的 `ErrRunSignalRetryable`。
- [x] 同命令重放幂等。
- [x] queue order 保持有效。

## 依赖与风险

依赖 C20；status 与 queue membership 分开写会领取执行权已取消执行实例。

## 审核

- [x] 批准
- [x] 修改：________________
