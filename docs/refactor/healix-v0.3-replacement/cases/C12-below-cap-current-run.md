# C12 — 中置信自愈（BELOW_CAP）

> 来源：历史替换评估中的同编号案例；本清单以该评估要求为输入，并以 healix-core 当前实现（`12d1ba2`）重新核验。

## 状态

**当前结果：已由 v0.3 替换实现覆盖；以下证据、清单与验收项按当前模型解释。**

## 业务不变量

BELOW_CAP 可恢复当前执行实例，但正式资产必须人工审核；运行时安全 与晋升 governance 是两个独立门槛。

## 当前证据

- `domain/heal/heal.go`：BelowCap/NeedsReview
- `domain/heal/assessment.go`：安全决策
- `domain/node/step.go`：执行实例局部覆盖层
- `application/automation/heal_review_service.go`：审核已有候选

## 调整清单

- [x] BELOW_CAP 表达 NeedsReview。
- [x] safety ALLOW 时可用于当前执行实例。
- [x] 明确 BELOW_CAP 永不自动发布。
- [x] 感知分档的阈值处置。
- [x] 三次成功生成/转为 AwaitingApproval。
- [x] review evidence 保存 selector/fingerprint/score/context/samples。
- [x] 拒绝后同 hash 是否抑制或重新观察需定规则。
- [x] 运行时 NeedsReview 不得被解释为 approval。

## 测试与验收

- [x] BELOW_CAP 可恢复 current 执行实例且不变更资产。
- [x] safety BLOCK 时失败。
- [x] 三次成功只 AwaitingApproval，不发布。
- [x] 更多成功仍不得绕过审核。
- [x] approval 后最多发布一个版本。

## 依赖与风险

依赖 C11 及 runtime evidence 基础；C12 的合格事实进入 C14 成熟流程，再由 C15 审核。最大风险是泛化 `Promote bool` 导致误自动发布。

## 审核

- [x] 批准“当前执行实例可用、资产需审核”
- [x] 修改：________________
