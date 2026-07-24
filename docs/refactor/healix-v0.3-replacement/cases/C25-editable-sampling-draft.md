# C25 — 采样草稿编辑

> 来源：Healix 仓库 `docs/refactor/healix-core-v0.3.0-replacement-assessment.md` 对应 Case；本清单以该评估要求为输入，并以 healix-core 当前 `master` 源码重新核验。

## 状态

**当前：稳定临时 ID 数据模型存在；完整编辑命令层缺失。**

## 业务不变量

编辑、移动、重排不改变 workflow/step/node/action 临时身份；删除不得留下悬空引用；临时 ID 与正式 ID 类型语义分离。

## 当前证据

- `domain/sampling/session.go`：stable capture identities
- `domain/sampling/workspace.go`：temporary workflow/node/steps、reference rebuild

## 调整清单

- [x] 为 insert/update/delete/move/reorder 增加领域命令。
- [x] 操作返回新 workspace，避免外部 slice 原地破坏 ID。
- [x] 校验 duplicate temp node/step/action IDs。
- [x] 删除时校验或清理 validation captured-action refs。
- [x] node-to-step references 始终 derived/rebuilt。
- [x] 使用 distinct ID value types 降低 temp/formal 混用。

## 测试与验收

- [x] reorder/move/edit 保留 IDs。
- [x] 删除仍被引用 node 失败。
- [x] 删除 step 后 rebuilt refs 正确。
- [x] duplicate IDs 被拒绝。
- [x] caller mutation 不改变 workspace。

## 依赖与风险

现有 exported mutable slices 容易让 Host 破坏不变量；引入命令 API 可能是 breaking change。

## 审核

- [x] 批准增加领域编辑命令
- [x] 修改：________________
