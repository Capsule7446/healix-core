package automation

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	domain "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/parameter"
)

type environmentRepositoryProbe struct {
	current     domain.Environment
	loadErr     error
	updateErr   error
	loadCalls   int
	updateCalls int
	updated     domain.Environment
	expected    domain.Revision
}

func (repository *environmentRepositoryProbe) Load(context.Context, string) (domain.Environment, error) {
	repository.loadCalls++
	return repository.current, repository.loadErr
}

func (*environmentRepositoryProbe) Create(_ context.Context, value domain.Environment) (domain.Environment, error) {
	return value, nil
}

func (repository *environmentRepositoryProbe) Update(_ context.Context, expected domain.Revision, value domain.Environment) (domain.Environment, error) {
	repository.updateCalls++
	repository.expected = expected
	repository.updated = value
	if repository.updateErr != nil {
		return domain.Environment{}, repository.updateErr
	}
	return value, nil
}

func environmentUseCaseFixture(t testing.TB) domain.Environment {
	t.Helper()
	value, err := domain.NewEnvironment(domain.Environment{
		ID: "environment", DisplayName: "Environment", BaseURL: "https://example.test",
		Variables: domain.EnvironmentVariables{"region": parameter.TextValue("us")}, CreatedAt: 1, UpdatedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func deletedEnvironmentUseCaseFixture(t testing.TB) domain.Environment {
	t.Helper()
	value, err := environmentUseCaseFixture(t).Delete(2)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestEnvironmentTransitionUseCasesCoverRulesDependenciesAndState(t *testing.T) {
	dependencyFailure := errors.New("environment repository unavailable")
	tests := []struct {
		name    string
		current func(testing.TB) domain.Environment
		invoke  func(EnvironmentService, domain.Revision) (domain.Environment, error)
		assert  func(*testing.T, domain.Environment)
	}{
		{
			name: "update", current: environmentUseCaseFixture,
			invoke: func(service EnvironmentService, revision domain.Revision) (domain.Environment, error) {
				return service.Update(context.Background(), "environment", "Updated", "https://updated.test", domain.EnvironmentVariables{"region": parameter.TextValue("eu")}, revision, 3)
			},
			assert: func(t *testing.T, result domain.Environment) {
				if result.DisplayName != "Updated" || result.BaseURL != "https://updated.test" || result.Variables["region"].Text() != "eu" {
					t.Fatalf("updated environment = %#v", result)
				}
			},
		},
		{
			name: "delete", current: environmentUseCaseFixture,
			invoke: func(service EnvironmentService, revision domain.Revision) (domain.Environment, error) {
				return service.Delete(context.Background(), "environment", revision, 3)
			},
			assert: func(t *testing.T, result domain.Environment) {
				if result.DeletedAt != 3 {
					t.Fatalf("deleted environment = %#v", result)
				}
			},
		},
		{
			name: "restore", current: deletedEnvironmentUseCaseFixture,
			invoke: func(service EnvironmentService, revision domain.Revision) (domain.Environment, error) {
				return service.Restore(context.Background(), "environment", revision, 3)
			},
			assert: func(t *testing.T, result domain.Environment) {
				if result.DeletedAt != 0 {
					t.Fatalf("restored environment = %#v", result)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := test.current(t)
			repository := &environmentRepositoryProbe{current: current}
			result, err := test.invoke(NewEnvironmentService(repository), current.Revision)
			if err != nil {
				t.Fatal(err)
			}
			if repository.loadCalls != 1 || repository.updateCalls != 1 || repository.expected != current.Revision || !reflect.DeepEqual(result, repository.updated) {
				t.Fatalf("load/update/expected/result = %d/%d/%d/%#v", repository.loadCalls, repository.updateCalls, repository.expected, result)
			}
			if result.Revision != current.Revision+1 || result.UpdatedAt != 3 {
				t.Fatalf("transition result = %#v", result)
			}
			test.assert(t, result)

			repository = &environmentRepositoryProbe{current: current, loadErr: dependencyFailure}
			if _, err := test.invoke(NewEnvironmentService(repository), current.Revision); !errors.Is(err, dependencyFailure) || repository.updateCalls != 0 {
				t.Fatalf("load failure/error/update calls = %v/%d", err, repository.updateCalls)
			}

			repository = &environmentRepositoryProbe{current: current, updateErr: dependencyFailure}
			if result, err := test.invoke(NewEnvironmentService(repository), current.Revision); !errors.Is(err, dependencyFailure) || !reflect.DeepEqual(result, domain.Environment{}) || repository.updateCalls != 1 {
				t.Fatalf("persist failure/result/error/calls = %#v/%v/%d", result, err, repository.updateCalls)
			}

			repository = &environmentRepositoryProbe{current: current}
			if _, err := test.invoke(NewEnvironmentService(repository), current.Revision+1); !errors.Is(err, CodeAutomationRevisionConflict) || repository.updateCalls != 0 {
				t.Fatalf("CAS rejection/error/update calls = %v/%d", err, repository.updateCalls)
			}
		})
	}
}

func TestEnvironmentTransitionUseCasesRejectDomainNoOpsBeforePersist(t *testing.T) {
	active := environmentUseCaseFixture(t)
	deleted := deletedEnvironmentUseCaseFixture(t)
	tests := []struct {
		name    string
		current domain.Environment
		invoke  func(EnvironmentService, domain.Revision) error
		want    string
	}{
		{name: "update deleted", current: deleted, want: domain.DeletedAggregateError().Error(), invoke: func(service EnvironmentService, revision domain.Revision) error {
			_, err := service.Update(context.Background(), "environment", "Updated", "https://updated.test", domain.EnvironmentVariables{}, revision, 3)
			return err
		}},
		{name: "delete twice", current: deleted, want: "lifecycle transition is a no-op", invoke: func(service EnvironmentService, revision domain.Revision) error {
			_, err := service.Delete(context.Background(), "environment", revision, 3)
			return err
		}},
		{name: "restore active", current: active, want: "lifecycle transition is a no-op", invoke: func(service EnvironmentService, revision domain.Revision) error {
			_, err := service.Restore(context.Background(), "environment", revision, 3)
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &environmentRepositoryProbe{current: test.current}
			if err := test.invoke(NewEnvironmentService(repository), test.current.Revision); err == nil || !strings.Contains(err.Error(), test.want) || repository.updateCalls != 0 {
				t.Fatalf("error/update calls = %v/%d", err, repository.updateCalls)
			}
		})
	}
}

type folderRepositoryProbe struct {
	snapshot       FolderSnapshot
	occupancy      FolderOccupancySnapshot
	loadErr        error
	occupancyErr   error
	saveErr        error
	deleteErr      error
	loadCalls      int
	occupancyCalls int
	saveCalls      int
	deleteCalls    int
	saved          FolderSnapshot
	deleted        DeleteEmptyFolderCommand
}

func (repository *folderRepositoryProbe) Load(context.Context, domain.FolderKind) (FolderSnapshot, error) {
	repository.loadCalls++
	return repository.snapshot, repository.loadErr
}

func (repository *folderRepositoryProbe) Occupancy(context.Context, domain.FolderKind, string) (FolderOccupancySnapshot, error) {
	repository.occupancyCalls++
	return repository.occupancy, repository.occupancyErr
}

func (repository *folderRepositoryProbe) Save(_ context.Context, _ domain.FolderKind, _ domain.Revision, next FolderSnapshot) (FolderSnapshot, error) {
	repository.saveCalls++
	repository.saved = next
	if repository.saveErr != nil {
		return FolderSnapshot{}, repository.saveErr
	}
	return next, nil
}

func (repository *folderRepositoryProbe) DeleteEmptyFolder(_ context.Context, command DeleteEmptyFolderCommand) (FolderSnapshot, error) {
	repository.deleteCalls++
	repository.deleted = command
	if repository.deleteErr != nil {
		return FolderSnapshot{}, repository.deleteErr
	}
	return command.Next, nil
}

func folderUseCaseFixture(id, parent string) domain.Folder {
	return domain.Folder{ID: id, Kind: domain.FolderWorkflow, ParentID: parent, DisplayName: id, CreatedAt: 1, UpdatedAt: 1}
}

func TestFolderCreateAndMoveCoverValidationCASAndPersistence(t *testing.T) {
	dependencyFailure := errors.New("folder repository unavailable")
	parent := folderUseCaseFixture("parent", "")
	child := folderUseCaseFixture("child", "")

	t.Run("create", func(t *testing.T) {
		repository := &folderRepositoryProbe{snapshot: FolderSnapshot{Revision: 1, Folders: []domain.Folder{parent}}}
		result, err := NewFolderService(repository).Create(context.Background(), child, 1)
		if err != nil || result.Revision != 2 || repository.saveCalls != 1 || len(result.Folders) != 2 {
			t.Fatalf("result/error/save calls = %#v/%v/%d", result, err, repository.saveCalls)
		}
		invalid := child
		invalid.ID = ""
		repository = &folderRepositoryProbe{snapshot: FolderSnapshot{Revision: 1, Folders: []domain.Folder{parent}}}
		if _, err := NewFolderService(repository).Create(context.Background(), invalid, 1); err == nil || repository.saveCalls != 0 {
			t.Fatalf("invalid create/error/save calls = %v/%d", err, repository.saveCalls)
		}
	})

	t.Run("move", func(t *testing.T) {
		repository := &folderRepositoryProbe{snapshot: FolderSnapshot{Revision: 1, Folders: []domain.Folder{parent, child}}}
		result, err := NewFolderService(repository).Move(context.Background(), domain.FolderWorkflow, child.ID, parent.ID, 1, 2)
		if err != nil || result.Folders[1].ParentID != parent.ID || repository.saveCalls != 1 {
			t.Fatalf("result/error/save calls = %#v/%v/%d", result, err, repository.saveCalls)
		}
		nestedChild := child
		nestedChild.ParentID = parent.ID
		repository = &folderRepositoryProbe{snapshot: FolderSnapshot{Revision: 1, Folders: []domain.Folder{parent, nestedChild}}}
		if _, err := NewFolderService(repository).Move(context.Background(), domain.FolderWorkflow, parent.ID, child.ID, 1, 2); err == nil || repository.saveCalls != 0 {
			t.Fatalf("cycle/error/save calls = %v/%d", err, repository.saveCalls)
		}
	})

	operations := []struct {
		name   string
		invoke func(FolderService, domain.Revision) error
	}{
		{name: "create", invoke: func(service FolderService, revision domain.Revision) error {
			_, err := service.Create(context.Background(), child, revision)
			return err
		}},
		{name: "move", invoke: func(service FolderService, revision domain.Revision) error {
			_, err := service.Move(context.Background(), domain.FolderWorkflow, child.ID, parent.ID, revision, 2)
			return err
		}},
	}
	for _, operation := range operations {
		t.Run(operation.name+" dependency and CAS matrix", func(t *testing.T) {
			snapshot := FolderSnapshot{Revision: 1, Folders: []domain.Folder{parent}}
			if operation.name == "move" {
				snapshot.Folders = append(snapshot.Folders, child)
			}
			repository := &folderRepositoryProbe{snapshot: snapshot, loadErr: dependencyFailure}
			if err := operation.invoke(NewFolderService(repository), 1); !errors.Is(err, dependencyFailure) || repository.saveCalls != 0 {
				t.Fatalf("load failure/error/save calls = %v/%d", err, repository.saveCalls)
			}
			repository = &folderRepositoryProbe{snapshot: snapshot}
			if err := operation.invoke(NewFolderService(repository), 2); !errors.Is(err, CodeAutomationRevisionConflict) || repository.saveCalls != 0 {
				t.Fatalf("CAS/error/save calls = %v/%d", err, repository.saveCalls)
			}
			repository = &folderRepositoryProbe{snapshot: snapshot, saveErr: dependencyFailure}
			if err := operation.invoke(NewFolderService(repository), 1); !errors.Is(err, dependencyFailure) || repository.saveCalls != 1 {
				t.Fatalf("save failure/error/save calls = %v/%d", err, repository.saveCalls)
			}
			snapshot.Revision = domain.Revision(math.MaxUint64)
			repository = &folderRepositoryProbe{snapshot: snapshot}
			if err := operation.invoke(NewFolderService(repository), snapshot.Revision); !fault.IsCode(err, domain.CodeRevisionExhausted) || repository.saveCalls != 0 {
				t.Fatalf("overflow/error/save calls = %v/%d", err, repository.saveCalls)
			}
		})
	}
}

func TestFolderDeleteCoversEveryPrecommitRejectionAndDependency(t *testing.T) {
	failure := errors.New("folder repository unavailable")
	folder := folderUseCaseFixture("folder", "")
	valid := FolderSnapshot{Revision: 1, Folders: []domain.Folder{folder}}
	tests := []struct {
		name       string
		repository *folderRepositoryProbe
		expected   domain.Revision
		want       error
		wantCode   fault.Code
		wantText   string
	}{
		{name: "load failure", repository: &folderRepositoryProbe{snapshot: valid, loadErr: failure}, expected: 1, want: failure},
		{name: "stale forest", repository: &folderRepositoryProbe{snapshot: valid}, expected: 2, want: CodeAutomationRevisionConflict},
		{name: "invalid loaded forest", repository: &folderRepositoryProbe{snapshot: FolderSnapshot{Revision: 1, Folders: []domain.Folder{{ID: "", Kind: domain.FolderWorkflow}}}}, expected: 1, wantText: "validate folder forest"},
		{name: "occupancy failure", repository: &folderRepositoryProbe{snapshot: valid, occupancyErr: failure}, expected: 1, want: failure},
		{name: "negative occupancy", repository: &folderRepositoryProbe{snapshot: valid, occupancy: FolderOccupancySnapshot{Revision: 1, Occupancy: domain.FolderOccupancy{Assets: -1}}}, expected: 1, wantCode: domain.CodeFolderInvalid},
		{name: "occupied", repository: &folderRepositoryProbe{snapshot: valid, occupancy: FolderOccupancySnapshot{Revision: 1, Occupancy: domain.FolderOccupancy{Assets: 1}}}, expected: 1, wantCode: domain.CodeFolderNotEmpty},
		{name: "revision overflow", repository: &folderRepositoryProbe{snapshot: FolderSnapshot{Revision: domain.Revision(math.MaxUint64), Folders: []domain.Folder{folder}}}, expected: domain.Revision(math.MaxUint64), wantCode: domain.CodeRevisionExhausted},
		{name: "delete failure", repository: &folderRepositoryProbe{snapshot: valid, occupancy: FolderOccupancySnapshot{Revision: 1}, deleteErr: failure}, expected: 1, want: failure},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := NewFolderService(test.repository).Delete(context.Background(), domain.FolderWorkflow, folder.ID, test.expected)
			if err == nil || !reflect.DeepEqual(result, FolderSnapshot{}) {
				t.Fatalf("result/error = %#v/%v", result, err)
			}
			if test.want != nil && !errors.Is(err, test.want) || test.wantCode != "" && !fault.IsCode(err, test.wantCode) || test.wantText != "" && !strings.Contains(err.Error(), test.wantText) {
				t.Fatalf("error = %v, want target %v or text %q", err, test.want, test.wantText)
			}
			if test.name == "delete failure" {
				if !errors.Is(err, failure) || test.repository.deleteCalls != 1 {
					t.Fatalf("delete failure/error/calls = %v/%d", err, test.repository.deleteCalls)
				}
			} else if test.repository.deleteCalls != 0 {
				t.Fatalf("delete called before validation: %d", test.repository.deleteCalls)
			}
		})
	}
}

func TestFolderDeletePreservesUnrelatedFolders(t *testing.T) {
	target := folderUseCaseFixture("target", "")
	sibling := folderUseCaseFixture("sibling", "")
	repository := &folderRepositoryProbe{
		snapshot:  FolderSnapshot{Revision: 1, Folders: []domain.Folder{sibling, target}},
		occupancy: FolderOccupancySnapshot{Revision: 3},
	}
	result, err := NewFolderService(repository).Delete(context.Background(), domain.FolderWorkflow, target.ID, 1)
	if err != nil || result.Revision != 2 || len(result.Folders) != 1 || result.Folders[0].ID != sibling.ID {
		t.Fatalf("result/error = %#v/%v", result, err)
	}
	if repository.deleted.ExpectedOccupancyRevision != 3 || len(repository.deleted.Next.Folders) != 1 {
		t.Fatalf("delete intent = %#v", repository.deleted)
	}
}

type nodeRepositoryMatrix struct {
	current   domain.ElementTargetAggregate
	loadErr   error
	saveErr   error
	loadCalls int
	saveCalls int
	saved     domain.ElementTargetAggregate
}

func (repository *nodeRepositoryMatrix) Load(context.Context, string) (domain.ElementTargetAggregate, error) {
	repository.loadCalls++
	return repository.current, repository.loadErr
}

func (*nodeRepositoryMatrix) Create(_ context.Context, value domain.ElementTargetAggregate) (domain.ElementTargetAggregate, error) {
	return value, nil
}

func (repository *nodeRepositoryMatrix) SaveAggregate(_ context.Context, _ domain.Revision, value domain.ElementTargetAggregate) (domain.ElementTargetAggregate, error) {
	repository.saveCalls++
	repository.saved = value
	if repository.saveErr != nil {
		return domain.ElementTargetAggregate{}, repository.saveErr
	}
	return value, nil
}

func nodeUseCaseFixture(t testing.TB) domain.ElementTargetAggregate {
	t.Helper()
	node := domain.ElementTarget{ID: "node", DisplayName: "ElementTarget", Properties: domain.Properties{}, CreatedAt: 1, UpdatedAt: 1}
	version := domain.ElementTargetVersion{ID: "node-v1", ElementTargetID: "node", VersionNumber: 1, Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "button"}}, Fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{}}, Source: domain.SourceManual, CreatedAt: 1}
	aggregate, err := domain.NewElementTarget(node, version)
	if err != nil {
		t.Fatal(err)
	}
	return aggregate
}

