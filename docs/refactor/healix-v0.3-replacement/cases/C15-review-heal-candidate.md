# C15 — 批准/拒绝自愈候选

> 来源：历史替换评估中的同编号案例；本清单以该评估要求为输入，并以 healix-core 当前实现（`12d1ba2`）重新核验。

## 状态

**当前结果：已由 v0.3 替换实现覆盖；以下证据、清单与验收项按当前模型解释。**

## 业务不变量

审核必须锁定 node ID、base version ID、candidate hash；approval 原子发布版本并标记 promoted，rejection 不发布。

## 当前证据

- `domain/automation/healing.go`：候选项与审核的命令和状态
- `application/automation/heal_review_service.go`：Approve/Reject
- `application/automation/heal_candidate_repository.go`：提交 contracts

## 调整清单

- [x] 命令 triple 校验。
- [x] awaiting 状态、reviewer authorization、candidate verification。
- [x] base current 与 node/candidate 修订号校验。
- [x] 节点 current 推进后，旧-base pending review 自动失效并退出待审核列表；Approve/Reject 均返回明确 stale conflict。
- [x] 失效不删除 candidate、观察事实、review audit history。
- [x] 确认 hash 全局唯一，否则按 composite key Load。
- [x] same approval retry 不创建第二版本。
- [x] 同一 `NodeID + BaseNodeVersionID + CandidateHash` 被拒绝后，只要 base 仍为 current，就不得重新建立候选或累计 streak；节点 current 更新后可按新 base 重新观察。
- [x] approve/reject race 有确定类型化冲突。
- [x] `CommitApproval` 适配器一致性测试。
- [x] verifier 重新计算 canonical hash。
- [x] reviewer authorization 明确 resource scope。
- [x] 发布 outbox 同事务。

## 测试与验收

- [x] node/base/hash 任一不匹配均拒绝。
- [x] stale base/revision 均不发布；旧-base review 不再出现在 active/pending query。
- [x] stale 后历史候选、观察事实和审核轨迹仍可查询。
- [x] duplicate approval 幂等。
- [x] rejected candidate 在同一 base 下重现时保持抑制；新 base 下可重新观察。
- [x] approve/reject race 只有一个终态。
- [x] 任一写失败 candidate/node 均回滚。

## 依赖与风险

依赖 C14；当前最大风险是接口承诺原子性但宿主实现未证明。

## 审核

- [x] 批准并要求 适配器一致性要求
- [x] 修改：________________
