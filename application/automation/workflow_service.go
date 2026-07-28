package automation

import (
	"context"
	"fmt"
	"strings"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

type WorkflowService struct{ repository WorkflowRepository }

func NewWorkflowService(repository WorkflowRepository) WorkflowService {
	return WorkflowService{repository: repository}
}

func (s WorkflowService) Create(ctx context.Context, workflow domain.Workflow, initial domain.WorkflowVersion) (domain.WorkflowAggregate, error) {
	if isNilDependency(s.repository) {
		return domain.WorkflowAggregate{}, ErrAutomationConfiguration
	}
	aggregate, err := domain.NewWorkflow(workflow, initial)
	if err != nil {
		return domain.WorkflowAggregate{}, fmt.Errorf("create workflow: %w", err)
	}
	result, err := s.repository.Create(ctx, aggregate)
	if err != nil {
		return domain.WorkflowAggregate{}, fmt.Errorf("persist workflow: %w", err)
	}
	return result, nil
}

func (s WorkflowService) Update(ctx context.Context, id, displayName, folderID string, properties domain.Properties, expected domain.Revision, at int64) (domain.WorkflowAggregate, error) {
	return s.transition(ctx, id, expected, func(a domain.WorkflowAggregate) (domain.WorkflowAggregate, error) {
		return a.UpdateMetadata(displayName, folderID, properties, at)
	})
}

func (s WorkflowService) PublishVersion(ctx context.Context, id, versionID string, definition domain.WorkflowDefinition, expected domain.Revision, at int64) (domain.WorkflowAggregate, error) {
	return s.transition(ctx, id, expected, func(a domain.WorkflowAggregate) (domain.WorkflowAggregate, error) {
		return a.PublishVersion(versionID, definition, at)
	})
}

func (s WorkflowService) Delete(ctx context.Context, id string, expected domain.Revision, at int64) (domain.WorkflowAggregate, error) {
	return s.transition(ctx, id, expected, func(a domain.WorkflowAggregate) (domain.WorkflowAggregate, error) { return a.Delete(at) })
}

func (s WorkflowService) Restore(ctx context.Context, id string, expected domain.Revision, at int64) (domain.WorkflowAggregate, error) {
	return s.transition(ctx, id, expected, func(a domain.WorkflowAggregate) (domain.WorkflowAggregate, error) { return a.Restore(at) })
}

func (s WorkflowService) transition(ctx context.Context, id string, expected domain.Revision, apply func(domain.WorkflowAggregate) (domain.WorkflowAggregate, error)) (domain.WorkflowAggregate, error) {
	if isNilDependency(s.repository) {
		return domain.WorkflowAggregate{}, ErrAutomationConfiguration
	}
	if strings.TrimSpace(id) == "" {
		return domain.WorkflowAggregate{}, fmt.Errorf("workflow ID is required")
	}
	current, err := s.repository.Load(ctx, id)
	if err != nil {
		return domain.WorkflowAggregate{}, fmt.Errorf("load workflow %q: %w", id, err)
	}
	if current.Workflow.Revision != expected {
		return domain.WorkflowAggregate{}, RevisionConflictError{AggregateKind: "workflow", ID: id, Expected: expected, Actual: current.Workflow.Revision}
	}
	next, err := apply(current)
	if err != nil {
		return domain.WorkflowAggregate{}, fmt.Errorf("transition workflow %q: %w", id, err)
	}
	result, err := s.repository.SaveAggregate(ctx, expected, next)
	if err != nil {
		return domain.WorkflowAggregate{}, fmt.Errorf("persist workflow %q: %w", id, err)
	}
	return result, nil
}
