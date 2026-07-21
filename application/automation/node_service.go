package automation

import (
	"context"
	"fmt"

	domain "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

type NodeService struct{ repository NodeRepository }

func NewNodeService(repository NodeRepository) NodeService {
	return NodeService{repository: repository}
}

func (s NodeService) Create(ctx context.Context, node domain.Node, initial domain.NodeVersion) (domain.NodeAggregate, error) {
	aggregate, err := domain.NewNode(node, initial)
	if err != nil {
		return domain.NodeAggregate{}, fmt.Errorf("create node: %w", err)
	}
	result, err := s.repository.Create(ctx, aggregate)
	if err != nil {
		return domain.NodeAggregate{}, fmt.Errorf("persist node: %w", err)
	}
	return result, nil
}

func (s NodeService) Update(ctx context.Context, id, displayName, folderID string, properties domain.Properties, expected domain.Revision, at int64) (domain.NodeAggregate, error) {
	return s.transition(ctx, id, expected, func(a domain.NodeAggregate) (domain.NodeAggregate, error) {
		return a.UpdateMetadata(displayName, folderID, properties, at)
	})
}

func (s NodeService) PublishVersion(ctx context.Context, id, versionID, pageURL, origin string, selectors []fingerprint.Selector, value fingerprint.Fingerprint, source domain.VersionSource, expected domain.Revision, at int64) (domain.NodeAggregate, error) {
	return s.transition(ctx, id, expected, func(a domain.NodeAggregate) (domain.NodeAggregate, error) {
		return a.PublishVersion(versionID, pageURL, origin, selectors, value, source, at)
	})
}

func (s NodeService) Delete(ctx context.Context, id string, expected domain.Revision, at int64) (domain.NodeAggregate, error) {
	return s.transition(ctx, id, expected, func(a domain.NodeAggregate) (domain.NodeAggregate, error) { return a.Delete(at) })
}

func (s NodeService) Restore(ctx context.Context, id string, expected domain.Revision, at int64) (domain.NodeAggregate, error) {
	return s.transition(ctx, id, expected, func(a domain.NodeAggregate) (domain.NodeAggregate, error) { return a.Restore(at) })
}

func (s NodeService) transition(ctx context.Context, id string, expected domain.Revision, apply func(domain.NodeAggregate) (domain.NodeAggregate, error)) (domain.NodeAggregate, error) {
	current, err := s.repository.Load(ctx, id)
	if err != nil {
		return domain.NodeAggregate{}, fmt.Errorf("load node %q: %w", id, err)
	}
	if current.Node.Revision != expected {
		return domain.NodeAggregate{}, RevisionConflictError{AggregateKind: "node", ID: id, Expected: expected, Actual: current.Node.Revision}
	}
	next, err := apply(current)
	if err != nil {
		return domain.NodeAggregate{}, fmt.Errorf("transition node %q: %w", id, err)
	}
	result, err := s.repository.SaveAggregate(ctx, expected, next)
	if err != nil {
		return domain.NodeAggregate{}, fmt.Errorf("persist node %q: %w", id, err)
	}
	return result, nil
}
