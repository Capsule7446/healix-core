package automation

import (
	"context"
	"fmt"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

type EnvironmentService struct {
	repository EnvironmentRepository
}

func NewEnvironmentService(repository EnvironmentRepository) EnvironmentService {
	return EnvironmentService{repository: repository}
}

func (s EnvironmentService) Create(ctx context.Context, value domain.Environment) (domain.Environment, error) {
	created, err := domain.NewEnvironment(value)
	if err != nil {
		return domain.Environment{}, fmt.Errorf("create environment: %w", err)
	}
	result, err := s.repository.Create(ctx, created)
	if err != nil {
		return domain.Environment{}, fmt.Errorf("persist environment: %w", err)
	}
	return result, nil
}

func (s EnvironmentService) Update(ctx context.Context, id, displayName, baseURL string, variables, properties domain.Properties, expected domain.Revision, at int64) (domain.Environment, error) {
	return s.transition(ctx, id, expected, func(current domain.Environment) (domain.Environment, error) {
		return current.UpdateMetadata(displayName, baseURL, variables, properties, at)
	})
}

func (s EnvironmentService) Delete(ctx context.Context, id string, expected domain.Revision, at int64) (domain.Environment, error) {
	return s.transition(ctx, id, expected, func(current domain.Environment) (domain.Environment, error) { return current.Delete(at) })
}

func (s EnvironmentService) Restore(ctx context.Context, id string, expected domain.Revision, at int64) (domain.Environment, error) {
	return s.transition(ctx, id, expected, func(current domain.Environment) (domain.Environment, error) { return current.Restore(at) })
}

func (s EnvironmentService) transition(ctx context.Context, id string, expected domain.Revision, apply func(domain.Environment) (domain.Environment, error)) (domain.Environment, error) {
	current, err := s.repository.Load(ctx, id)
	if err != nil {
		return domain.Environment{}, fmt.Errorf("load environment %q: %w", id, err)
	}
	if current.Revision != expected {
		return domain.Environment{}, RevisionConflictError{AggregateKind: "environment", ID: id, Expected: expected, Actual: current.Revision}
	}
	next, err := apply(current)
	if err != nil {
		return domain.Environment{}, fmt.Errorf("transition environment %q: %w", id, err)
	}
	result, err := s.repository.Update(ctx, expected, next)
	if err != nil {
		return domain.Environment{}, fmt.Errorf("persist environment %q: %w", id, err)
	}
	return result, nil
}
