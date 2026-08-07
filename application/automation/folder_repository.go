package automation

import (
	"context"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

// Deprecated: 文件夹层级正在移交宿主，见 docs/contracts/retirement-plan.md。
type FolderSnapshot struct {
	Revision domain.Revision
	Folders  []domain.Folder
}

// Deprecated: 文件夹层级正在移交宿主，见 docs/contracts/retirement-plan.md。
type FolderOccupancySnapshot struct {
	Revision  domain.Revision
	Occupancy domain.FolderOccupancy
}

// Deprecated: 文件夹层级正在移交宿主，见 docs/contracts/retirement-plan.md。
type DeleteEmptyFolderCommand struct {
	Kind                      domain.FolderKind
	FolderID                  string
	ExpectedForestRevision    domain.Revision
	ExpectedOccupancyRevision domain.Revision
	Next                      FolderSnapshot
}

// Deprecated: 文件夹层级正在移交宿主，见 docs/contracts/retirement-plan.md。
type FolderRepository interface {
	Load(context.Context, domain.FolderKind) (FolderSnapshot, error)
	Occupancy(context.Context, domain.FolderKind, string) (FolderOccupancySnapshot, error)
	Save(context.Context, domain.FolderKind, domain.Revision, FolderSnapshot) (FolderSnapshot, error)
	DeleteEmptyFolder(context.Context, DeleteEmptyFolderCommand) (FolderSnapshot, error)
}
