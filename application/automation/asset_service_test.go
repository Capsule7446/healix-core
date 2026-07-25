package automation

import (
	"context"
	"errors"
	"strings"
	"testing"

	domain "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

type nodeRepositoryFake struct {
	current      domain.NodeAggregate
	expected     domain.Revision
	loadErr      error
	createErr    error
	saveErr      error
	createCalls  int
	saveCalls    int
	saveInput    domain.NodeAggregate
	saveExpected domain.Revision
}

func (f *nodeRepositoryFake) Load(context.Context, string) (domain.NodeAggregate, error) {
	return f.current, f.loadErr
}
func (f *nodeRepositoryFake) Create(_ context.Context, value domain.NodeAggregate) (domain.NodeAggregate, error) {
	f.createCalls++
	if f.createErr != nil {
		return domain.NodeAggregate{}, f.createErr
	}
	f.current = value
	return value, nil
}
func (f *nodeRepositoryFake) SaveAggregate(_ context.Context, expected domain.Revision, value domain.NodeAggregate) (domain.NodeAggregate, error) {
	f.saveCalls++
	f.saveExpected, f.saveInput = expected, value
	if f.saveErr != nil {
		return domain.NodeAggregate{}, f.saveErr
	}
	f.expected, f.current = expected, value
	return value, nil
}

type workflowRepositoryFake struct {
	current  domain.WorkflowAggregate
	expected domain.Revision
}

func (f *workflowRepositoryFake) Load(context.Context, string) (domain.WorkflowAggregate, error) {
	return f.current, nil
}
func (f *workflowRepositoryFake) Create(_ context.Context, value domain.WorkflowAggregate) (domain.WorkflowAggregate, error) {
	f.current = value
	return value, nil
}
func (f *workflowRepositoryFake) SaveAggregate(_ context.Context, expected domain.Revision, value domain.WorkflowAggregate) (domain.WorkflowAggregate, error) {
	f.expected, f.current = expected, value
	return value, nil
}

func TestRevisionConflictErrorExposesClassificationAndContext(t *testing.T) {
	err := RevisionConflictError{AggregateKind: "node", ID: "node-1", Expected: 2, Actual: 3}
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("errors.Is(%v, ErrRevisionConflict) = false", err)
	}
	for _, context := range []string{"node", "node-1", "expected 2", "actual 3"} {
		if !strings.Contains(err.Error(), context) {
			t.Fatalf("Error() = %q, missing %q", err.Error(), context)
		}
	}
}

func TestNodeServiceLifecycleAndPublication(t *testing.T) {
	repository := &nodeRepositoryFake{}
	service := NewNodeService(repository)
	node := domain.Node{ID: "node", DisplayName: "Node", Properties: domain.Properties{}, CreatedAt: 1, UpdatedAt: 1}
	version := domain.NodeVersion{ID: "node-v1", NodeID: "node", VersionNumber: 1, Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "button"}}, Fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{}}, Source: domain.SourceManual, CreatedAt: 1}
	_, err := service.Create(context.Background(), node, version)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := service.Update(context.Background(), "node", "Updated", "folder", domain.Properties{}, 1, 2)
	if err != nil || repository.expected != 1 {
		t.Fatalf("update = %#v/%v", updated, err)
	}
	published, err := service.PublishVersion(context.Background(), "node", "node-v2", "https://example.com", "https://example.com", version.Selectors, version.Fingerprint, domain.SourceManual, 2, 3)
	if err != nil || published.Current.ID != "node-v2" {
		t.Fatalf("publish = %#v/%v", published, err)
	}
	deleted, err := service.Delete(context.Background(), "node", 3, 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Restore(context.Background(), "node", deleted.Node.Revision, 5); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Update(context.Background(), "node", "x", "", domain.Properties{}, 99, 6); err == nil {
		t.Fatal("stale revision accepted")
	}
}

