package automation

import (
	"context"
	"fmt"
	"strings"

	domain "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

type NodeService struct{ repository NodeRepository }

func NewNodeService(repository NodeRepository) NodeService {
	return NodeService{repository: repository}
}

func (s NodeService) Create(ctx context.Context, node domain.ElementTarget, initial domain.ElementTargetVersion) (domain.ElementTargetAggregate, error) {
	if isNilDependency(s.repository) {
		return domain.ElementTargetAggregate{}, AutomationConfigurationError()
	}
	aggregate, err := domain.NewElementTarget(node, initial)
	if err != nil {
		// The domain constructor already returns a registered code; wrapping it
		// here would bury that code under an unclassified layer.
		return domain.ElementTargetAggregate{}, err
	}
	result, err := s.repository.Create(ctx, aggregate)
	if err != nil {
		return domain.ElementTargetAggregate{}, fmt.Errorf("persist node: %w", err)
	}
	return result, nil
}

func (s NodeService) Update(ctx context.Context, id, displayName, folderID string, properties domain.Properties, expected domain.Revision, at int64) (domain.ElementTargetAggregate, error) {
	return s.transition(ctx, id, expected, func(a domain.ElementTargetAggregate) (domain.ElementTargetAggregate, error) {
		return a.UpdateMetadata(displayName, folderID, properties, at)
	})
}

func (s NodeService) PublishVersion(ctx context.Context, id, versionID, pageURL, origin string, selectors []fingerprint.Selector, value fingerprint.Fingerprint, source domain.VersionSource, expected domain.Revision, at int64) (domain.ElementTargetAggregate, error) {
	return s.transition(ctx, id, expected, func(a domain.ElementTargetAggregate) (domain.ElementTargetAggregate, error) {
		return a.PublishVersion(versionID, pageURL, origin, selectors, value, source, at)
	})
}

func (s NodeService) Delete(ctx context.Context, id string, expected domain.Revision, at int64) (domain.ElementTargetAggregate, error) {
	return s.transition(ctx, id, expected, func(a domain.ElementTargetAggregate) (domain.ElementTargetAggregate, error) { return a.Delete(at) })
}

func (s NodeService) Restore(ctx context.Context, id string, expected domain.Revision, at int64) (domain.ElementTargetAggregate, error) {
	return s.transition(ctx, id, expected, func(a domain.ElementTargetAggregate) (domain.ElementTargetAggregate, error) { return a.Restore(at) })
}

func (s NodeService) transition(ctx context.Context, id string, expected domain.Revision, apply func(domain.ElementTargetAggregate) (domain.ElementTargetAggregate, error)) (domain.ElementTargetAggregate, error) {
	if isNilDependency(s.repository) {
		return domain.ElementTargetAggregate{}, AutomationConfigurationError()
	}
	if strings.TrimSpace(id) == "" {
		return domain.ElementTargetAggregate{}, fmt.Errorf("node ID is required")
	}
	current, err := s.repository.Load(ctx, id)
	if err != nil {
		return domain.ElementTargetAggregate{}, fmt.Errorf("load node %q: %w", id, err)
	}
	if current.ElementTarget.Revision != expected {
		return domain.ElementTargetAggregate{}, AutomationRevisionConflictError()
	}
	next, err := apply(current)
	if err != nil {
		// The aggregate transition already returns a registered code, and the wrapper
		// this replaces also welded the element target id into public text.
		return domain.ElementTargetAggregate{}, err
	}
	result, err := s.repository.SaveAggregate(ctx, expected, next)
	if err != nil {
		return domain.ElementTargetAggregate{}, fmt.Errorf("persist node %q: %w", id, err)
	}
	return result, nil
}
