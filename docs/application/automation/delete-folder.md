# 删除文件夹

形状 A（先读后写，带 Revision CAS），共同的校验顺序、错误约定与适配器责任见 [模块 README](README.md)。以下只记录本用例独有的部分。，外加第二次 CAS。

- 方法：`FolderService.Delete(context.Context, kind domain.FolderKind、id、expected)`
- 输出：`FolderSnapshot`
- 领域转换：`domain.NewFolderForest` 重建森林后调用 `FolderForest.RequireEmpty(id, occupancy.Occupancy)` —— 目标不存在返回 `AUTOMATION_FOLDER_NOT_FOUND`，还有子文件夹或资产返回 `AUTOMATION_FOLDER_NOT_EMPTY`。通过后从副本里摘掉该文件夹。
- 端口：`FolderRepository.Load` / `Occupancy` / `DeleteEmptyFolder`，是本目录里唯一用到三个读写端口的用例。森林读取失败包装为 `load folder forest`，占用读取失败包装为 `load folder occupancy`，提交失败包装为 `delete empty folder %q`。
- 双 CAS：`DeleteEmptyFolderCommand` 同时携带 `ExpectedForestRevision`（即 `expected`）与 `ExpectedOccupancyRevision`（读占用时拿到的那个）。适配器必须在同一个事务里对两者都做条件写；只校验其中一个，就等于允许在森林 Revision 不变的窗口里把资产塞进正要被删的文件夹。

## 源码

- [应用服务](../../../application/automation/folder_service.go)
- [端口](../../../application/automation/folder_repository.go)
- [领域模型](../../../domain/automation/folders.go)
- [测试](../../../application/automation/folder_service_test.go)
