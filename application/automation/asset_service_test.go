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
	current      domain.ElementTargetAggregate
	expected     domain.Revision
	loadErr      error
	createErr    error
	saveErr      error
	createCalls  int
	saveCalls    int
	saveInput    domain.ElementTargetAggregate
	saveExpected domain.Revision
}

func (f *nodeRepositoryFake) Load(context.Context, string) (domain.ElementTargetAggregate, error) {
	return f.current, f.loadErr
}
func (f *nodeRepositoryFake) Create(_ context.Context, value domain.ElementTargetAggregate) (domain.ElementTargetAggregate, error) {
	f.createCalls++
	if f.createErr != nil {
		return domain.ElementTargetAggregate{}, f.createErr
	}
	f.current = value
	return value, nil
}
func (f *nodeRepositoryFake) SaveAggregate(_ context.Context, expected domain.Revision, value domain.ElementTargetAggregate) (domain.ElementTargetAggregate, error) {
	f.saveCalls++
	f.saveExpected, f.saveInput = expected, value
	if f.saveErr != nil {
		return domain.ElementTargetAggregate{}, f.saveErr
	}
	f.expected, f.current = expected, value
	return value, nil
}

type workflowRepositoryFake struct {
	current  domain.FlowFragmentAggregate
	expected domain.Revision
}

func (f *workflowRepositoryFake) Load(context.Context, string) (domain.FlowFragmentAggregate, error) {
	return f.current, nil
}
func (f *workflowRepositoryFake) Create(_ context.Context, value domain.FlowFragmentAggregate) (domain.FlowFragmentAggregate, error) {
	f.current = value
	return value, nil
}
func (f *workflowRepositoryFake) SaveAggregate(_ context.Context, expected domain.Revision, value domain.FlowFragmentAggregate) (domain.FlowFragmentAggregate, error) {
	f.expected, f.current = expected, value
	return value, nil
}

func TestRevisionConflictErrorExposesSafeClassification(t *testing.T) {
	err := AutomationRevisionConflictError()
	if !errors.Is(err, CodeAutomationRevisionConflict) {
		t.Fatalf("fault.IsCode(%v, %v) = false", err, CodeAutomationRevisionConflict)
	}
	for _, sensitive := range []string{"node", "node-1", "expected 2", "actual 3"} {
		if strings.Contains(err.Error(), sensitive) {
			t.Fatalf("Error() leaked %q: %q", sensitive, err.Error())
		}
	}
}

func TestNodeServiceLifecycleAndPublication(t *testing.T) {
	repository := &nodeRepositoryFake{}
	service := NewNodeService(repository)
	node := domain.ElementTarget{ID: "node", DisplayName: "ElementTarget", Properties: domain.Properties{}, CreatedAt: 1, UpdatedAt: 1}
	version := domain.ElementTargetVersion{ID: "node-v1", ElementTargetID: "node", VersionNumber: 1, Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "button"}}, Fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{}}, Source: domain.SourceManual, CreatedAt: 1}
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
	if _, err := service.Restore(context.Background(), "node", deleted.ElementTarget.Revision, 5); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Update(context.Background(), "node", "x", "", domain.Properties{}, 99, 6); err == nil {
		t.Fatal("stale revision accepted")
	}
}

func TestNodeServiceRepositoryFailuresDoNotPartiallyWrite(t *testing.T) {
	sentinel := errors.New("repository unavailable")
	validNode := domain.ElementTarget{ID: "node", DisplayName: "ElementTarget", Properties: domain.Properties{}, CreatedAt: 1, UpdatedAt: 1}
	validVersion := domain.ElementTargetVersion{ID: "node-v1", ElementTargetID: "node", VersionNumber: 1, Selectors: []fingerprint.Selector{{Type: fingerprint.SelectorCSS, Value: "button"}}, Fingerprint: fingerprint.Fingerprint{Tag: "button", Attributes: map[string]string{}}, Source: domain.SourceManual, CreatedAt: 1}

	t.Run("invalid aggregate is rejected before create", func(t *testing.T) {
		repository := &nodeRepositoryFake{}
		_, err := NewNodeService(repository).Create(context.Background(), domain.ElementTarget{}, domain.ElementTargetVersion{})
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
		aggregate, err := domain.NewElementTarget(validNode, validVersion)
		if err != nil {
			t.Fatal(err)
		}
		repository := &nodeRepositoryFake{current: aggregate}
		_, err = NewNodeService(repository).Update(context.Background(), "node", "", "", domain.Properties{}, aggregate.ElementTarget.Revision, 2)
		// The aggregate's own failure now propagates unwrapped; the "transition node"
		// layer also welded the element target id into public text. Its inner text is
		// still a bare error, which is the remaining domain/automation migration.
		if err == nil || !strings.Contains(err.Error(), "display name is required") {
			t.Fatalf("Update() error = %v", err)
		}
		if repository.saveCalls != 0 {
			t.Fatalf("Update() save calls = %d, want 0", repository.saveCalls)
		}
	})

	t.Run("save failure reports error after submitting transitioned aggregate", func(t *testing.T) {
		aggregate, err := domain.NewElementTarget(validNode, validVersion)
		if err != nil {
			t.Fatal(err)
		}
		repository := &nodeRepositoryFake{current: aggregate, saveErr: sentinel}
		_, err = NewNodeService(repository).Update(context.Background(), "node", "Updated", "", domain.Properties{}, aggregate.ElementTarget.Revision, 2)
		if !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "persist node") {
			t.Fatalf("Update() error = %v", err)
		}
		if repository.saveCalls != 1 || repository.saveExpected != aggregate.ElementTarget.Revision {
			t.Fatalf("SaveAggregate() calls/expected = %d/%d", repository.saveCalls, repository.saveExpected)
		}
		if repository.saveInput.ElementTarget.DisplayName != "Updated" || repository.saveInput.ElementTarget.Revision != aggregate.ElementTarget.Revision+1 {
			t.Fatalf("SaveAggregate() input = %#v", repository.saveInput.ElementTarget)
		}
	})
}

func TestWorkflowServiceLifecycleAndPublication(t *testing.T) {
	repository := &workflowRepositoryFake{}
	service := NewFlowFragmentService(repository)
	definition := domain.FlowFragmentContent{Steps: []domain.FlowFragmentStep{{ID: "press", DisplayName: "Press", Kind: domain.StepAction, Action: "press", Value: "Enter"}}}
	workflow := domain.FlowFragment{ID: "workflow", DisplayName: "FlowFragment", Properties: domain.Properties{}, CreatedAt: 1, UpdatedAt: 1}
	version := domain.FlowFragmentVersion{ID: "workflow-v1", FlowFragmentID: "workflow", VersionNumber: 1, Definition: definition, CreatedAt: 1}
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
	if _, err := service.Restore(context.Background(), "workflow", deleted.FlowFragment.Revision, 5); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Delete(context.Background(), "workflow", 99, 6); err == nil {
		t.Fatal("stale revision accepted")
	}
}
