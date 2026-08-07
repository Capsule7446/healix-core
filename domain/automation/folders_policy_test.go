package automation

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

func assertFolderFault(t *testing.T, err error, code fault.Code, kind fault.Kind, message string) {
	t.Helper()
	descriptor, ok := fault.Describe(err)
	if !ok || descriptor.Code() != code || descriptor.Kind() != kind || descriptor.Message() != message {
		t.Fatalf("descriptor = %#v, ok = %v, error = %v", descriptor, ok, err)
	}
	if len(descriptor.Params()) != 0 || len(descriptor.Violations()) != 0 || err.Error() != string(code)+": "+message {
		t.Fatalf("unsafe folder fault: %#v, error = %v", descriptor, err)
	}
}

func TestFolderFaultContractsRejectHostileInputs(t *testing.T) {
	malformed := []string{
		" identity ",
		"identity\x00",
		"identity‮",
		string([]byte{0xff}),
		strings.Repeat("x", parameter.MaxNameBytes+1),
	}
	for _, value := range malformed {
		for _, field := range []struct {
			name string
			make func(string) Folder
		}{
			{name: "id", make: func(value string) Folder { return Folder{ID: value, Kind: FolderNode, DisplayName: "Nodes"} }},
			{name: "parent", make: func(value string) Folder {
				return Folder{ID: "folder", Kind: FolderNode, ParentID: value, DisplayName: "Nodes"}
			}},
			{name: "name", make: func(value string) Folder { return Folder{ID: "folder", Kind: FolderNode, DisplayName: value} }},
		} {
			t.Run(field.name, func(t *testing.T) {
				err := field.make(value).Validate()
				assertFolderFault(t, err, CodeFolderInvalid, fault.InvalidArgument, "automation folder is invalid")
				if strings.Contains(err.Error(), value) {
					t.Fatalf("error disclosed rejected value: %q", err.Error())
				}
			})
		}
	}

	_, err := NewFolderForest([]Folder{{ID: "folder", Kind: FolderNode, ParentID: "missing", DisplayName: "Folder"}})
	assertFolderFault(t, err, CodeFolderTreeInvalid, fault.FailedPrecondition, "automation folder tree is invalid")

	forest, err := NewFolderForest([]Folder{{ID: "folder", Kind: FolderNode, DisplayName: "Folder"}})
	if err != nil {
		t.Fatal(err)
	}
	before := forest.Depths()
	err = forest.RequireEmpty("folder", FolderOccupancy{Assets: 1})
	assertFolderFault(t, err, CodeFolderNotEmpty, fault.FailedPrecondition, "automation folder must be empty")
	if !reflect.DeepEqual(before, forest.Depths()) {
		t.Fatal("RequireEmpty mutated folder forest")
	}
}

func TestFolderValidationMatrix(t *testing.T) {
	tests := []struct {
		name   string
		folder Folder
		valid  bool
	}{
		{name: "valid root", folder: Folder{ID: "root", Kind: FolderNode, DisplayName: "Nodes"}, valid: true},
		{name: "missing id", folder: Folder{Kind: FolderNode, DisplayName: "Nodes"}},
		{name: "invalid kind", folder: Folder{ID: "root", Kind: "OTHER", DisplayName: "Nodes"}},
		{name: "missing display name", folder: Folder{ID: "root", Kind: FolderNode}},
		{name: "self parent", folder: Folder{ID: "root", Kind: FolderNode, ParentID: "root", DisplayName: "Nodes"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.folder.Validate()
			if tt.valid {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			assertFolderFault(t, err, CodeFolderInvalid, fault.InvalidArgument, "automation folder is invalid")
		})
	}

	assertFolderFault(t, FolderKind("secret-kind").Validate(), CodeFolderInvalid, fault.InvalidArgument, "automation folder is invalid")
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
	}{
		{name: "duplicate id", folders: append(valid, Folder{ID: "child", Kind: FolderWorkflow, DisplayName: "Other"})},
		{name: "duplicate sibling name ignores case", folders: []Folder{{ID: "a", Kind: FolderNode, DisplayName: "Shared"}, {ID: "b", Kind: FolderNode, DisplayName: "shared"}}},
		{name: "missing parent", folders: []Folder{{ID: "a", Kind: FolderNode, ParentID: "missing", DisplayName: "A"}}},
		{name: "mixed kinds", folders: []Folder{{ID: "a", Kind: FolderNode, DisplayName: "A"}, {ID: "b", Kind: FolderTask, ParentID: "a", DisplayName: "B"}}},
		{name: "cycle", folders: []Folder{{ID: "a", Kind: FolderNode, ParentID: "b", DisplayName: "A"}, {ID: "b", Kind: FolderNode, ParentID: "a", DisplayName: "B"}}},
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
	}{name: "too deep", folders: chain})
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewFolderForest(tt.folders)
			assertFolderFault(t, err, CodeFolderTreeInvalid, fault.FailedPrecondition, "automation folder tree is invalid")
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
		code      fault.Code
		kind      fault.Kind
		message   string
	}{
		{name: "missing id", id: " ", code: CodeFolderInvalid, kind: fault.InvalidArgument, message: "automation folder is invalid"},
		{name: "unknown folder", id: "missing", occupancy: FolderOccupancy{Assets: -1}, code: CodeFolderNotFound, kind: fault.NotFound, message: "automation folder was not found"},
		{name: "negative occupancy", id: "child", occupancy: FolderOccupancy{Assets: -1}, code: CodeFolderInvalid, kind: fault.InvalidArgument, message: "automation folder is invalid"},
		{name: "folder with child", id: "root", code: CodeFolderNotEmpty, kind: fault.FailedPrecondition, message: "automation folder must be empty"},
		{name: "folder with asset", id: "child", occupancy: FolderOccupancy{Assets: 1}, code: CodeFolderNotEmpty, kind: fault.FailedPrecondition, message: "automation folder must be empty"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := forest.RequireEmpty(tt.id, tt.occupancy)
			assertFolderFault(t, err, tt.code, tt.kind, tt.message)
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