func deletedNodeUseCaseFixture(t testing.TB) domain.ElementTargetAggregate {
	t.Helper()
	aggregate, err := nodeUseCaseFixture(t).Delete(2)
	if err != nil {
		t.Fatal(err)
	}
	return aggregate
}

func TestNodePublishDeleteRestoreCoverRulesDependenciesCASAndState(t *testing.T) {
	failure := errors.New("node repository unavailable")
	tests := []struct {
		name    string
		current func(testing.TB) domain.ElementTargetAggregate
		invoke  func(NodeService, domain.Revision) (domain.ElementTargetAggregate, error)
		assert  func(*testing.T, domain.ElementTargetAggregate)
	}{
		{name: "publish", current: nodeUseCaseFixture, invoke: func(service NodeService, revision domain.Revision) (domain.ElementTargetAggregate, error) {
			current := nodeUseCaseFixture(t)
			return service.PublishVersion(context.Background(), "node", "node-v2", "https://example.test", "https://example.test", current.Current.Selectors, current.Current.Fingerprint, domain.SourceManual, revision, 2)
		}, assert: func(t *testing.T, result domain.ElementTargetAggregate) {
			if result.Current.ID != "node-v2" || result.Current.VersionNumber != 2 {
				t.Fatalf("published node = %#v", result)
			}
		}},
		{name: "delete", current: nodeUseCaseFixture, invoke: func(service NodeService, revision domain.Revision) (domain.ElementTargetAggregate, error) {
			return service.Delete(context.Background(), "node", revision, 2)
		}, assert: func(t *testing.T, result domain.ElementTargetAggregate) {
			if result.ElementTarget.DeletedAt != 2 {
				t.Fatalf("deleted node = %#v", result)
			}
		}},
		{name: "restore", current: deletedNodeUseCaseFixture, invoke: func(service NodeService, revision domain.Revision) (domain.ElementTargetAggregate, error) {
			return service.Restore(context.Background(), "node", revision, 3)
		}, assert: func(t *testing.T, result domain.ElementTargetAggregate) {
			if result.ElementTarget.DeletedAt != 0 {
				t.Fatalf("restored node = %#v", result)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := test.current(t)
			repository := &nodeRepositoryMatrix{current: current}
			result, err := test.invoke(NewNodeService(repository), current.ElementTarget.Revision)
			if err != nil || repository.saveCalls != 1 || !reflect.DeepEqual(result, repository.saved) {
				t.Fatalf("result/error/save calls = %#v/%v/%d", result, err, repository.saveCalls)
			}
			test.assert(t, result)

			repository = &nodeRepositoryMatrix{current: current, loadErr: failure}
			if _, err := test.invoke(NewNodeService(repository), current.ElementTarget.Revision); !errors.Is(err, failure) || repository.saveCalls != 0 {
				t.Fatalf("load failure/error/save calls = %v/%d", err, repository.saveCalls)
			}
			repository = &nodeRepositoryMatrix{current: current}
			if _, err := test.invoke(NewNodeService(repository), current.ElementTarget.Revision+1); !errors.Is(err, CodeAutomationRevisionConflict) || repository.saveCalls != 0 {
				t.Fatalf("CAS/error/save calls = %v/%d", err, repository.saveCalls)
			}
			repository = &nodeRepositoryMatrix{current: current, saveErr: failure}
			if result, err := test.invoke(NewNodeService(repository), current.ElementTarget.Revision); !errors.Is(err, failure) || !reflect.DeepEqual(result, domain.ElementTargetAggregate{}) || repository.saveCalls != 1 {
				t.Fatalf("save failure/result/error/calls = %#v/%v/%d", result, err, repository.saveCalls)
			}
		})
	}
}

func TestNodePublishDeleteRestoreRejectDomainRulesBeforeSave(t *testing.T) {
	active := nodeUseCaseFixture(t)
	deleted := deletedNodeUseCaseFixture(t)
	tests := []struct {
		name    string
		current domain.ElementTargetAggregate
		invoke  func(NodeService, domain.Revision) error
		want    string
	}{
		{name: "publish duplicate version", current: active, want: "new version id must differ from the current version", invoke: func(service NodeService, revision domain.Revision) error {
			_, err := service.PublishVersion(context.Background(), "node", "node-v1", "https://example.test", "https://example.test", active.Current.Selectors, active.Current.Fingerprint, domain.SourceManual, revision, 2)
			return err
		}},
		{name: "delete twice", current: deleted, want: "lifecycle transition is a no-op", invoke: func(service NodeService, revision domain.Revision) error {
			_, err := service.Delete(context.Background(), "node", revision, 3)
			return err
		}},
		{name: "restore active", current: active, want: "lifecycle transition is a no-op", invoke: func(service NodeService, revision domain.Revision) error {
			_, err := service.Restore(context.Background(), "node", revision, 2)
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &nodeRepositoryMatrix{current: test.current}
			if err := test.invoke(NewNodeService(repository), test.current.ElementTarget.Revision); err == nil || !strings.Contains(err.Error(), test.want) || repository.saveCalls != 0 {
				t.Fatalf("error/save calls = %v/%d", err, repository.saveCalls)
			}
		})
	}
}

type testTaskRepositoryMatrix struct {
	current     domain.ExecutionFlowAggregate
	createErr   error
	loadErr     error
	saveErr     error
	createCalls int
	loadCalls   int
	saveCalls   int
}

func (repository *testTaskRepositoryMatrix) Load(context.Context, string) (domain.ExecutionFlowAggregate, error) {
	repository.loadCalls++
	return repository.current, repository.loadErr
}

func (repository *testTaskRepositoryMatrix) Create(_ context.Context, value domain.ExecutionFlowAggregate) (domain.ExecutionFlowAggregate, error) {
	repository.createCalls++
	if repository.createErr != nil {
		return domain.ExecutionFlowAggregate{}, repository.createErr
	}
	return value, nil
}

func (repository *testTaskRepositoryMatrix) SaveAggregate(_ context.Context, _ domain.Revision, value domain.ExecutionFlowAggregate) (domain.ExecutionFlowAggregate, error) {
	repository.saveCalls++
	if repository.saveErr != nil {
		return domain.ExecutionFlowAggregate{}, repository.saveErr
	}
	return value, nil
}

func testTaskAggregateUseCaseFixture(t testing.TB) domain.ExecutionFlowAggregate {
	t.Helper()
	task, version := testTaskFixture()
	aggregate, err := domain.NewExecutionFlow(task, version)
	if err != nil {
		t.Fatal(err)
	}
	return aggregate
}

func validTestTaskPublication() domain.ExecutionFlowVersionPublication {
	return domain.ExecutionFlowVersionPublication{ID: "task-v2", CreatedAt: 2, FailurePolicy: domain.FailurePolicyStopOnFailure, Items: []domain.ExecutionFlowItem{{ID: "item-v2", FlowFragmentID: "workflow", VersionPolicy: domain.FlowFragmentVersionLatest}}}
}

func TestTestTaskUseCasesCoverDependencyFailuresAndPrecommitRejections(t *testing.T) {
	failure := errors.New("test task repository unavailable")
	task, version := testTaskFixture()
	repository := &testTaskRepositoryMatrix{createErr: failure}
	if result, err := NewExecutionFlowService(repository).Create(context.Background(), task, version); !errors.Is(err, failure) || !reflect.DeepEqual(result, domain.ExecutionFlowAggregate{}) || repository.createCalls != 1 {
		t.Fatalf("create failure/result/error/calls = %#v/%v/%d", result, err, repository.createCalls)
	}

	current := testTaskAggregateUseCaseFixture(t)
	publication := validTestTaskPublication()
	tests := []struct {
		name        string
		taskID      string
		expected    domain.Revision
		publication domain.ExecutionFlowVersionPublication
		repository  *testTaskRepositoryMatrix
		want        error
		wantText    string
	}{
		{name: "blank task id", taskID: " ", expected: 1, publication: publication, repository: &testTaskRepositoryMatrix{current: current}, wantText: "test task ID is required"},
		{name: "blank version id", taskID: "task", expected: 1, publication: func() domain.ExecutionFlowVersionPublication { value := publication; value.ID = " "; return value }(), repository: &testTaskRepositoryMatrix{current: current}, wantText: "test task version ID is required"},
		{name: "load failure", taskID: "task", expected: 1, publication: publication, repository: &testTaskRepositoryMatrix{current: current, loadErr: failure}, want: failure},
		{name: "stale revision", taskID: "task", expected: 2, publication: publication, repository: &testTaskRepositoryMatrix{current: current}, want: CodeAutomationRevisionConflict},
		{name: "domain rejection", taskID: "task", expected: 1, publication: func() domain.ExecutionFlowVersionPublication {
			value := publication
			value.ID = "task-v1"
			return value
		}(), repository: &testTaskRepositoryMatrix{current: current}, wantText: "version id already exists"},
		{name: "save failure", taskID: "task", expected: 1, publication: publication, repository: &testTaskRepositoryMatrix{current: current, saveErr: failure}, want: failure},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := NewExecutionFlowService(test.repository).PublishVersion(context.Background(), test.taskID, test.expected, test.publication)
			if err == nil || !reflect.DeepEqual(result, domain.ExecutionFlowAggregate{}) || test.want != nil && !errors.Is(err, test.want) || test.wantText != "" && !strings.Contains(err.Error(), test.wantText) {
				t.Fatalf("result/error = %#v/%v", result, err)
			}
			if test.name != "save failure" && test.repository.saveCalls != 0 {
				t.Fatalf("save called before rejection: %d", test.repository.saveCalls)
			}
		})
	}
}

