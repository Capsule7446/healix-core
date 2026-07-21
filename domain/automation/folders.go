package automation

import (
	"errors"
	"fmt"
	"strings"
)

var ErrFolderNotFound = errors.New("folder not found")

type FolderKind string

const (
	FolderNode     FolderKind = "NODE"
	FolderWorkflow FolderKind = "WORKFLOW"
	FolderTask     FolderKind = "TEST_TASK"
	MaxFolderDepth            = 5
)

func (kind FolderKind) Validate() error {
	switch kind {
	case FolderNode, FolderWorkflow, FolderTask:
		return nil
	default:
		return fmt.Errorf("unsupported folder kind %q", kind)
	}
}

type Folder struct {
	ID          string
	Kind        FolderKind
	ParentID    string
	DisplayName string
	CreatedAt   int64
	UpdatedAt   int64
}

// FolderForest 拥有跨越所有文件夹的不变量。适配器在其事务内重新加载林，因此这些决策是针对持久保存的同一快照做出的。
type FolderForest struct {
	byID   map[string]Folder
	depths map[string]int
}

type FolderOccupancy struct {
	Assets int
}

func (folder Folder) Validate() error {
	if strings.TrimSpace(folder.ID) == "" {
		return errors.New("folder id is required")
	}
	if err := folder.Kind.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(folder.DisplayName) == "" {
		return errors.New("folder display name is required")
	}
	if folder.ParentID == folder.ID {
		return errors.New("folder cannot be its own parent")
	}
	return nil
}

// ValidateFolderTree 验证所有层次结构不变量并返回每个真实文件夹的从一开始的深度。空父 ID 表示深度为零的虚拟根，并且故意不保留为文件夹行。
func ValidateFolderTree(folders []Folder) (map[string]int, error) {
	forest, err := NewFolderForest(folders)
	if err != nil {
		return nil, err
	}
	return forest.Depths(), nil
}

func NewFolderForest(folders []Folder) (FolderForest, error) {
	byID := make(map[string]Folder, len(folders))
	siblings := make(map[string]string, len(folders))
	for _, folder := range folders {
		if err := folder.Validate(); err != nil {
			return FolderForest{}, fmt.Errorf("folder %q: %w", folder.ID, err)
		}
		if _, exists := byID[folder.ID]; exists {
			return FolderForest{}, fmt.Errorf("duplicate folder id %q", folder.ID)
		}
		byID[folder.ID] = folder
		nameKey := string(folder.Kind) + "\x00" + folder.ParentID + "\x00" + strings.ToLower(strings.TrimSpace(folder.DisplayName))
		if siblingID, exists := siblings[nameKey]; exists {
			return FolderForest{}, fmt.Errorf("sibling folder name %q conflicts between %s and %s", folder.DisplayName, siblingID, folder.ID)
		}
		siblings[nameKey] = folder.ID
	}
	for _, folder := range folders {
		if folder.ParentID == "" {
			continue
		}
		parent, exists := byID[folder.ParentID]
		if !exists {
			return FolderForest{}, fmt.Errorf("folder %s parent %s does not exist", folder.ID, folder.ParentID)
		}
		if parent.Kind != folder.Kind {
			return FolderForest{}, fmt.Errorf("folder %s and parent %s must have the same kind", folder.ID, parent.ID)
		}
	}

	depths := make(map[string]int, len(folders))
	visiting := make(map[string]bool, len(folders))
	var depth func(string) (int, error)
	depth = func(id string) (int, error) {
		if value := depths[id]; value != 0 {
			return value, nil
		}
		if visiting[id] {
			return 0, fmt.Errorf("folder hierarchy contains a cycle at %s", id)
		}
		visiting[id] = true
		folder := byID[id]
		value := 1
		if folder.ParentID != "" {
			parentDepth, err := depth(folder.ParentID)
			if err != nil {
				return 0, err
			}
			value = parentDepth + 1
		}
		if value > MaxFolderDepth {
			return 0, fmt.Errorf("folder %s exceeds maximum depth %d", id, MaxFolderDepth)
		}
		visiting[id] = false
		depths[id] = value
		return value, nil
	}
	for id := range byID {
		if _, err := depth(id); err != nil {
			return FolderForest{}, err
		}
	}
	return FolderForest{byID: byID, depths: depths}, nil
}

func (f FolderForest) Depths() map[string]int {
	result := make(map[string]int, len(f.depths))
	for id, depth := range f.depths {
		result[id] = depth
	}
	return result
}

func (f FolderForest) RequireEmpty(id string, occupancy FolderOccupancy) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("folder id is required")
	}
	if _, exists := f.byID[id]; !exists {
		return fmt.Errorf("folder %s: %w", id, ErrFolderNotFound)
	}
	if occupancy.Assets < 0 {
		return errors.New("folder occupancy cannot be negative")
	}
	for _, folder := range f.byID {
		if folder.ParentID == id {
			return errors.New("folder must be empty before deletion")
		}
	}
	if occupancy.Assets != 0 {
		return errors.New("folder must be empty before deletion")
	}
	return nil
}
