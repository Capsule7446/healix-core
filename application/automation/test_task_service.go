package automation

import (
	"context"
	"fmt"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

type TestTaskService struct{ repository TestTaskRepository }

func NewTestTaskService(repository TestTaskRepository) TestTaskService {
	return TestTaskService{repository: repository}
}

func (s TestTaskService) Create(ctx context.Context, task domain.TestTask, initial domain.TestTaskVersion) (domain.TestTaskAggregate, error) {
	aggregate, err := domain.NewTestTask(task, initial)
	if err != nil {
		return domain.TestTaskAggregate{}, fmt.Errorf("create test task: %w", err)
	}
	result, err := s.repository.Create(ctx, aggregate)
	if err != nil {
		return domain.TestTaskAggregate{}, fmt.Errorf("persist test task: %w", err)
	}
	return result, nil
}

func (s TestTaskService) SavePublished(ctx context.Context, aggregate domain.TestTaskAggregate, expected domain.Revision) (domain.TestTaskAggregate, error) {
	current, err := s.repository.Load(ctx, aggregate.Task.ID)
	if err != nil {
		return domain.TestTaskAggregate{}, fmt.Errorf("load test task %q: %w", aggregate.Task.ID, err)
	}
	if current.Task.Revision != expected {
		return domain.TestTaskAggregate{}, RevisionConflictError{AggregateKind: "test task", ID: aggregate.Task.ID, Expected: expected, Actual: current.Task.Revision}
	}
	nextRevision, err := expected.Next()
	if err != nil {
		return domain.TestTaskAggregate{}, fmt.Errorf("advance test task revision: %w", err)
	}
	if aggregate.Task.Revision != nextRevision {
		return domain.TestTaskAggregate{}, fmt.Errorf("test task %q publication must advance revision exactly once", aggregate.Task.ID)
	}
	if err := aggregate.Validate(); err != nil {
		return domain.TestTaskAggregate{}, fmt.Errorf("validate test task publication: %w", err)
	}
	result, err := s.repository.SaveAggregate(ctx, expected, aggregate)
	if err != nil {
		return domain.TestTaskAggregate{}, fmt.Errorf("persist test task %q: %w", aggregate.Task.ID, err)
	}
	return result, nil
}

type SamplingPublicationRepository interface {
	Publish(context.Context, string, domain.SamplingPublication) (domain.SamplingPublicationResult, error)
}

type SamplingPublicationService struct{ repository SamplingPublicationRepository }

func NewSamplingPublicationService(repository SamplingPublicationRepository) SamplingPublicationService {
	return SamplingPublicationService{repository: repository}
}

func (s SamplingPublicationService) Publish(ctx context.Context, publicationID string, publication domain.SamplingPublication) (domain.SamplingPublicationResult, error) {
	if publicationID == "" {
		return domain.SamplingPublicationResult{}, fmt.Errorf("sampling publication id is required")
	}
	if err := publication.Validate(); err != nil {
		return domain.SamplingPublicationResult{}, fmt.Errorf("validate sampling publication: %w", err)
	}
	result, err := s.repository.Publish(ctx, publicationID, publication)
	if err != nil {
		return domain.SamplingPublicationResult{}, fmt.Errorf("publish sampling result: %w", err)
	}
	return result, nil
}