type workflowRepositoryMatrix struct {
	current     domain.FlowFragmentAggregate
	createErr   error
	loadErr     error
	saveErr     error
	createCalls int
	loadCalls   int
	saveCalls   int
	saved       domain.FlowFragmentAggregate
}

func (repository *workflowRepositoryMatrix) Load(context.Context, string) (domain.FlowFragmentAggregate, error) {
	repository.loadCalls++
	return repository.current, repository.loadErr
}

func (repository *workflowRepositoryMatrix) Create(_ context.Context, value domain.FlowFragmentAggregate) (domain.FlowFragmentAggregate, error) {
	repository.createCalls++
	if repository.createErr != nil {
		return domain.FlowFragmentAggregate{}, repository.createErr
	}
	return value, nil
}

func (repository *workflowRepositoryMatrix) SaveAggregate(_ context.Context, _ domain.Revision, value domain.FlowFragmentAggregate) (domain.FlowFragmentAggregate, error) {
	repository.saveCalls++
	repository.saved = value
	if repository.saveErr != nil {
		return domain.FlowFragmentAggregate{}, repository.saveErr
	}
	return value, nil
}

func workflowUseCaseFixture(t testing.TB) domain.FlowFragmentAggregate {
	t.Helper()
	definition := domain.FlowFragmentContent{Steps: []domain.FlowFragmentStep{{ID: "press", DisplayName: "Press", Kind: domain.StepAction, Action: "press", Value: "Enter"}}}
	workflow := domain.FlowFragment{ID: "workflow", DisplayName: "FlowFragment", Properties: domain.Properties{}, CreatedAt: 1, UpdatedAt: 1}
	version := domain.FlowFragmentVersion{ID: "workflow-v1", FlowFragmentID: "workflow", Definition: definition, CreatedAt: 1}
	aggregate, err := domain.NewFlowFragment(workflow, version)
	if err != nil {
		t.Fatal(err)
	}
	return aggregate
}

