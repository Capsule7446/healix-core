# C12 — 中置信自愈（BELOW_CAP）

> 来源：Healix 仓库 `docs/refactor/healix-core-v0.3.0-replacement-assessment.md` 对应 Case；本清单以该评估要求为输入，并以 healix-core 当前 `master` 源码重新核验。

## 状态

**当前：当前 master 已比 v0.3.0 前进，可在 safety ALLOW 后 run-local 使用；治理闭环仍缺失。**

## 业务不变量

BELOW_CAP 可恢复当前 Run，但正式资产必须人工审核；runtime safety 与 promotion governance 是两个独立门槛。

## 当前证据

- `domain/heal/heal.go`：BelowCap/NeedsReview
- `domain/heal/assessment.go`：安全决策
- `domain/node/step.go`：run-local overlay
- `application/automation/heal_review_service.go`：审核已有候选

## 调整清单

- [x] BELOW_CAP 表达 NeedsReview。
- [x] safety ALLOW 时可用于当前 Run。
- [x] 明确 BELOW_CAP 永不自动发布。
- [x] band-aware threshold disposition。
- [x] 三次成功生成/转为 AwaitingApproval。
- [x] review evidence 保存 selector/fingerprint/score/context/samples。
- [x] 拒绝后同 hash 是否抑制或重新观察需定规则。
- [x] Runtime NeedsReview 不得被解释为 approval。

## 测试与验收

- [x] BELOW_CAP 可恢复 current Run 且不变更资产。
- [x] safety BLOCK 时失败。
- [x] 三次成功只 AwaitingApproval，不发布。
- [x] 更多成功仍不得绕过审核。
- [x] approval 后最多发布一个版本。

## 依赖与风险

依赖 C11 及 runtime evidence 基础；C12 的合格事实进入 C14 成熟流程，再由 C15 审核。最大风险是泛化 `Promote bool` 导致误自动发布。

## 审核

- [x] 批准“当前 Run 可用、资产需审核”
- [x] 修改：________________
