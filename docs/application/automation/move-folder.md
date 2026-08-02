# 移动文件夹

形状 A（先读后写，带 Revision CAS），共同的校验顺序、错误约定与适配器责任见 [模块 README](README.md)。以下只记录本用例独有的部分。

- 方法：`FolderService.Move(context.Context, kind domain.FolderKind、id、parentID、expected、at)`
- 输出：`FolderSnapshot`
- 领域转换：在森林副本里就地改写目标的 `ParentID` 与 `UpdatedAt`，再用 `domain.NewFolderForest` 重新校验——目标不存在返回 `AUTOMATION_FOLDER_NOT_FOUND`，新父节点异类、成环或超过 `MaxFolderDepth` 返回 `AUTOMATION_FOLDER_TREE_INVALID`。
- 端口：`FolderRepository.Load` / `Save`；读取失败包装为 `load folder forest`，写入失败包装为 `persist %s folder forest`。
- 独有的前置校验：`id` 为空时在 `Load` 之前就返回未分类的校验错误。下一个 Revision 由应用层的 `expected.Next()` 算出。

## 源码

- [应用服务](../../../application/automation/folder_service.go)
- [端口](../../../application/automation/folder_repository.go)
- [领域模型](../../../domain/automation/folders.go)
- [测试](../../../application/automation/folder_service_test.go)