func deletedWorkflowUseCaseFixture(t testing.TB) domain.FlowFragmentAggregate {
	t.Helper()
	aggregate, err := workflowUseCaseFixture(t).Delete(2)
	if err != nil {
		t.Fatal(err)
	}
	return aggregate
}

func TestWorkflowCreateCoversValidationAndRepositoryFailure(t *testing.T) {
	failure := errors.New("workflow repository unavailable")
	current := workflowUseCaseFixture(t)
	repository := &workflowRepositoryMatrix{createErr: failure}
	result, err := NewFlowFragmentService(repository).Create(context.Background(), current.FlowFragment, current.Current)
	if !errors.Is(err, failure) || !reflect.DeepEqual(result, domain.FlowFragmentAggregate{}) || repository.createCalls != 1 {
		t.Fatalf("result/error/calls = %#v/%v/%d", result, err, repository.createCalls)
	}
	repository = &workflowRepositoryMatrix{}
	invalid := current.FlowFragment
	invalid.DisplayName = ""
	if _, err := NewFlowFragmentService(repository).Create(context.Background(), invalid, current.Current); err == nil || repository.createCalls != 0 {
		t.Fatalf("invalid create/error/calls = %v/%d", err, repository.createCalls)
	}
}

func TestWorkflowTransitionsCoverRulesDependenciesCASAndState(t *testing.T) {
	failure := errors.New("workflow repository unavailable")
	tests := []struct {
		name    string
		current func(testing.TB) domain.FlowFragmentAggregate
		invoke  func(FlowFragmentService, domain.Revision) (domain.FlowFragmentAggregate, error)
		assert  func(*testing.T, domain.FlowFragmentAggregate)
	}{
		{name: "update", current: workflowUseCaseFixture, invoke: func(service FlowFragmentService, revision domain.Revision) (domain.FlowFragmentAggregate, error) {
			return service.Update(context.Background(), "workflow", "Updated", "folder", domain.Properties{"owner": "qa"}, revision, 2)
		}, assert: func(t *testing.T, result domain.FlowFragmentAggregate) {
			if result.FlowFragment.DisplayName != "Updated" {
				t.Fatalf("updated workflow = %#v", result)
			}
		}},
		{name: "publish", current: workflowUseCaseFixture, invoke: func(service FlowFragmentService, revision domain.Revision) (domain.FlowFragmentAggregate, error) {
			return service.PublishVersion(context.Background(), "workflow", "workflow-v2", workflowUseCaseFixture(t).Current.Definition, revision, 2)
		}, assert: func(t *testing.T, result domain.FlowFragmentAggregate) {
			if result.Current.ID != "workflow-v2" || result.Current.VersionNumber != 2 {
				t.Fatalf("published workflow = %#v", result)
			}
		}},
		{name: "delete", current: workflowUseCaseFixture, invoke: func(service FlowFragmentService, revision domain.Revision) (domain.FlowFragmentAggregate, error) {
			return service.Delete(context.Background(), "workflow", revision, 2)
		}, assert: func(t *testing.T, result domain.FlowFragmentAggregate) {
			if result.FlowFragment.DeletedAt != 2 {
				t.Fatalf("deleted workflow = %#v", result)
			}
		}},
		{name: "restore", current: deletedWorkflowUseCaseFixture, invoke: func(service FlowFragmentService, revision domain.Revision) (domain.FlowFragmentAggregate, error) {
			return service.Restore(context.Background(), "workflow", revision, 3)
		}, assert: func(t *testing.T, result domain.FlowFragmentAggregate) {
			if result.FlowFragment.DeletedAt != 0 {
				t.Fatalf("restored workflow = %#v", result)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := test.current(t)
			repository := &workflowRepositoryMatrix{current: current}
			result, err := test.invoke(NewFlowFragmentService(repository), current.FlowFragment.Revision)
			if err != nil || repository.saveCalls != 1 || !reflect.DeepEqual(result, repository.saved) {
				t.Fatalf("result/error/save calls = %#v/%v/%d", result, err, repository.saveCalls)
			}
			test.assert(t, result)

			repository = &workflowRepositoryMatrix{current: current, loadErr: failure}
			if _, err := test.invoke(NewFlowFragmentService(repository), current.FlowFragment.Revision); !errors.Is(err, failure) || repository.saveCalls != 0 {
				t.Fatalf("load failure/error/save calls = %v/%d", err, repository.saveCalls)
			}
			repository = &workflowRepositoryMatrix{current: current}
			if _, err := test.invoke(NewFlowFragmentService(repository), current.FlowFragment.Revision+1); !errors.Is(err, CodeAutomationRevisionConflict) || repository.saveCalls != 0 {
				t.Fatalf("CAS/error/save calls = %v/%d", err, repository.saveCalls)
			}
			repository = &workflowRepositoryMatrix{current: current, saveErr: failure}
			if result, err := test.invoke(NewFlowFragmentService(repository), current.FlowFragment.Revision); !errors.Is(err, failure) || !reflect.DeepEqual(result, domain.FlowFragmentAggregate{}) || repository.saveCalls != 1 {
				t.Fatalf("save failure/result/error/calls = %#v/%v/%d", result, err, repository.saveCalls)
			}
		})
	}
}

func TestWorkflowTransitionsRejectDomainRulesBeforeSave(t *testing.T) {
	active := workflowUseCaseFixture(t)
	deleted := deletedWorkflowUseCaseFixture(t)
	tests := []struct {
		name    string
		current domain.FlowFragmentAggregate
		invoke  func(FlowFragmentService, domain.Revision) error
		want    string
	}{
		{name: "invalid update", current: active, want: "display name is required", invoke: func(service FlowFragmentService, revision domain.Revision) error {
			_, err := service.Update(context.Background(), "workflow", "", "", domain.Properties{}, revision, 2)
			return err
		}},
		{name: "duplicate publication", current: active, want: "new version id must differ from the current version", invoke: func(service FlowFragmentService, revision domain.Revision) error {
			_, err := service.PublishVersion(context.Background(), "workflow", "workflow-v1", active.Current.Definition, revision, 2)
			return err
		}},
		{name: "delete twice", current: deleted, want: "lifecycle transition is a no-op", invoke: func(service FlowFragmentService, revision domain.Revision) error {
			_, err := service.Delete(context.Background(), "workflow", revision, 3)
			return err
		}},
		{name: "restore active", current: active, want: "lifecycle transition is a no-op", invoke: func(service FlowFragmentService, revision domain.Revision) error {
			_, err := service.Restore(context.Background(), "workflow", revision, 2)
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &workflowRepositoryMatrix{current: test.current}
			if err := test.invoke(NewFlowFragmentService(repository), test.current.FlowFragment.Revision); err == nil || !strings.Contains(err.Error(), test.want) || repository.saveCalls != 0 {
				t.Fatalf("error/save calls = %v/%d", err, repository.saveCalls)
			}
		})
	}
}
