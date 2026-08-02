# 创建文件夹

形状 A（先读后写，带 Revision CAS），共同的校验顺序、错误约定与适配器责任见 [模块 README](README.md)。以下只记录本用例独有的部分。

文件夹创建同样要先读出整片森林才能校验父子关系，所以它是形状 A 而不是形状 B —— 四个 `create-*` 资产用例里只有它带 CAS。

- 方法：`FolderService.Create(context.Context, folder domain.Folder、expected)`
- 输出：`FolderSnapshot`
- 领域转换：把 `folder` 追加到已加载 `snapshot.Folders` 的副本上，再用 `domain.NewFolderForest` 重新校验整片森林——重复身份与同级重名返回 `AUTOMATION_FOLDER_TREE_INVALID`，字段本身不合法返回 `AUTOMATION_FOLDER_INVALID`。
- 端口：`FolderRepository.Load` / `Save`；读取失败包装为 `load folder forest`，写入失败包装为 `persist %s folder forest`。
- 独有的前置校验：`folder.ID` 为空时在 `Load` 之前就返回未分类的校验错误。CAS 用的 `kind` 取自 `folder.Kind`，不是单独的参数。下一个 Revision 由应用层的 `expected.Next()` 算出，不是领域推进的。

## 源码

- [应用服务](../../../application/automation/folder_service.go)
- [端口](../../../application/automation/folder_repository.go)
- [领域模型](../../../domain/automation/folders.go)
- [测试](../../../application/automation/folder_service_test.go)
