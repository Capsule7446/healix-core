package automation

import (
	"context"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

// FolderSnapshot 保存文件夹森林及其修订快照。
// Deprecated: 文件夹层级正在移交宿主，见 docs/contracts/retirement-plan.md。
type FolderSnapshot struct {
	Revision domain.Revision
	Folders  []domain.Folder
}

// FolderOccupancySnapshot 保存宿主查询的占用数量及其修订。
// Deprecated: 文件夹层级正在移交宿主，见 docs/contracts/retirement-plan.md。
type FolderOccupancySnapshot struct {
	Revision  domain.Revision
	Occupancy domain.FolderOccupancy
}

// DeleteEmptyFolderCommand 携带删除空文件夹所需的森林和占用期望修订。
// Deprecated: 文件夹层级正在移交宿主，见 docs/contracts/retirement-plan.md。
type DeleteEmptyFolderCommand struct {
	Kind                      domain.FolderKind
	FolderID                  string
	ExpectedForestRevision    domain.Revision
	ExpectedOccupancyRevision domain.Revision
	Next                      FolderSnapshot
}

// FolderRepository 定义已弃用文件夹层级的读取、占用查询、保存和空文件夹删除端口。
// Deprecated: 文件夹层级正在移交宿主，见 docs/contracts/retirement-plan.md。
type FolderRepository interface {
	// Load 读取指定种类的文件夹快照；不存在或存储失败时返回错误。
	Load(context.Context, domain.FolderKind) (FolderSnapshot, error)
	// Occupancy 查询指定种类和文件夹 ID 的占用快照。
	Occupancy(context.Context, domain.FolderKind, string) (FolderOccupancySnapshot, error)
	// Save 按期望森林修订保存文件夹快照。
	Save(context.Context, domain.FolderKind, domain.Revision, FolderSnapshot) (FolderSnapshot, error)
	// DeleteEmptyFolder 按命令中的森林和占用修订以 CAS 方式删除空文件夹。
	DeleteEmptyFolder(context.Context, DeleteEmptyFolderCommand) (FolderSnapshot, error)
}