func TestNodeServiceRepositoryFailuresDoNotPartiallyWrite(t *testing.T) {
	sentinel := errors.New("repository unavailable")
	validNode := domain.Node{ID: "node", DisplayName: "Node", Properties: domain.Properties{}, CreatedAt: 1, UpdatedAt: 1}
	validVersion := domain.NodeVersion{ID: "node-v1", NodeID: "node", VersionNumber: 1, Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "button"}}, Fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{}}, Source: domain.SourceManual, CreatedAt: 1}

	t.Run("invalid aggregate is rejected before create", func(t *testing.T) {
		repository := &nodeRepositoryFake{}
		_, err := NewNodeService(repository).Create(context.Background(), domain.Node{}, domain.NodeVersion{})
		if err == nil || !strings.Contains(err.Error(), "create node") {
			t.Fatalf("Create() error = %v", err)
		}
		if repository.createCalls != 0 {
			t.Fatalf("Create() repository calls = %d, want 0", repository.createCalls)
		}
	})

	t.Run("create failure is returned", func(t *testing.T) {
		repository := &nodeRepositoryFake{createErr: sentinel}
		_, err := NewNodeService(repository).Create(context.Background(), validNode, validVersion)
		if !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "persist node") {
			t.Fatalf("Create() error = %v", err)
		}
	})

	t.Run("load failure prevents save", func(t *testing.T) {
		repository := &nodeRepositoryFake{loadErr: sentinel}
		_, err := NewNodeService(repository).Update(context.Background(), "node", "Updated", "", domain.Properties{}, 1, 2)
		if !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "load node") {
			t.Fatalf("Update() error = %v", err)
		}
		if repository.saveCalls != 0 {
			t.Fatalf("Update() save calls = %d, want 0", repository.saveCalls)
		}
	})

	t.Run("transition validation prevents save", func(t *testing.T) {
		aggregate, err := domain.NewNode(validNode, validVersion)
		if err != nil {
			t.Fatal(err)
		}
		repository := &nodeRepositoryFake{current: aggregate}
		_, err = NewNodeService(repository).Update(context.Background(), "node", "", "", domain.Properties{}, aggregate.Node.Revision, 2)
		if err == nil || !strings.Contains(err.Error(), "transition node") {
			t.Fatalf("Update() error = %v", err)
		}
		if repository.saveCalls != 0 {
			t.Fatalf("Update() save calls = %d, want 0", repository.saveCalls)
		}
	})

	t.Run("save failure reports error after submitting transitioned aggregate", func(t *testing.T) {
		aggregate, err := domain.NewNode(validNode, validVersion)
		if err != nil {
			t.Fatal(err)
		}
		repository := &nodeRepositoryFake{current: aggregate, saveErr: sentinel}
		_, err = NewNodeService(repository).Update(context.Background(), "node", "Updated", "", domain.Properties{}, aggregate.Node.Revision, 2)
		if !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "persist node") {
			t.Fatalf("Update() error = %v", err)
		}
		if repository.saveCalls != 1 || repository.saveExpected != aggregate.Node.Revision {
			t.Fatalf("SaveAggregate() calls/expected = %d/%d", repository.saveCalls, repository.saveExpected)
		}
		if repository.saveInput.Node.DisplayName != "Updated" || repository.saveInput.Node.Revision != aggregate.Node.Revision+1 {
			t.Fatalf("SaveAggregate() input = %#v", repository.saveInput.Node)
		}
	})
}

func TestWorkflowServiceLifecycleAndPublication(t *testing.T) {
	repository := &workflowRepositoryFake{}
	service := NewWorkflowService(repository)
	definition := domain.WorkflowDefinition{Steps: []domain.WorkflowStep{{ID: "press", DisplayName: "Press", Kind: domain.StepAction, Action: "press", Value: "Enter"}}}
	workflow := domain.Workflow{ID: "workflow", DisplayName: "Workflow", Properties: domain.Properties{}, CreatedAt: 1, UpdatedAt: 1}
	version := domain.WorkflowVersion{ID: "workflow-v1", WorkflowID: "workflow", VersionNumber: 1, Definition: definition, CreatedAt: 1}
	if _, err := service.Create(context.Background(), workflow, version); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Update(context.Background(), "workflow", "Updated", "folder", domain.Properties{}, 1, 2); err != nil {
		t.Fatal(err)
	}
	published, err := service.PublishVersion(context.Background(), "workflow", "workflow-v2", definition, 2, 3)
	if err != nil || published.Current.ID != "workflow-v2" {
		t.Fatalf("publish = %#v/%v", published, err)
	}
	deleted, err := service.Delete(context.Background(), "workflow", 3, 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Restore(context.Background(), "workflow", deleted.Workflow.Revision, 5); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Delete(context.Background(), "workflow", 99, 6); err == nil {
		t.Fatal("stale revision accepted")
	}
}
