package automation

import (
	"context"
	"fmt"
	"strings"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

// FolderService 编排已弃用的文件夹创建、移动和删除操作。
// Deprecated: 文件夹层级正在移交宿主，见 docs/contracts/retirement-plan.md。
type FolderService struct{ repository FolderRepository }

// NewFolderService 构造已弃用的文件夹服务。
// Deprecated: 文件夹层级正在移交宿主，见 docs/contracts/retirement-plan.md。
func NewFolderService(repository FolderRepository) FolderService {
	return FolderService{repository: repository}
}

// Create 校验文件夹 ID，将文件夹追加到森林并按期望修订执行 CAS 保存。
// Deprecated: 文件夹层级正在移交宿主，见 docs/contracts/retirement-plan.md。
func (s FolderService) Create(ctx context.Context, folder domain.Folder, expected domain.Revision) (FolderSnapshot, error) {
	if strings.TrimSpace(folder.ID) == "" {
		return FolderSnapshot{}, fmt.Errorf("folder ID is required")
	}
	return s.change(ctx, folder.Kind, expected, func(folders []domain.Folder) ([]domain.Folder, error) {
		return append(folders, folder), nil
	})
}

// Move 校验文件夹 ID，更新其父级和时间戳，并按期望修订执行 CAS 保存。
// Deprecated: 文件夹层级正在移交宿主，见 docs/contracts/retirement-plan.md。
func (s FolderService) Move(ctx context.Context, kind domain.FolderKind, id, parentID string, expected domain.Revision, at int64) (FolderSnapshot, error) {
	if strings.TrimSpace(id) == "" {
		return FolderSnapshot{}, fmt.Errorf("folder ID is required")
	}
	return s.change(ctx, kind, expected, func(folders []domain.Folder) ([]domain.Folder, error) {
		for index := range folders {
			if folders[index].ID == id {
				folders[index].ParentID = parentID
				folders[index].UpdatedAt = at
				return folders, nil
			}
		}
		return nil, domain.FolderNotFoundError()
	})
}

// Delete 校验文件夹为空，同时携带森林和占用修订执行 CAS 删除。
// Deprecated: 文件夹层级正在移交宿主，见 docs/contracts/retirement-plan.md。
func (s FolderService) Delete(ctx context.Context, kind domain.FolderKind, id string, expected domain.Revision) (FolderSnapshot, error) {
	if isNilDependency(s.repository) {
		return FolderSnapshot{}, AutomationConfigurationError()
	}
	if strings.TrimSpace(id) == "" {
		return FolderSnapshot{}, fmt.Errorf("folder ID is required")
	}
	snapshot, err := s.repository.Load(ctx, kind)
	if err != nil {
		return FolderSnapshot{}, fmt.Errorf("load folder forest: %w", err)
	}
	if snapshot.Revision != expected {
		return FolderSnapshot{}, AutomationRevisionConflictError()
	}
	forest, err := domain.NewFolderForest(snapshot.Folders)
	if err != nil {
		// NewFolderForest 已返回 AUTOMATION_FOLDER_INVALID 或
		// AUTOMATION_FOLDER_TREE_INVALID；此处不包裹，避免未分类外层错误掩盖原分类。
		return FolderSnapshot{}, err
	}
	occupancy, err := s.repository.Occupancy(ctx, kind, id)
	if err != nil {
		return FolderSnapshot{}, fmt.Errorf("load folder occupancy: %w", err)
	}
	if err := forest.RequireEmpty(id, occupancy.Occupancy); err != nil {
		return FolderSnapshot{}, err
	}
	folders := make([]domain.Folder, 0, len(snapshot.Folders)-1)
	for _, folder := range snapshot.Folders {
		if folder.ID != id {
			folders = append(folders, folder)
		}
	}
	revision, err := expected.Next()
	if err != nil {
		// Revision.Next 已返回注册错误码。
		return FolderSnapshot{}, err
	}
	result, err := s.repository.DeleteEmptyFolder(ctx, DeleteEmptyFolderCommand{
		Kind:                      kind,
		FolderID:                  id,
		ExpectedForestRevision:    expected,
		ExpectedOccupancyRevision: occupancy.Revision,
		Next:                      FolderSnapshot{Revision: revision, Folders: folders},
	})
	if err != nil {
		return FolderSnapshot{}, fmt.Errorf("delete empty folder %q: %w", id, err)
	}
	return result, nil
}

// change 读取森林、校验期望修订、应用变更并持久化；任何校验或变更失败都不会写入。
func (s FolderService) change(ctx context.Context, kind domain.FolderKind, expected domain.Revision, apply func([]domain.Folder) ([]domain.Folder, error)) (FolderSnapshot, error) {
	if isNilDependency(s.repository) {
		return FolderSnapshot{}, AutomationConfigurationError()
	}
	snapshot, err := s.repository.Load(ctx, kind)
	if err != nil {
		return FolderSnapshot{}, fmt.Errorf("load folder forest: %w", err)
	}
	if snapshot.Revision != expected {
		return FolderSnapshot{}, AutomationRevisionConflictError()
	}
	folders := append([]domain.Folder(nil), snapshot.Folders...)
	folders, err = apply(folders)
	if err != nil {
		return FolderSnapshot{}, err
	}
	if _, err := domain.NewFolderForest(folders); err != nil {
		// NewFolderForest 已返回 AUTOMATION_FOLDER_INVALID 或
		// AUTOMATION_FOLDER_TREE_INVALID；此处不包裹，避免未分类外层错误掩盖原分类。
		return FolderSnapshot{}, err
	}
	return s.persist(ctx, kind, expected, folders)
}

// persist 递增森林修订，并按期望修订保存文件夹快照。
func (s FolderService) persist(ctx context.Context, kind domain.FolderKind, expected domain.Revision, folders []domain.Folder) (FolderSnapshot, error) {
	revision, err := expected.Next()
	if err != nil {
		// Revision.Next 已返回注册错误码。
		return FolderSnapshot{}, err
	}
	result, err := s.repository.Save(ctx, kind, expected, FolderSnapshot{Revision: revision, Folders: folders})
	if err != nil {
		return FolderSnapshot{}, fmt.Errorf("persist %s folder forest: %w", kind, err)
	}
	return result, nil
}
