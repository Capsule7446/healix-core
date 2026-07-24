package automation

import (
	"context"
	"errors"
	"strings"
	"testing"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

type folderRepositoryFake struct {
	snapshot     FolderSnapshot
	occupancy    FolderOccupancySnapshot
	deleted      DeleteEmptyFolderCommand
	loadErr      error
	occupancyErr error
	saveErr      error
	deleteErr    error
}

func (fake *folderRepositoryFake) Load(context.Context, domain.FolderKind) (FolderSnapshot, error) {
	return fake.snapshot, fake.loadErr
}
func (fake *folderRepositoryFake) Occupancy(context.Context, domain.FolderKind, string) (FolderOccupancySnapshot, error) {
	return fake.occupancy, fake.occupancyErr
}
func (fake *folderRepositoryFake) Save(_ context.Context, _ domain.FolderKind, _ domain.Revision, next FolderSnapshot) (FolderSnapshot, error) {
	return next, fake.saveErr
}
func (fake *folderRepositoryFake) DeleteEmptyFolder(_ context.Context, command DeleteEmptyFolderCommand) (FolderSnapshot, error) {
	fake.deleted = command
	return command.Next, fake.deleteErr
}

func TestFolderServiceCreateAndMove(t *testing.T) {
	repository := &folderRepositoryFake{snapshot: FolderSnapshot{Revision: 1}}
	service := NewFolderService(repository)
	folder := domain.Folder{ID: "folder", Kind: domain.FolderWorkflow, DisplayName: "Flows", CreatedAt: 1, UpdatedAt: 1}
	created, err := service.Create(context.Background(), folder, 1)
	if err != nil || created.Revision != 2 || len(created.Folders) != 1 {
		t.Fatalf("create = %#v, %v", created, err)
	}
	repository.snapshot = created
	moved, err := service.Move(context.Background(), domain.FolderWorkflow, folder.ID, "parent", 2, 3)
	if err == nil {
		t.Fatal("move to missing parent accepted")
	}
	parent := domain.Folder{ID: "parent", Kind: domain.FolderWorkflow, DisplayName: "Parent", CreatedAt: 1, UpdatedAt: 1}
	repository.snapshot = FolderSnapshot{Revision: 2, Folders: []domain.Folder{parent, folder}}
	moved, err = service.Move(context.Background(), domain.FolderWorkflow, folder.ID, parent.ID, 2, 3)
	if err != nil || moved.Revision != 3 || moved.Folders[1].ParentID != parent.ID {
		t.Fatalf("move = %#v, %v", moved, err)
	}
}

func TestFolderServiceRejectsStaleRevisionAndUnknownMove(t *testing.T) {
	folder := domain.Folder{ID: "folder", Kind: domain.FolderNode, DisplayName: "Nodes", CreatedAt: 1, UpdatedAt: 1}
	repository := &folderRepositoryFake{snapshot: FolderSnapshot{Revision: 2, Folders: []domain.Folder{folder}}}
	service := NewFolderService(repository)
	if _, err := service.Create(context.Background(), folder, 1); err == nil {
		t.Fatal("stale revision accepted")
	}
	if _, err := service.Move(context.Background(), domain.FolderNode, "missing", "", 2, 3); err == nil {
		t.Fatal("unknown folder moved")
	}
}

func TestFolderServiceDeleteCarriesForestAndOccupancyIdentities(t *testing.T) {
	folder := domain.Folder{ID: "folder-7", Kind: domain.FolderWorkflow, DisplayName: "Flows", CreatedAt: 1, UpdatedAt: 1}
	repository := &folderRepositoryFake{
		snapshot:  FolderSnapshot{Revision: 4, Folders: []domain.Folder{folder}},
		occupancy: FolderOccupancySnapshot{Revision: 9},
	}
	result, err := NewFolderService(repository).Delete(context.Background(), domain.FolderWorkflow, folder.ID, 4)
	if err != nil {
		t.Fatal(err)
	}
	command := repository.deleted
	if command.Kind != domain.FolderWorkflow || command.FolderID != folder.ID || command.ExpectedForestRevision != 4 || command.ExpectedOccupancyRevision != 9 {
		t.Fatalf("delete command = %#v", command)
	}
	if result.Revision != 5 || len(result.Folders) != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestFolderServiceWrapsRepositoryFailures(t *testing.T) {
	sentinel := errors.New("repository unavailable")
	folder := domain.Folder{ID: "folder", Kind: domain.FolderNode, DisplayName: "Nodes", CreatedAt: 1, UpdatedAt: 1}
	tests := []struct {
		name string
		repo *folderRepositoryFake
		run  func(FolderService) error
		want string
	}{
		{name: "load", repo: &folderRepositoryFake{loadErr: sentinel}, run: func(s FolderService) error { _, err := s.Create(context.Background(), folder, 1); return err }, want: "load folder forest"},
		{name: "save", repo: &folderRepositoryFake{snapshot: FolderSnapshot{Revision: 1}, saveErr: sentinel}, run: func(s FolderService) error { _, err := s.Create(context.Background(), folder, 1); return err }, want: "persist NODE folder forest"},
		{name: "occupancy", repo: &folderRepositoryFake{snapshot: FolderSnapshot{Revision: 1, Folders: []domain.Folder{folder}}, occupancyErr: sentinel}, run: func(s FolderService) error {
			_, err := s.Delete(context.Background(), domain.FolderNode, folder.ID, 1)
			return err
		}, want: "load folder occupancy"},
		{name: "delete", repo: &folderRepositoryFake{snapshot: FolderSnapshot{Revision: 1, Folders: []domain.Folder{folder}}, deleteErr: sentinel}, run: func(s FolderService) error {
			_, err := s.Delete(context.Background(), domain.FolderNode, folder.ID, 1)
			return err
		}, want: "delete empty folder"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run(NewFolderService(tt.repo))
			if !errors.Is(err, sentinel) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q wrapping sentinel", err, tt.want)
			}
		})
	}
}

func TestFolderServiceDeleteRejectsOccupiedFolderBeforeCommit(t *testing.T) {
	folder := domain.Folder{ID: "folder", Kind: domain.FolderNode, DisplayName: "Nodes", CreatedAt: 1, UpdatedAt: 1}
	repository := &folderRepositoryFake{
		snapshot:  FolderSnapshot{Revision: 1, Folders: []domain.Folder{folder}},
		occupancy: FolderOccupancySnapshot{Revision: 2, Occupancy: domain.FolderOccupancy{Assets: 1}},
	}
	if _, err := NewFolderService(repository).Delete(context.Background(), domain.FolderNode, folder.ID, 1); err == nil {
		t.Fatal("occupied folder deleted")
	}
	if repository.deleted.FolderID != "" {
		t.Fatal("delete commit invoked")
	}
}
