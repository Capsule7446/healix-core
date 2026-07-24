# C14 — BELOW_CAP 三次成功进入人工审核

> 来源：Healix 仓库 `docs/refactor/healix-core-v0.3.0-replacement-assessment.md` 对应 Case；本清单以该评估要求为输入，并以 healix-core 当前 `master` 源码重新核验。

## 状态

**当前：状态与审核下游存在；候选成熟编排缺失。**

## 业务不变量

同 node/base/hash 的 BELOW_CAP 三次合格成功后，原子进入 `AWAITING_APPROVAL/PENDING`，不得改变 Node current version。

## 当前证据

- `domain/automation/healing.go`：BelowCap、AwaitingApproval、ApprovalPending
- `application/automation/heal_review_service.go`：消费 awaiting candidate

## 调整清单

- [x] threshold disposition 明确为 AwaitApproval。
- [x] 增加 candidate 状态转换规则。
- [x] 第三次 observation + streak + candidate transition 同事务。
- [x] 按 node/base/hash/band 唯一，避免重复 review item。
- [x] 保存 contributing Run IDs 和 evidence summary。
- [x] base 变化后，旧-base awaiting candidate、streak 和 pending review 自动转为 `STALE/CANCELED` 并退出活跃查询；历史 evidence/review audit 保留。
- [x] 所有成熟、通知和审核入口先校验 `BaseNodeVersionID == CurrentVersionID`。
- [x] reviewer notification 使用 transactional outbox。
- [x] 明确 original recovery 对 awaiting candidate 的处理。

## 测试与验收

- [x] 1/2 次保持 observing。
- [x] 第 3 次产生一个 awaiting candidate。
- [x] Node current 不变；若由其他发布流程推进，旧-base 待审项立即从活跃集合失效且不可批准。
- [x] 旧-base observation 和审核历史不被物理删除。
- [x] duplicate/concurrent 第 3 次不重复。
- [x] BELOW_CAP 绝不 auto-publish。
- [x] 事务失败 count/candidate 均不变化。

## 依赖与风险

依赖 C11/C12，直接供 C15 使用；状态与计数分离写会造成“成熟但无审核项”。

## 审核

- [x] 批准
- [x] 修改：________________
