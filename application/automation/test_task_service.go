package automation

import (
	"context"
	"fmt"
	"strings"

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

func (s TestTaskService) PublishVersion(
	ctx context.Context,
	taskID string,
	expected domain.Revision,
	publication domain.TestTaskVersionPublication,
) (domain.TestTaskAggregate, error) {
	if strings.TrimSpace(taskID) == "" {
		return domain.TestTaskAggregate{}, fmt.Errorf("test task ID is required")
	}
	if strings.TrimSpace(publication.ID) == "" {
		return domain.TestTaskAggregate{}, fmt.Errorf("test task version ID is required")
	}
	current, err := s.repository.Load(ctx, taskID)
	if err != nil {
		return domain.TestTaskAggregate{}, fmt.Errorf("load test task %q: %w", taskID, err)
	}
	if current.Task.Revision != expected {
		return domain.TestTaskAggregate{}, RevisionConflictError{
			AggregateKind: "test task",
			ID:            taskID,
			Expected:      expected,
			Actual:        current.Task.Revision,
		}
	}
	published, err := current.PublishVersion(publication)
	if err != nil {
		return domain.TestTaskAggregate{}, fmt.Errorf("publish test task %q version: %w", taskID, err)
	}
	result, err := s.repository.SaveAggregate(ctx, expected, published)
	if err != nil {
		return domain.TestTaskAggregate{}, fmt.Errorf("persist test task %q: %w", taskID, err)
	}
	return result, nil
}
