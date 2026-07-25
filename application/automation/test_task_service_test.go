package automation

import (
	"context"
	"errors"
	"strings"
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

func TestTestTaskServiceCreateAndPublishVersion(t *testing.T) {
	repository := &testTaskRepositoryFake{}
	service := NewTestTaskService(repository)
	task, version := testTaskFixture()
	created, err := service.Create(context.Background(), task, version)
	if err != nil {
		t.Fatal(err)
	}
	nextVersion := domain.TestTaskVersionPublication{
		ID:            "task-v2",
		CreatedAt:     2,
		FailurePolicy: domain.FailurePolicyStopOnFailure,
		Items: []domain.TestTaskItem{{
			ID:            "item-2",
			WorkflowID:    "workflow",
			VersionPolicy: domain.WorkflowVersionLatest,
		}},
	}
	result, err := service.PublishVersion(context.Background(), "task", 1, nextVersion)
	if err != nil {
		t.Fatal(err)
	}
	if result.Current.ID != "task-v2" || result.Current.VersionNumber != 2 ||
		result.Current.SourceVersionID != "task-v1" || result.Task.Revision != 2 ||
		repository.expected != 1 {
		t.Fatalf("published = %#v", result)
	}
	if result.Versions[0].ID != created.Versions[0].ID ||
		result.Current.Items[0].TestTaskVersionID != "task-v2" ||
		result.Current.Items[0].SequenceNumber != 1 {
		t.Fatalf("publication rewrote history or failed to derive item identity: %#v", result)
	}
}

func TestTestTaskServiceRejectsInvalidAndStaleWrites(t *testing.T) {
	repository := &testTaskRepositoryFake{}
	service := NewTestTaskService(repository)
	task, version := testTaskFixture()
	if _, err := service.Create(context.Background(), domain.TestTask{}, version); err == nil {
		t.Fatal("invalid task accepted")
	}
	if _, err := service.Create(context.Background(), task, version); err != nil {
		t.Fatal(err)
	}
	candidate := domain.TestTaskVersionPublication{
		ID:            "task-v2",
		CreatedAt:     2,
		FailurePolicy: domain.FailurePolicyStopOnFailure,
		Items: []domain.TestTaskItem{{
			ID:            "item-2",
			WorkflowID:    "workflow",
			VersionPolicy: domain.WorkflowVersionLatest,
		}},
	}
	if _, err := service.PublishVersion(context.Background(), "task", 2, candidate); err == nil {
		t.Fatal("stale revision accepted")
	}
	candidate.ID = "task-v1"
	if _, err := service.PublishVersion(context.Background(), "task", 1, candidate); err == nil {
		t.Fatal("existing version identity accepted")
	}
	repository.loadErr = errors.New("load failed")
	if _, err := service.PublishVersion(context.Background(), "task", 1, candidate); err == nil {
		t.Fatal("load error swallowed")
	}
}

type samplingRepositoryFake struct {
	outcome PublishSamplingOutcome
	err     error
	called  bool
	mutate  func(*PublishSamplingIntent)
}

func (fake *samplingRepositoryFake) LookupSamplingPublication(_ context.Context, publicationID, digest string) (PublishSamplingOutcome, bool, error) {
	if fake.outcome.Status != PublishSamplingReplayed || fake.outcome.PublicationID != publicationID || fake.outcome.RequestDigest != digest {
		return PublishSamplingOutcome{}, false, nil
	}
	return fake.outcome, true, nil
}

func (fake *samplingRepositoryFake) PublishSampling(_ context.Context, intent PublishSamplingIntent) (PublishSamplingOutcome, error) {
	fake.called = true
	if fake.mutate != nil {
		fake.mutate(&intent)
	}
	outcome := fake.outcome
	if outcome.PublicationID == "" {
		outcome.PublicationID = intent.PublicationID
		outcome.RequestDigest = intent.RequestDigest
	}
	return outcome, fake.err
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

func TestPublishSamplingIntentDigestValidation(t *testing.T) {
	command := SamplingPublicationCommand{PublicationID: "publication", Publication: samplingPublicationFixture(t)}
	digest, err := SamplingPublicationRequestDigest(command)
	if err != nil {
		t.Fatal(err)
	}
	intent := PublishSamplingIntent{PublicationID: command.PublicationID, Publication: command.Publication, RequestDigest: digest}
	if err := ValidatePublishSamplingIntentDigest(intent); err != nil {
		t.Fatalf("valid digest rejected: %v", err)
	}
	intent.RequestDigest = "sha256:wrong"
	if err := ValidatePublishSamplingIntentDigest(intent); !errors.Is(err, ErrSamplingPublicationDigestMismatch) {
		t.Fatalf("digest mismatch error = %v", err)
	}
	intent.PublicationID = ""
	if err := ValidatePublishSamplingIntentDigest(intent); err == nil || !strings.Contains(err.Error(), "validate sampling publication intent") {
		t.Fatalf("invalid intent error = %v", err)
	}
}

func TestSamplingPublicationErrorsExposeStableClassification(t *testing.T) {
	identity := &SamplingPublicationIdentityConflictError{PublicationID: "publication"}
	if !errors.Is(identity, ErrSamplingPublicationIdentityConflict) || !strings.Contains(identity.Error(), "publication") {
		t.Fatalf("identity error = %v", identity)
	}
	cause := errors.New("bad outcome")
	contract := &SamplingPublicationContractError{Cause: cause}
	if !errors.Is(contract, ErrSamplingPublicationContract) || !errors.Is(contract, cause) || !strings.Contains(contract.Error(), cause.Error()) {
		t.Fatalf("contract error = %v", contract)
	}
}

func TestSamplingPublicationServiceRejectsMissingTransaction(t *testing.T) {
	_, err := NewSamplingPublicationService(nil, nil).Publish(context.Background(), SamplingPublicationCommand{PublicationID: "publication", Publication: samplingPublicationFixture(t)})
	if !errors.Is(err, ErrSamplingPublicationConfiguration) {
		t.Fatalf("Publish() error = %v", err)
	}
}

func TestSamplingPublicationServiceRejectsInvalidAdapterOutcome(t *testing.T) {
	repository := &samplingRepositoryFake{outcome: PublishSamplingOutcome{Status: "UNKNOWN"}}
	_, err := NewSamplingPublicationService(repository, nil).Publish(context.Background(), SamplingPublicationCommand{PublicationID: "publication", Publication: samplingPublicationFixture(t)})
	if !errors.Is(err, ErrSamplingPublicationContract) {
		t.Fatalf("Publish() error = %v", err)
	}
	var contract *SamplingPublicationContractError
	if !errors.As(err, &contract) || !strings.Contains(contract.Error(), "unsupported status") {
		t.Fatalf("Publish() contract context = %v", err)
	}
}

func TestSamplingPublicationServiceValidatesAndPublishes(t *testing.T) {
	repository := &samplingRepositoryFake{outcome: PublishSamplingOutcome{Status: PublishSamplingApplied, Result: domain.SamplingPublicationResult{WorkflowID: "workflow", WorkflowVersionID: "workflow-v1", VersionNumber: 1}}}
	service := NewSamplingPublicationService(repository, nil)
	result, err := service.Publish(context.Background(), SamplingPublicationCommand{PublicationID: "publication", Publication: samplingPublicationFixture(t)})
	if err != nil || result.WorkflowID != "workflow" || !repository.called {
		t.Fatalf("publish = %#v, %v", result, err)
	}
}

func TestSamplingPublicationServiceRejectsInvalidInputAndWrapsErrors(t *testing.T) {
	repository := &samplingRepositoryFake{}
	service := NewSamplingPublicationService(repository, nil)
	if _, err := service.Publish(context.Background(), SamplingPublicationCommand{Publication: samplingPublicationFixture(t)}); err == nil {
		t.Fatal("empty publication id accepted")
	}
	if _, err := service.Publish(context.Background(), SamplingPublicationCommand{PublicationID: "publication"}); err == nil {
		t.Fatal("invalid publication accepted")
	}
	repository.err = errors.New("persist failed")
	if _, err := service.Publish(context.Background(), SamplingPublicationCommand{PublicationID: "publication", Publication: samplingPublicationFixture(t)}); err == nil {
		t.Fatal("repository error swallowed")
	}
}
