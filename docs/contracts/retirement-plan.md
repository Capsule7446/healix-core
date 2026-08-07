# 退役计划

本文件登记**正在移出 Core、但尚未删除**的能力。这是 Core 唯一允许出现
`Deprecated:` 标记的地方。

## 为什么会有这份清单

Core 原本禁止任何 `Deprecated:` 标记（[`unified_language_boundary_test.go`](../../architecture/unified_language_boundary_test.go)
的 `TestNoDeprecationMarkersPromiseAnOldNameStillWorks`）。理由是：改名不给过渡期，
**替代物本身就是迁移**——旧名字留着只会让两套词汇同时存在。

但有一种情况这条理由覆盖不到：**能力整体移交宿主**。此时 Core 里没有替代物，
替代物在宿主那边。Core 不能在宿主建好之前删掉它，所以过渡期不是便利，而是
删除唯一可行的顺序。

两者形状相同、含义相反，因此用一份显式清单区分，而不是靠标记本身判断。
未登记的 `Deprecated:` 仍然报红。

## 规则

1. 只有本表登记的文件可以携带 `Deprecated:` 标记。
2. 本表必须与 `unified_language_boundary_test.go` 的 `retiringFiles` 逐条对齐。
3. 标记为"整体退役"的文件，**每一个导出符号都必须带标记**——由
   `TestRetiringSurfaceDoesNotGrowUnmarked` 强制。这条是清单能安全存在的前提：
   否则被允许携带标记的文件会变成新导出符号的堆积地，退役悄悄变回永久特性。
4. 删除落地后，同时移除本表条目与 `retiringFiles` 条目（守卫会检查登记的文件
   是否还存在）。

## 当前退役项

### 文件夹层级（Folder）

| 文件 | 范围 |
|---|---|
| [`domain/automation/folders.go`](../../domain/automation/folders.go) | 整体退役 |
| [`application/automation/folder_service.go`](../../application/automation/folder_service.go) | 整体退役 |
| [`application/automation/folder_repository.go`](../../application/automation/folder_repository.go) | 整体退役 |
| [`domain/automation/assets.go`](../../domain/automation/assets.go) | 仅 `ElementTarget.FolderID` 与 `FlowFragment.FolderID` 两个字段 |

**为什么移出**

文件夹强制的全部不变量——无环、同级不重名（大小写不敏感）、最大深度 5、
父子同类、删除前为空——都是通用目录树规则。把 `Folder` 换成书签或网盘目录，
`folders.go` 一行都不用改。

它与 Automation 上下文的核心（版本化、发布、引用锁定）没有交集：

- 没有版本实体，也没有发布快照
- 不进 `ExecutionFlowVersionPublication` 的依赖闭包
- 不进 `execution.InstanceSnapshot`（`WorkflowSnapshot` / `NodeSnapshot` 都不带 `FolderID`）
- 资产上的 `FolderID` **从未被校验到 `FolderForest` 上**——指向不存在的文件夹，
  domain 不会有任何反应
- `FolderID` 在执行链路上只出现一次，是被计入字节预算统计；执行侧对它的全部
  兴趣就是这个字符串有多长

连唯一像业务规则的那条也不是 Core 判的：`FolderForest.RequireEmpty(id, occupancy)`
里的资产计数由宿主查询提供，domain 只是把宿主给的数字跟 0 比。真正的引用完整性
一直在宿主的存储层。

**宿主接手需要实现什么**

Core 删除之前，宿主需要自行承担：

1. 目录树本身的存取与并发控制（当前由 `FolderRepository` + `Revision` CAS 表达）
2. 树不变量：无环、同级不重名、深度上限、父子同类
3. 删除保护：有子文件夹或有资产时拒绝删除
4. 资产到文件夹的归属关系，以及**它的引用完整性**——这一条 Core 从来没有兑现过，
   宿主接手时应当补上，否则删除文件夹会留下一批悬空的 `FolderID`

前三条在关系型数据库里通常比在领域层更直接：外键、唯一索引、递归 CTE、
`ON DELETE RESTRICT`。

**删除时机**

宿主完整实现并迁移完成之后的某个版本。当前未定版本号，不设截止日期——
过早的截止日期会变成谎言。

**错误码**

`AUTOMATION_FOLDER_NOT_FOUND`、`AUTOMATION_FOLDER_INVALID`、
`AUTOMATION_FOLDER_TREE_INVALID`、`AUTOMATION_FOLDER_NOT_EMPTY`
（[错误码注册表](error-code-registry.md) 第 116-119 行）随本次退役一并移除。
按注册表规则，删除后以墓碑保留，不得复用这四个码。
