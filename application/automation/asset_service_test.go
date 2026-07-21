package automation

import (
	"context"
	"testing"

	domain "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

type nodeRepositoryFake struct {
	current  domain.NodeAggregate
	expected domain.Revision
}

func (f *nodeRepositoryFake) Load(context.Context, string) (domain.NodeAggregate, error) {
	return f.current, nil
}
func (f *nodeRepositoryFake) Create(_ context.Context, value domain.NodeAggregate) (domain.NodeAggregate, error) {
	f.current = value
	return value, nil
}
func (f *nodeRepositoryFake) SaveAggregate(_ context.Context, expected domain.Revision, value domain.NodeAggregate) (domain.NodeAggregate, error) {
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
