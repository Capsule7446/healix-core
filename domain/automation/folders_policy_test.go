package automation

import (
	"math"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
)

func TestFolderValidationMatrix(t *testing.T) {
	tests := []struct {
		name    string
		folder  Folder
		wantErr string
	}{
		{name: "valid root", folder: Folder{ID: "root", Kind: FolderNode, DisplayName: "Nodes"}},
		{name: "missing id", folder: Folder{Kind: FolderNode, DisplayName: "Nodes"}, wantErr: "folder id is required"},
		{name: "invalid kind", folder: Folder{ID: "root", Kind: "OTHER", DisplayName: "Nodes"}, wantErr: "unsupported folder kind"},
		{name: "missing display name", folder: Folder{ID: "root", Kind: FolderNode}, wantErr: "folder display name is required"},
		{name: "self parent", folder: Folder{ID: "root", Kind: FolderNode, ParentID: "root", DisplayName: "Nodes"}, wantErr: "own parent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.folder.Validate()
			if tt.wantErr == "" && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateFolderTree(t *testing.T) {
	depths, err := ValidateFolderTree([]Folder{
		{ID: "root", Kind: FolderNode, DisplayName: "Root"},
		{ID: "child", Kind: FolderNode, ParentID: "root", DisplayName: "Child"},
	})
	if err != nil {
		t.Fatalf("ValidateFolderTree() error = %v", err)
	}
	if depths["root"] != 1 || depths["child"] != 2 {
		t.Fatalf("ValidateFolderTree() depths = %v", depths)
	}
	if _, err := ValidateFolderTree([]Folder{{ID: "bad", Kind: FolderNode, ParentID: "missing", DisplayName: "Bad"}}); err == nil {
		t.Fatal("ValidateFolderTree accepted an orphan")
	}
}

func TestNewFolderForestValidation(t *testing.T) {
	valid := []Folder{
		{ID: "root", Kind: FolderWorkflow, DisplayName: "Workflows"},
		{ID: "child", Kind: FolderWorkflow, ParentID: "root", DisplayName: "Child"},
	}
	forest, err := NewFolderForest(valid)
	if err != nil {
		t.Fatalf("NewFolderForest() error = %v", err)
	}
	depths := forest.Depths()
	if depths["root"] != 1 || depths["child"] != 2 {
		t.Fatalf("Depths() = %v", depths)
	}
	depths["root"] = 99
	if forest.Depths()["root"] != 1 {
		t.Fatal("Depths returned mutable internal map")
	}

	cases := []struct {
		name    string
		folders []Folder
		wantErr string
	}{
		{name: "duplicate id", folders: append(valid, Folder{ID: "child", Kind: FolderWorkflow, DisplayName: "Other"}), wantErr: "duplicate folder id"},
		{name: "duplicate sibling name ignores case and space", folders: []Folder{{ID: "a", Kind: FolderNode, DisplayName: " Shared "}, {ID: "b", Kind: FolderNode, DisplayName: "shared"}}, wantErr: "sibling folder name"},
		{name: "missing parent", folders: []Folder{{ID: "a", Kind: FolderNode, ParentID: "missing", DisplayName: "A"}}, wantErr: "does not exist"},
		{name: "mixed kinds", folders: []Folder{{ID: "a", Kind: FolderNode, DisplayName: "A"}, {ID: "b", Kind: FolderTask, ParentID: "a", DisplayName: "B"}}, wantErr: "same kind"},
		{name: "cycle", folders: []Folder{{ID: "a", Kind: FolderNode, ParentID: "b", DisplayName: "A"}, {ID: "b", Kind: FolderNode, ParentID: "a", DisplayName: "B"}}, wantErr: "cycle"},
	}
	chain := make([]Folder, MaxFolderDepth+1)
	for i := range chain {
		chain[i] = Folder{ID: string(rune('a' + i)), Kind: FolderNode, DisplayName: string(rune('A' + i))}
		if i > 0 {
			chain[i].ParentID = chain[i-1].ID
		}
	}
	cases = append(cases, struct {
		name    string
		folders []Folder
		wantErr string
	}{name: "too deep", folders: chain, wantErr: "exceeds maximum depth"})
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewFolderForest(tt.folders)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("NewFolderForest() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestFolderForestRequireEmpty(t *testing.T) {
	forest, err := NewFolderForest([]Folder{{ID: "root", Kind: FolderTask, DisplayName: "Root"}, {ID: "child", Kind: FolderTask, ParentID: "root", DisplayName: "Child"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := forest.RequireEmpty("child", FolderOccupancy{}); err != nil {
		t.Fatalf("empty leaf rejected: %v", err)
	}
	cases := []struct {
		name      string
		id        string
		occupancy FolderOccupancy
		wantCode  fault.Code
		text      string
	}{
		{name: "missing id", id: " ", text: "folder id is required"},
		{name: "unknown folder", id: "missing", wantCode: CodeFolderNotFound},
		{name: "negative occupancy", id: "child", occupancy: FolderOccupancy{Assets: -1}, text: "cannot be negative"},
		{name: "folder with child", id: "root", text: "must be empty"},
		{name: "folder with asset", id: "child", occupancy: FolderOccupancy{Assets: 1}, text: "must be empty"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := forest.RequireEmpty(tt.id, tt.occupancy)
			if err == nil || (tt.wantCode != "" && !fault.IsCode(err, tt.wantCode)) || (tt.text != "" && !strings.Contains(err.Error(), tt.text)) {
				t.Errorf("RequireEmpty(%q, %+v) error = %v", tt.id, tt.occupancy, err)
			}
		})
	}
}

func TestHealerPolicyValidationMatrix(t *testing.T) {
	if err := (HealerPolicySnapshotV1{}).Validate(); err != nil {
		t.Fatalf("zero policy should normalize to defaults: %v", err)
	}
	base := DefaultHealerPolicySnapshotV1()
	cases := []struct {
		name   string
		mutate func(*HealerPolicySnapshotV1)
		want   string
	}{
		{name: "unsupported version", mutate: func(p *HealerPolicySnapshotV1) { p.Version = 2 }, want: "unsupported"},
		{name: "nan threshold", mutate: func(p *HealerPolicySnapshotV1) { p.ReviewCap = math.NaN() }, want: "thresholds"},
		{name: "threshold order", mutate: func(p *HealerPolicySnapshotV1) { p.ReviewCap = p.AppliedCap }, want: "lower"},
		{name: "negative weight", mutate: func(p *HealerPolicySnapshotV1) { p.Weights.Tag = -1 }, want: "non-negative"},
		{name: "nan weight", mutate: func(p *HealerPolicySnapshotV1) { p.Weights.Tag = math.NaN() }, want: "finite"},
		{name: "all zero weights", mutate: func(p *HealerPolicySnapshotV1) { p.Weights = HealerWeightsSnapshotV1{} }, want: "at least one"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			policy := base
			tt.mutate(&policy)
			if err := policy.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestScreenshotPolicyNormalizationAndValidation(t *testing.T) {
	policy := NewScreenshotPolicy(true, "  artifacts/run  ")
	if policy.Destination != "artifacts/run" || policy.Validate() != nil {
		t.Fatalf("NewScreenshotPolicy() = %+v", policy)
	}
	if err := NewScreenshotPolicy(true, " \t ").Validate(); err == nil {
		t.Fatal("enabled policy without destination accepted")
	}
	if err := (ScreenshotPolicy{Destination: " ignored "}).Validate(); err != nil {
		t.Fatalf("disabled policy rejected: %v", err)
	}
}
