# C27 — 临时资产转正式资产

> 来源：历史替换评估中的同编号案例；本清单以该评估要求为输入，并以 healix-core 当前实现（`12d1ba2`）重新核验。

## 状态

**当前结果：已由 v0.3 替换实现覆盖；以下证据、清单与验收项按当前模型解释。**

## 业务不变量

每个被引用 TemporaryNodeID 恰好映射一个 `(NodeID, NodeVersionID)`；所有 root/children/validation branches 引用递归重写，发布结果不残留临时 ID。

## 当前证据

- `domain/sampling/workspace.go`：`RebuildTemporaryNodeReferences`
- `domain/automation/sampling_publication.go`：`SamplingNodeMapping`、递归引用校验

## 调整清单

- [x] 明确 IdentityKey、NodeUUID、TemporaryNodeID 关系。
- [x] 新增 canonical temp→formal rewrite function/application service。
- [x] 递归覆盖 root、repeat children、validation branches。
- [x] input temp IDs 与 output mappings exact-set equality。
- [x] missing/duplicate/extra mappings 拒绝。
- [x] rewrite 将每个 node reference 精确替换为 mapping 的 `(NodeID, NodeVersionID)`，保留 step/workflow identity 和结构；MERGE 即使保留 NodeID，也必须写入新 NodeVersionID。
- [x] 发布 retry 返回原 mapping，不重新分配。
- [x] mappings 与正式 assets 同事务持久化。
- [x] 断言 publishable workflow 中无 temp IDs。

## 测试与验收

- [x] 两 steps 共用 temp node 得到同 mapping。
- [x] 各层级递归引用全部重写。
- [x] missing/duplicate mapping 在持久化前失败。
- [x] failed 发布不留 mapping/assets。
- [x] retry mapping byte-for-byte 等价。

## 依赖与风险

依赖 C25/C26；raw string IDs 容易类别混用，建议 distinct value types。

## 审核

- [x] 批准
- [x] 修改：________________
