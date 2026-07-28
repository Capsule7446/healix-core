package automation

import (
	"context"
	"fmt"
	"strings"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

type FolderService struct{ repository FolderRepository }

func NewFolderService(repository FolderRepository) FolderService {
	return FolderService{repository: repository}
}

func (s FolderService) Create(ctx context.Context, folder domain.Folder, expected domain.Revision) (FolderSnapshot, error) {
	if strings.TrimSpace(folder.ID) == "" {
		return FolderSnapshot{}, fmt.Errorf("folder ID is required")
	}
	return s.change(ctx, folder.Kind, expected, func(folders []domain.Folder) ([]domain.Folder, error) {
		return append(folders, folder), nil
	})
}

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
		return nil, fmt.Errorf("folder %s: %w", id, domain.ErrFolderNotFound)
	})
}

func (s FolderService) Delete(ctx context.Context, kind domain.FolderKind, id string, expected domain.Revision) (FolderSnapshot, error) {
	if isNilDependency(s.repository) {
		return FolderSnapshot{}, ErrAutomationConfiguration
	}
	if strings.TrimSpace(id) == "" {
		return FolderSnapshot{}, fmt.Errorf("folder ID is required")
	}
	snapshot, err := s.repository.Load(ctx, kind)
	if err != nil {
		return FolderSnapshot{}, fmt.Errorf("load folder forest: %w", err)
	}
	if snapshot.Revision != expected {
		return FolderSnapshot{}, RevisionConflictError{AggregateKind: "folder forest", ID: string(kind), Expected: expected, Actual: snapshot.Revision}
	}
	forest, err := domain.NewFolderForest(snapshot.Folders)
	if err != nil {
		return FolderSnapshot{}, fmt.Errorf("validate folder forest: %w", err)
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
		return FolderSnapshot{}, fmt.Errorf("advance folder forest revision: %w", err)
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

func (s FolderService) change(ctx context.Context, kind domain.FolderKind, expected domain.Revision, apply func([]domain.Folder) ([]domain.Folder, error)) (FolderSnapshot, error) {
	if isNilDependency(s.repository) {
		return FolderSnapshot{}, ErrAutomationConfiguration
	}
	snapshot, err := s.repository.Load(ctx, kind)
	if err != nil {
		return FolderSnapshot{}, fmt.Errorf("load folder forest: %w", err)
	}
	if snapshot.Revision != expected {
		return FolderSnapshot{}, RevisionConflictError{AggregateKind: "folder forest", ID: string(kind), Expected: expected, Actual: snapshot.Revision}
	}
	folders := append([]domain.Folder(nil), snapshot.Folders...)
	folders, err = apply(folders)
	if err != nil {
		return FolderSnapshot{}, err
	}
	if _, err := domain.NewFolderForest(folders); err != nil {
		return FolderSnapshot{}, fmt.Errorf("validate folder forest: %w", err)
	}
	return s.persist(ctx, kind, expected, folders)
}

func (s FolderService) persist(ctx context.Context, kind domain.FolderKind, expected domain.Revision, folders []domain.Folder) (FolderSnapshot, error) {
	revision, err := expected.Next()
	if err != nil {
		return FolderSnapshot{}, fmt.Errorf("advance folder forest revision: %w", err)
	}
	result, err := s.repository.Save(ctx, kind, expected, FolderSnapshot{Revision: revision, Folders: folders})
	if err != nil {
		return FolderSnapshot{}, fmt.Errorf("persist %s folder forest: %w", kind, err)
	}
	return result, nil
}
