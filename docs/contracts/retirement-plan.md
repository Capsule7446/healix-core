# Core 能力移交登记

本文件是 Core 中唯一允许出现 `Deprecated:` 标记的契约登记。登记项表示能力的所有权由宿主承担；在登记存在期间，Core 保留当前实现和测试，宿主必须按本页前置条件完成接管。

## 规则

1. 只有本页登记的文件或字段可以携带 `Deprecated:` 标记。
2. 本页路径和符号必须与 [`architecture/unified_language_boundary_test.go`](../../architecture/unified_language_boundary_test.go) 的 `retiringFiles` 完全一致。
3. 整体登记文件的每个导出符号都必须带标记；部分登记只允许列出的字段带标记。守卫会阻止未登记导出面增长。
4. 代码删除或所有权完成转移后，同时移除本页对应项和守卫登记；不得留下失效登记。

## 当前登记项：文件夹层级

| 文件 | 登记范围 | 宿主接管前置条件 |
|---|---|---|
| [`domain/automation/folders.go`](../../domain/automation/folders.go) | 整个文件 | 宿主实现目录树存取、CAS、无环/同级唯一/深度上限和非空删除保护。 |
| [`application/automation/folder_service.go`](../../application/automation/folder_service.go) | 整个文件 | 宿主提供等价的创建、移动、删除服务，并保持 `Revision` 与错误码语义。 |
| [`application/automation/folder_repository.go`](../../application/automation/folder_repository.go) | 整个文件 | 宿主提供目录读取、占用统计和原子保存端口。 |
| [`domain/automation/assets.go`](../../domain/automation/assets.go) | `ElementTarget.FolderID`、`FlowFragment.FolderID` 字段 | 宿主接管资产与目录的引用完整性，避免留下悬空 `FolderID`。 |

文件夹规则只约束目录树和字段引用，不参与版本发布、执行计划或 `InstanceSnapshot`。Core 当前仍会校验已登记文件中的领域规则；宿主在接管前不得假设这些端口已经由 Core 持久化。

## 验证

- [`TestNoDeprecationMarkersPromiseAnOldNameStillWorks`](../../architecture/unified_language_boundary_test.go) 阻止未登记的弃用标记。
- [`TestRetiringSurfaceDoesNotGrowUnmarked`](../../architecture/unified_language_boundary_test.go) 校验整体文件的导出面不增长、部分字段仍带标记。
- [`TestRetirementRegisterMatchesItsPlan`](../../architecture/unified_language_boundary_test.go) 双向校验本页路径与代码登记。
