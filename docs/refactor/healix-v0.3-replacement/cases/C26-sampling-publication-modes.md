# C26 — 采样发布策略

> 来源：历史替换评估中的同编号案例；本清单以该评估要求为输入，并以 healix-core 当前实现（`12d1ba2`）重新核验。

## 状态

**当前结果：已由 v0.3 替换实现覆盖；以下证据、清单与验收项按当前模型解释。**

## 业务不变量

每个临时节点必须明确选择 CREATE、FORCE_CREATE、MERGE 或 REUSE；UNDECIDED 不可发布，结果工作流必须引用对应正式版本。MERGE 保留已有节点身份和元数据，以采样得到的 PageURL、Origin、Selectors、Fingerprint 发布一个完整的新 NodeVersion，不与旧版本隐式合并 selectors 或 fingerprint。

## 当前证据

- `domain/sampling/workspace.go`：解析模式
- `domain/automation/sampling_publication.go`：规范化决策与校验
- `domain/automation/sampling_publication.go` 与 `application/automation/asset_service_test.go`：发布规范化及应用边界

## 调整清单

- [x] 实现 workspace mode → 发布 application mapper。
- [x] UNDECIDED 在调用 repository 前拒绝。
- [x] 保留 original mode 到发布 audit/result，尤其 FORCE_CREATE。
- [x] MERGE 保留已有节点 identity/metadata，以采样字段整体替换版本内容；不得隐式 union selectors/fingerprint。
- [x] MERGE/REUSE 带 expected current version/revision CAS。
- [x] MERGE 后将所有工作流 node references 精确重写到新建的 `(NodeID, NodeVersionID)`。
- [x] FORCE_CREATE 要求明确 authorization policy。
- [x] 发布 ID 的 same/different payload replay 规则。
- [x] 所有 node decisions + workflow 同事务。
- [x] 定义 version/identity conflict typed errors。

## 测试与验收

- [x] 四种 mode 全矩阵通过。
- [x] CREATE/FORCE_CREATE 都 version 1，但审计可区分。
- [x] stale MERGE/REUSE 冲突。
- [x] MERGE 保留 NodeID、发布完整替换的新 NodeVersion，且工作流精确引用该版本。
- [x] MERGE 不保留旧 selectors/fingerprint，除非采样结果本身包含它们。
- [x] mixed modes 原子发布。
- [x] retry 不重复创建 node/version。

## 依赖与风险

依赖 C25；C26 先定义 mode→发布语义，C27 再执行临时资产到正式资产的规范化 rewrite。规范化后丢失 FORCE_CREATE 意图会削弱审计和授权。

## 审核

- [x] 批准保留 mode 到审计结果
- [x] 修改：________________
