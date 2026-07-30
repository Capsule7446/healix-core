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

func (s TestTaskService) Create(ctx context.Context, task domain.ExecutionFlow, initial domain.ExecutionFlowVersion) (domain.ExecutionFlowAggregate, error) {
	if isNilDependency(s.repository) {
		return domain.ExecutionFlowAggregate{}, ErrAutomationConfiguration
	}
	aggregate, err := domain.NewExecutionFlow(task, initial)
	if err != nil {
		return domain.ExecutionFlowAggregate{}, fmt.Errorf("create test task: %w", err)
	}
	result, err := s.repository.Create(ctx, aggregate)
	if err != nil {
		return domain.ExecutionFlowAggregate{}, fmt.Errorf("persist test task: %w", err)
	}
	return result, nil
}

func (s TestTaskService) PublishVersion(
	ctx context.Context,
	taskID string,
	expected domain.Revision,
	publication domain.ExecutionFlowVersionPublication,
) (domain.ExecutionFlowAggregate, error) {
	if isNilDependency(s.repository) {
		return domain.ExecutionFlowAggregate{}, ErrAutomationConfiguration
	}
	if strings.TrimSpace(taskID) == "" {
		return domain.ExecutionFlowAggregate{}, fmt.Errorf("test task ID is required")
	}
	if strings.TrimSpace(publication.ID) == "" {
		return domain.ExecutionFlowAggregate{}, fmt.Errorf("test task version ID is required")
	}
	current, err := s.repository.Load(ctx, taskID)
	if err != nil {
		return domain.ExecutionFlowAggregate{}, fmt.Errorf("load test task %q: %w", taskID, err)
	}
	if current.Task.Revision != expected {
		return domain.ExecutionFlowAggregate{}, RevisionConflictError{
			AggregateKind: "test task",
			ID:            taskID,
			Expected:      expected,
			Actual:        current.Task.Revision,
		}
	}
	published, err := current.PublishVersion(publication)
	if err != nil {
		return domain.ExecutionFlowAggregate{}, fmt.Errorf("publish test task %q version: %w", taskID, err)
	}
	result, err := s.repository.SaveAggregate(ctx, expected, published)
	if err != nil {
		return domain.ExecutionFlowAggregate{}, fmt.Errorf("persist test task %q: %w", taskID, err)
	}
	return result, nil
}
