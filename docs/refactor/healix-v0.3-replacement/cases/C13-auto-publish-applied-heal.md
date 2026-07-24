# C13 — APPLIED 三次成功自动发布

> 来源：Healix 仓库 `docs/refactor/healix-core-v0.3.0-replacement-assessment.md` 对应 Case；本清单以该评估要求为输入，并以 healix-core 当前 `master` 源码重新核验。

## 状态

**当前：规则和发布构件存在；原子应用服务缺失。**

## 业务不变量

同 node/base/hash 的 APPLIED 候选，经三个不同 Run 连续成功后，恰好发布一个新 NodeVersion，并原子推进 current、标记 promoted、关闭 streak。

## 当前证据

- `domain/automation/healing.go`：`HealStreak.Observe`
- `domain/automation/versioning.go`：Node version publication
- `application/automation/heal_review_service.go`：原子 commit intent 范例

## 调整清单

- [x] 引入 band-aware observation/promotion service。
- [x] 只接受三个 distinct successful Run IDs。
- [x] 第三次时重新校验 current base version。
- [x] candidate/streak/node expected revisions 全部纳入 commit。
- [x] 原子创建 version、更新 current、promote 当前 candidate、close 当前 streak，并使该 Node 下所有其他旧-base 活跃 candidate/streak/pending review 失效。
- [x] 失效只作用于活跃治理状态；历史 observations、已 promoted/rejected candidate 和审核记录保留。
- [x] promotion idempotency key=`node/base/hash/disposition`。
- [x] duplicate/concurrent/fourth success 返回已有 promotion。
- [x] outbox 与发布同事务。

## 测试与验收

- [x] 第 1/2 次不发布，第 3 次只发布一次。
- [x] 重复第 3 次和并发阈值只产生一个版本。
- [x] base/hash 变化不发布旧候选；Node current 推进后旧-base 活跃 candidate/streak/pending review 不再出现在活跃集合且不能继续操作。
- [x] 旧-base observations 与终结治理记录仍可审计。
- [x] 任一步写失败全回滚。
- [x] failed Run 不计数。

## 依赖与风险

依赖 C11/C23；这是最严格的跨聚合原子性和并发热点。

## 审核

- [x] 批准
- [x] 修改：________________
