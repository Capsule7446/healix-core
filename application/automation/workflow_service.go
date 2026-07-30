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

func (s WorkflowService) Create(ctx context.Context, workflow domain.FlowFragment, initial domain.FlowFragmentVersion) (domain.FlowFragmentAggregate, error) {
	if isNilDependency(s.repository) {
		return domain.FlowFragmentAggregate{}, ErrAutomationConfiguration
	}
	aggregate, err := domain.NewFlowFragment(workflow, initial)
	if err != nil {
		return domain.FlowFragmentAggregate{}, fmt.Errorf("create workflow: %w", err)
	}
	result, err := s.repository.Create(ctx, aggregate)
	if err != nil {
		return domain.FlowFragmentAggregate{}, fmt.Errorf("persist workflow: %w", err)
	}
	return result, nil
}

func (s WorkflowService) Update(ctx context.Context, id, displayName, folderID string, properties domain.Properties, expected domain.Revision, at int64) (domain.FlowFragmentAggregate, error) {
	return s.transition(ctx, id, expected, func(a domain.FlowFragmentAggregate) (domain.FlowFragmentAggregate, error) {
		return a.UpdateMetadata(displayName, folderID, properties, at)
	})
}

func (s WorkflowService) PublishVersion(ctx context.Context, id, versionID string, definition domain.FlowFragmentContent, expected domain.Revision, at int64) (domain.FlowFragmentAggregate, error) {
	return s.transition(ctx, id, expected, func(a domain.FlowFragmentAggregate) (domain.FlowFragmentAggregate, error) {
		return a.PublishVersion(versionID, definition, at)
	})
}

func (s WorkflowService) Delete(ctx context.Context, id string, expected domain.Revision, at int64) (domain.FlowFragmentAggregate, error) {
	return s.transition(ctx, id, expected, func(a domain.FlowFragmentAggregate) (domain.FlowFragmentAggregate, error) { return a.Delete(at) })
}

func (s WorkflowService) Restore(ctx context.Context, id string, expected domain.Revision, at int64) (domain.FlowFragmentAggregate, error) {
	return s.transition(ctx, id, expected, func(a domain.FlowFragmentAggregate) (domain.FlowFragmentAggregate, error) { return a.Restore(at) })
}

func (s WorkflowService) transition(ctx context.Context, id string, expected domain.Revision, apply func(domain.FlowFragmentAggregate) (domain.FlowFragmentAggregate, error)) (domain.FlowFragmentAggregate, error) {
	if isNilDependency(s.repository) {
		return domain.FlowFragmentAggregate{}, ErrAutomationConfiguration
	}
	if strings.TrimSpace(id) == "" {
		return domain.FlowFragmentAggregate{}, fmt.Errorf("workflow ID is required")
	}
	current, err := s.repository.Load(ctx, id)
	if err != nil {
		return domain.FlowFragmentAggregate{}, fmt.Errorf("load workflow %q: %w", id, err)
	}
	if current.FlowFragment.Revision != expected {
		return domain.FlowFragmentAggregate{}, RevisionConflictError{AggregateKind: "workflow", ID: id, Expected: expected, Actual: current.FlowFragment.Revision}
	}
	next, err := apply(current)
	if err != nil {
		return domain.FlowFragmentAggregate{}, fmt.Errorf("transition workflow %q: %w", id, err)
	}
	result, err := s.repository.SaveAggregate(ctx, expected, next)
	if err != nil {
		return domain.FlowFragmentAggregate{}, fmt.Errorf("persist workflow %q: %w", id, err)
	}
	return result, nil
}
