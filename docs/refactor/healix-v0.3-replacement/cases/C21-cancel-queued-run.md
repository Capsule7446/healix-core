# C21 — Cancel queued Run

> 来源：Healix 仓库 `docs/refactor/healix-core-v0.3.0-replacement-assessment.md` 对应 Case；本清单以该评估要求为输入，并以 healix-core 当前 `master` 源码重新核验。

## 状态

**当前：状态规则和端口存在；应用命令闭环缺失。**

## 业务不变量

只有 QUEUED 可直接 cancel；cancel 必须原子移出 claim eligibility，与 concurrent claim 只有一个赢家。

## 当前证据

- `domain/execution/run.go`：Queued → Canceled
- `application/scheduling/ports.go`：`CancelRun`

## 调整清单

- [x] 新增 `CancelQueuedRunService`。
- [x] command 包含 run ID、expected revision/status、command ID、trusted time。
- [x] 原子更新 status、queue membership 与 fence eligibility。
- [x] 定义 already canceled replay、already running、terminal conflict。
- [x] actor/reason 由可信入站边界提供。
- [x] 适配器实现 cancel/claim race conformance test。

## 测试与验收

- [x] canceled queued Run 永不被 ClaimNext 返回。
- [x] cancel/claim race 仅一个成功。
- [x] running Run 被 queued-cancel 拒绝。
- [x] 同 command 重放幂等。
- [x] queue order 保持有效。

## 依赖与风险

依赖 C20；status 与 queue membership 分开写会 claim 已取消 Run。

## 审核

- [x] 批准
- [x] 修改：________________
