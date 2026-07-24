# C13 — APPLIED 三次成功自动发布

> 来源：历史替换评估中的同编号案例；本清单以该评估要求为输入，并以 healix-core 当前实现（`12d1ba2`）重新核验。

## 状态

**当前结果：已由 v0.3 替换实现覆盖；以下证据、清单与验收项按当前模型解释。**

## 业务不变量

同 node/base/hash 的 APPLIED 候选，经三个不同执行实例连续成功后，恰好发布一个新 NodeVersion，并原子推进 current、标记 promoted、关闭 streak。

## 当前证据

- `domain/automation/healing.go`：`HealStreak.Observe`
- `domain/automation/versioning.go`：节点 version 发布
- `application/automation/heal_review_service.go`：原子提交 intent 范例

## 调整清单

- [x] 引入 感知分档的观测与晋升服务。
- [x] 只接受三个 distinct successful 执行实例 IDs。
- [x] 第三次时重新校验 current base version。
- [x] candidate/streak/node expected revisions 全部纳入提交。
- [x] 原子创建 version、更新 current、promote 当前 candidate、close 当前 streak，并使该节点下所有其他旧-base 活跃 candidate/streak/pending review 失效。
- [x] 失效只作用于活跃治理状态；历史 observations、已 promoted/rejected candidate 和审核记录保留。
- [x] 晋升 idempotency key=`node/base/hash/disposition`。
- [x] duplicate/concurrent/fourth success 返回已有晋升。
- [x] outbox 与发布同事务。

## 测试与验收

- [x] 第 1/2 次不发布，第 3 次只发布一次。
- [x] 重复第 3 次和并发阈值只产生一个版本。
- [x] base/hash 变化不发布旧候选；节点 current 推进后旧-base 活跃 candidate/streak/pending review 不再出现在活跃集合且不能继续操作。
- [x] 旧-base observations 与终结治理记录仍可审计。
- [x] 任一步写失败全回滚。
- [x] failed 执行实例不计数。

## 依赖与风险

依赖 C11/C23；这是最严格的跨聚合原子性和并发热点。

## 审核

- [x] 批准
- [x] 修改：________________
