package automation

import (
	"context"
	"fmt"
	"strings"

	domain "github.com/Capsule7446/healix-core/domain/automation"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
)

// NodeService 编排元素目标聚合的创建、元数据更新、版本发布和生命周期变更。
type NodeService struct{ repository NodeRepository }

// NewNodeService 构造使用给定仓储的节点服务；仓储为 nil 时由具体操作返回配置错误。
func NewNodeService(repository NodeRepository) NodeService {
	return NodeService{repository: repository}
}

// Create 校验并创建元素目标聚合，再通过仓储持久化；领域错误保持原错误码。
func (s NodeService) Create(ctx context.Context, node domain.ElementTarget, initial domain.ElementTargetVersion) (domain.ElementTargetAggregate, error) {
	if isNilDependency(s.repository) {
		return domain.ElementTargetAggregate{}, AutomationConfigurationError()
	}
	aggregate, err := domain.NewElementTarget(node, initial)
	if err != nil {
		// 领域构造器已返回注册错误码，此处保持原错误，避免丢失分类。
		return domain.ElementTargetAggregate{}, err
	}
	result, err := s.repository.Create(ctx, aggregate)
	if err != nil {
		return domain.ElementTargetAggregate{}, fmt.Errorf("persist node: %w", err)
	}
	return result, nil
}

// Update 读取元素目标并按期望修订更新元数据；冲突或仓储失败时返回错误。
func (s NodeService) Update(ctx context.Context, id, displayName, folderID string, properties domain.Properties, expected domain.Revision, at int64) (domain.ElementTargetAggregate, error) {
	return s.transition(ctx, id, expected, func(a domain.ElementTargetAggregate) (domain.ElementTargetAggregate, error) {
		return a.UpdateMetadata(displayName, folderID, properties, at)
	})
}

// PublishVersion 读取元素目标并按期望修订发布新版本，保留版本身份和内容校验错误码。
func (s NodeService) PublishVersion(ctx context.Context, id, versionID, pageURL, origin string, selectors []fingerprint.Selector, value fingerprint.Fingerprint, source domain.VersionSource, expected domain.Revision, at int64) (domain.ElementTargetAggregate, error) {
	return s.transition(ctx, id, expected, func(a domain.ElementTargetAggregate) (domain.ElementTargetAggregate, error) {
		return a.PublishVersion(versionID, pageURL, origin, selectors, value, source, at)
	})
}

// Delete 读取元素目标并按期望修订标记删除。
func (s NodeService) Delete(ctx context.Context, id string, expected domain.Revision, at int64) (domain.ElementTargetAggregate, error) {
	return s.transition(ctx, id, expected, func(a domain.ElementTargetAggregate) (domain.ElementTargetAggregate, error) { return a.Delete(at) })
}

// Restore 读取元素目标并按期望修订清除删除标记。
func (s NodeService) Restore(ctx context.Context, id string, expected domain.Revision, at int64) (domain.ElementTargetAggregate, error) {
	return s.transition(ctx, id, expected, func(a domain.ElementTargetAggregate) (domain.ElementTargetAggregate, error) { return a.Restore(at) })
}

// transition 读取聚合、校验期望修订、应用变更并通过 CAS 保存；不会在配置或 ID 无效时写入。
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
		// 聚合变更已返回注册错误码，此处保持原错误并避免将元素目标身份写入公共文本。
		return domain.ElementTargetAggregate{}, err
	}
	result, err := s.repository.SaveAggregate(ctx, expected, next)
	if err != nil {
		return domain.ElementTargetAggregate{}, fmt.Errorf("persist node %q: %w", id, err)
	}
	return result, nil
}
