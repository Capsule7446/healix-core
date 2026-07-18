package workspace

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateFolderTreeAllowsFiveLevels(t *testing.T) {
	folders := []WorkspaceFolder{
		{ID: "folder-1", Kind: FolderNode, DisplayName: "一级"},
		{ID: "folder-2", Kind: FolderNode, ParentID: "folder-1", DisplayName: "二级"},
		{ID: "folder-3", Kind: FolderNode, ParentID: "folder-2", DisplayName: "三级"},
		{ID: "folder-4", Kind: FolderNode, ParentID: "folder-3", DisplayName: "四级"},
		{ID: "folder-5", Kind: FolderNode, ParentID: "folder-4", DisplayName: "五级"},
	}

	depths, err := ValidateFolderTree(folders)
	if err != nil {
		t.Fatal(err)
	}
	if depths["folder-5"] != MaxFolderDepth {
		t.Fatalf("depth = %d, want %d", depths["folder-5"], MaxFolderDepth)
	}
}

func TestValidateFolderTreeRejectsInvalidHierarchy(t *testing.T) {
	validFiveLevels := []WorkspaceFolder{
		{ID: "folder-1", Kind: FolderNode, DisplayName: "一级"},
		{ID: "folder-2", Kind: FolderNode, ParentID: "folder-1", DisplayName: "二级"},
		{ID: "folder-3", Kind: FolderNode, ParentID: "folder-2", DisplayName: "三级"},
		{ID: "folder-4", Kind: FolderNode, ParentID: "folder-3", DisplayName: "四级"},
		{ID: "folder-5", Kind: FolderNode, ParentID: "folder-4", DisplayName: "五级"},
	}
	tests := []struct {
		name    string
		folders []WorkspaceFolder
		want    string
	}{
		{
			name: "sixth level",
			folders: append(append([]WorkspaceFolder(nil), validFiveLevels...), WorkspaceFolder{
				ID: "folder-6", Kind: FolderNode, ParentID: "folder-5", DisplayName: "六级",
			}),
			want: "maximum depth",
		},
		{
			name: "cross kind parent",
			folders: []WorkspaceFolder{
				{ID: "nodes", Kind: FolderNode, DisplayName: "Node"},
				{ID: "workflows", Kind: FolderWorkflow, ParentID: "nodes", DisplayName: "Workflow"},
			},
			want: "same kind",
		},
		{
			name: "cycle",
			folders: []WorkspaceFolder{
				{ID: "a", Kind: FolderNode, ParentID: "b", DisplayName: "A"},
				{ID: "b", Kind: FolderNode, ParentID: "a", DisplayName: "B"},
			},
			want: "cycle",
		},
		{
			name: "sibling name conflict",
			folders: []WorkspaceFolder{
				{ID: "a", Kind: FolderTask, DisplayName: "Smoke"},
				{ID: "b", Kind: FolderTask, DisplayName: "smoke"},
			},
			want: "sibling folder name",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ValidateFolderTree(test.folders)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestFolderForestOwnsEmptyDeletionDecision(t *testing.T) {
	forest, err := NewFolderForest([]WorkspaceFolder{{ID: "root", Kind: FolderNode, DisplayName: "节点"},
		{ID: "child", Kind: FolderNode, ParentID: "root", DisplayName: "子目录"},
		{ID: "empty", Kind: FolderNode, DisplayName: "空目录"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := forest.RequireEmpty("empty", FolderOccupancy{}); err != nil {
		t.Fatalf("empty folder: %v", err)
	}
	for name, occupancy := range map[string]FolderOccupancy{
		"child folder": {},
		"asset":        {Assets: 1},
	} {
		t.Run(name, func(t *testing.T) {
			id := "empty"
			if name == "child folder" {
				id = "root"
			}
			if err := forest.RequireEmpty(id, occupancy); err == nil || !strings.Contains(err.Error(), "must be empty") {
				t.Fatalf("error = %v", err)
			}
		})
	}
	if err := forest.RequireEmpty("missing", FolderOccupancy{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing folder error = %v", err)
	}
}
