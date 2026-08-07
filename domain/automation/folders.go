package automation

// 本文件整体退役。文件夹的全部不变量——无环、同级不重名、深度上限、删除前为空——都是通用目录树规则，
// 把 Folder 换成书签或网盘目录，这段代码一行都不用改。它与 Automation 的核心（版本、发布、引用锁定）没有交集：
// 没有版本实体、不进发布依赖闭包、不进执行快照。资产上的 FolderID 甚至从未被校验到本树上。
//
// 真正的引用完整性（RequireEmpty 用的 occupancy）也是宿主查出来传进来的，domain 只是拿它跟 0 比。
// 宿主的存储层本就更适合承担这件事。宿主完整实现之前不会删除。

import (
	"errors"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

// Deprecated: 文件夹层级正在移交宿主，见 docs/contracts/retirement-plan.md。
const (
	CodeFolderNotFound    fault.Code = "AUTOMATION_FOLDER_NOT_FOUND"
	CodeFolderInvalid     fault.Code = "AUTOMATION_FOLDER_INVALID"
	CodeFolderTreeInvalid fault.Code = "AUTOMATION_FOLDER_TREE_INVALID"
	CodeFolderNotEmpty    fault.Code = "AUTOMATION_FOLDER_NOT_EMPTY"
)

func folderFault(cause error, kind fault.Kind, code fault.Code, message string) error {
	faultErr, err := fault.Wrap(cause, kind, code, message)
	if err != nil {
		panic(err)
	}
	return faultErr
}

func folderInvalidError(cause error) error {
	return folderFault(cause, fault.InvalidArgument, CodeFolderInvalid, "automation folder is invalid")
}

func folderTreeInvalidError(cause error) error {
	return folderFault(cause, fault.FailedPrecondition, CodeFolderTreeInvalid, "automation folder tree is invalid")
}

func folderNotEmptyError(cause error) error {
	return folderFault(cause, fault.FailedPrecondition, CodeFolderNotEmpty, "automation folder must be empty")
}

// Deprecated: 文件夹层级正在移交宿主，见 docs/contracts/retirement-plan.md。
func FolderNotFoundError() error {
	faultErr, err := fault.New(fault.NotFound, CodeFolderNotFound, "automation folder was not found")
	if err != nil {
		panic(err)
	}
	return faultErr
}

// Deprecated: 文件夹层级正在移交宿主，见 docs/contracts/retirement-plan.md。
type FolderKind string

// Deprecated: 文件夹层级正在移交宿主，见 docs/contracts/retirement-plan.md。
const (
	FolderNode     FolderKind = "NODE"
	FolderWorkflow FolderKind = "WORKFLOW"
	FolderTask     FolderKind = "TEST_TASK"
	MaxFolderDepth            = 5
)

// Deprecated: 文件夹层级正在移交宿主，见 docs/contracts/retirement-plan.md。
func (kind FolderKind) Validate() error {
	switch kind {
	case FolderNode, FolderWorkflow, FolderTask:
		return nil
	default:
		return folderInvalidError(errors.New("unsupported folder kind"))
	}
}

// Deprecated: 文件夹层级正在移交宿主，见 docs/contracts/retirement-plan.md。
type Folder struct {
	ID          string
	Kind        FolderKind
	ParentID    string
	DisplayName string
	CreatedAt   int64
	UpdatedAt   int64
}

// FolderForest 拥有跨越所有文件夹的不变量。适配器在其事务内重新加载林，因此这些决策是针对持久保存的同一快照做出的。
//
// Deprecated: 文件夹层级正在移交宿主，见 docs/contracts/retirement-plan.md。
type FolderForest struct {
	byID   map[string]Folder
	depths map[string]int
}

// Deprecated: 文件夹层级正在移交宿主，见 docs/contracts/retirement-plan.md。
type FolderOccupancy struct {
	Assets int
}

func validateFolderText(value string) error {
	if value != strings.TrimSpace(value) || parameter.ValidateName(value) != nil {
		return errors.New("folder text is invalid")
	}
	return nil
}

// Deprecated: 文件夹层级正在移交宿主，见 docs/contracts/retirement-plan.md。
func (folder Folder) Validate() error {
	if err := validateFolderText(folder.ID); err != nil {
		return folderInvalidError(err)
	}
	if err := folder.Kind.Validate(); err != nil {
		return err
	}
	if err := validateFolderText(folder.DisplayName); err != nil {
		return folderInvalidError(err)
	}
	if folder.ParentID != "" {
		if err := validateFolderText(folder.ParentID); err != nil {
			return folderInvalidError(err)
		}
	}
	if folder.ParentID == folder.ID {
		return folderInvalidError(errors.New("folder cannot be its own parent"))
	}
	return nil
}

// ValidateFolderTree 验证所有层次结构不变量并返回每个真实文件夹的从一开始的深度。空父 ID 表示深度为零的虚拟根，并且故意不保留为文件夹行。
//
// Deprecated: 文件夹层级正在移交宿主，见 docs/contracts/retirement-plan.md。
func ValidateFolderTree(folders []Folder) (map[string]int, error) {
	forest, err := NewFolderForest(folders)
	if err != nil {
		return nil, err
	}
	return forest.Depths(), nil
}

// Deprecated: 文件夹层级正在移交宿主，见 docs/contracts/retirement-plan.md。
func NewFolderForest(folders []Folder) (FolderForest, error) {
	byID := make(map[string]Folder, len(folders))
	siblings := make(map[string]string, len(folders))
	for _, folder := range folders {
		if err := folder.Validate(); err != nil {
			return FolderForest{}, err
		}
		if _, exists := byID[folder.ID]; exists {
			return FolderForest{}, folderTreeInvalidError(errors.New("duplicate folder identity"))
		}
		byID[folder.ID] = folder
		nameKey := string(folder.Kind) + "\x00" + folder.ParentID + "\x00" + strings.ToLower(strings.TrimSpace(folder.DisplayName))
		if _, exists := siblings[nameKey]; exists {
			return FolderForest{}, folderTreeInvalidError(errors.New("sibling folder name conflicts"))
		}
		siblings[nameKey] = folder.ID
	}
	for _, folder := range folders {
		if folder.ParentID == "" {
			continue
		}
		parent, exists := byID[folder.ParentID]
		if !exists {
			return FolderForest{}, folderTreeInvalidError(errors.New("folder parent does not exist"))
		}
		if parent.Kind != folder.Kind {
			return FolderForest{}, folderTreeInvalidError(errors.New("folder and parent kinds differ"))
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
			return 0, errors.New("folder hierarchy contains a cycle")
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
			return 0, errors.New("folder hierarchy exceeds maximum depth")
		}
		visiting[id] = false
		depths[id] = value
		return value, nil
	}
	for id := range byID {
		if _, err := depth(id); err != nil {
			return FolderForest{}, folderTreeInvalidError(err)
		}
	}
	return FolderForest{byID: byID, depths: depths}, nil
}

// Deprecated: 文件夹层级正在移交宿主，见 docs/contracts/retirement-plan.md。
func (f FolderForest) Depths() map[string]int {
	result := make(map[string]int, len(f.depths))
	for id, depth := range f.depths {
		result[id] = depth
	}
	return result
}

// Deprecated: 文件夹层级正在移交宿主，见 docs/contracts/retirement-plan.md。
func (f FolderForest) RequireEmpty(id string, occupancy FolderOccupancy) error {
	if err := validateFolderText(id); err != nil {
		return folderInvalidError(err)
	}
	if _, exists := f.byID[id]; !exists {
		return FolderNotFoundError()
	}
	if occupancy.Assets < 0 {
		return folderInvalidError(errors.New("folder occupancy cannot be negative"))
	}
	for _, folder := range f.byID {
		if folder.ParentID == id {
			return folderNotEmptyError(errors.New("folder has child folders"))
		}
	}
	if occupancy.Assets != 0 {
		return folderNotEmptyError(errors.New("folder has assets"))
	}
	return nil
}
