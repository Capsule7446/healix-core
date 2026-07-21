package automation

import (
	"context"
	"errors"
	"testing"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

type testTaskRepositoryFake struct {
	current   domain.TestTaskAggregate
	loadErr   error
	createErr error
	saveErr   error
	expected  domain.Revision
}

func (fake *testTaskRepositoryFake) Load(context.Context, string) (domain.TestTaskAggregate, error) {
	return fake.current, fake.loadErr
}
func (fake *testTaskRepositoryFake) Create(_ context.Context, aggregate domain.TestTaskAggregate) (domain.TestTaskAggregate, error) {
	fake.current = aggregate
	return aggregate, fake.createErr
}
func (fake *testTaskRepositoryFake) SaveAggregate(_ context.Context, expected domain.Revision, aggregate domain.TestTaskAggregate) (domain.TestTaskAggregate, error) {
	fake.expected = expected
	fake.current = aggregate
	return aggregate, fake.saveErr
}

func testTaskFixture() (domain.TestTask, domain.TestTaskVersion) {
	task := domain.TestTask{ID: "task", DisplayName: "Task", CreatedAt: 1, UpdatedAt: 1}
	version := domain.TestTaskVersion{ID: "task-v1", TestTaskID: "task", VersionNumber: 1, CreatedAt: 1,
		FailurePolicy: domain.FailurePolicyStopOnFailure,
		Items:         []domain.TestTaskItem{{ID: "item", TestTaskVersionID: "task-v1", SequenceNumber: 1, WorkflowID: "workflow", VersionPolicy: domain.WorkflowVersionLatest}}}
	return task, version
}

func TestTestTaskServiceCreateAndSavePublished(t *testing.T) {
	repository := &testTaskRepositoryFake{}
	service := NewTestTaskService(repository)
	task, version := testTaskFixture()
	created, err := service.Create(context.Background(), task, version)
	if err != nil {
		t.Fatal(err)
	}
	nextVersion := version
	nextVersion.ID, nextVersion.VersionNumber, nextVersion.SourceVersionID, nextVersion.CreatedAt = "task-v2", 2, "task-v1", 2
	nextVersion.Items = []domain.TestTaskItem{{ID: "item-2", TestTaskVersionID: "task-v2", SequenceNumber: 1, WorkflowID: "workflow", VersionPolicy: domain.WorkflowVersionLatest}}
	published := domain.TestTaskAggregate{Task: created.Task, Current: nextVersion, Versions: append(created.Versions, nextVersion)}
	published.Task.CurrentVersionID, published.Task.UpdatedAt, published.Task.Revision = "task-v2", 2, 2
	result, err := service.SavePublished(context.Background(), published, 1)
	if err != nil || result.Current.ID != "task-v2" || repository.expected != 1 {
		t.Fatalf("published = %#v, %v", result, err)
	}
}

func TestTestTaskServiceRejectsInvalidAndStaleWrites(t *testing.T) {
	repository := &testTaskRepositoryFake{}
	service := NewTestTaskService(repository)
	task, version := testTaskFixture()
	if _, err := service.Create(context.Background(), domain.TestTask{}, version); err == nil {
		t.Fatal("invalid task accepted")
	}
	created, err := service.Create(context.Background(), task, version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SavePublished(context.Background(), created, 2); err == nil {
		t.Fatal("stale revision accepted")
	}
	if _, err := service.SavePublished(context.Background(), created, 1); err == nil {
		t.Fatal("publication without revision advance accepted")
	}
	repository.loadErr = errors.New("load failed")
	if _, err := service.SavePublished(context.Background(), created, 1); err == nil {
		t.Fatal("load error swallowed")
	}
}

type samplingRepositoryFake struct {
	result domain.SamplingPublicationResult
	err    error
	called bool
}

func (fake *samplingRepositoryFake) Publish(_ context.Context, _ string, _ domain.SamplingPublication) (domain.SamplingPublicationResult, error) {
	fake.called = true
	return fake.result, fake.err
}

func samplingPublicationFixture(t *testing.T) domain.SamplingPublication {
	t.Helper()
	workflow := domain.Workflow{ID: "workflow", DisplayName: "Workflow", Properties: domain.Properties{}, CreatedAt: 1, UpdatedAt: 1}
	version := domain.WorkflowVersion{ID: "workflow-v1", WorkflowID: "workflow", VersionNumber: 1, CreatedAt: 1,
		Definition: domain.WorkflowDefinition{Steps: []domain.WorkflowStep{{ID: "wait", DisplayName: "Wait", Kind: domain.StepWait, WaitKind: "sleep", WaitMS: 1}}}}
	aggregate, err := domain.NewWorkflow(workflow, version)
	if err != nil {
		t.Fatal(err)
	}
	return domain.SamplingPublication{Workflow: aggregate}
}

func TestSamplingPublicationServiceValidatesAndPublishes(t *testing.T) {
	repository := &samplingRepositoryFake{result: domain.SamplingPublicationResult{WorkflowID: "workflow", WorkflowVersionID: "workflow-v1", VersionNumber: 1}}
	service := NewSamplingPublicationService(repository)
	result, err := service.Publish(context.Background(), "publication", samplingPublicationFixture(t))
	if err != nil || result.WorkflowID != "workflow" || !repository.called {
		t.Fatalf("publish = %#v, %v", result, err)
	}
}

func TestSamplingPublicationServiceRejectsInvalidInputAndWrapsErrors(t *testing.T) {
	repository := &samplingRepositoryFake{}
	service := NewSamplingPublicationService(repository)
	if _, err := service.Publish(context.Background(), "", samplingPublicationFixture(t)); err == nil {
		t.Fatal("empty publication id accepted")
	}
	if _, err := service.Publish(context.Background(), "publication", domain.SamplingPublication{}); err == nil {
		t.Fatal("invalid publication accepted")
	}
	repository.err = errors.New("persist failed")
	if _, err := service.Publish(context.Background(), "publication", samplingPublicationFixture(t)); err == nil {
		t.Fatal("repository error swallowed")
	}
}
