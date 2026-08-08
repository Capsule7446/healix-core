package automation

import (
	"context"
	"fmt"
	"strings"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

// FlowFragmentService 编排流程片段聚合的创建、元数据更新、版本发布和生命周期变更。
type FlowFragmentService struct{ repository FlowFragmentRepository }

// NewFlowFragmentService 构造流程片段服务；仓储为 nil 时由具体操作返回配置错误。
func NewFlowFragmentService(repository FlowFragmentRepository) FlowFragmentService {
	return FlowFragmentService{repository: repository}
}

// Create 校验并创建流程片段，再通过仓储持久化；领域错误保持原错误码。
func (s FlowFragmentService) Create(ctx context.Context, workflow domain.FlowFragment, initial domain.FlowFragmentVersion) (domain.FlowFragmentAggregate, error) {
	if isNilDependency(s.repository) {
		return domain.FlowFragmentAggregate{}, AutomationConfigurationError()
	}
	aggregate, err := domain.NewFlowFragment(workflow, initial)
	if err != nil {
		// 领域构造器已返回注册错误码，此处保持原错误，避免丢失分类。
		return domain.FlowFragmentAggregate{}, err
	}
	result, err := s.repository.Create(ctx, aggregate)
	if err != nil {
		return domain.FlowFragmentAggregate{}, fmt.Errorf("persist workflow: %w", err)
	}
	return result, nil
}

// Update 读取流程片段并按期望修订更新元数据。
func (s FlowFragmentService) Update(ctx context.Context, id, displayName, folderID string, properties domain.Properties, expected domain.Revision, at int64) (domain.FlowFragmentAggregate, error) {
	return s.transition(ctx, id, expected, func(a domain.FlowFragmentAggregate) (domain.FlowFragmentAggregate, error) {
		return a.UpdateMetadata(displayName, folderID, properties, at)
	})
}

// PublishVersion 读取流程片段并按期望修订发布不可变版本内容，通过 CAS 保存结果。
func (s FlowFragmentService) PublishVersion(ctx context.Context, id, versionID string, definition domain.FlowFragmentContent, expected domain.Revision, at int64) (domain.FlowFragmentAggregate, error) {
	return s.transition(ctx, id, expected, func(a domain.FlowFragmentAggregate) (domain.FlowFragmentAggregate, error) {
		return a.PublishVersion(versionID, definition, at)
	})
}

// Delete 读取流程片段并按期望修订标记删除。
func (s FlowFragmentService) Delete(ctx context.Context, id string, expected domain.Revision, at int64) (domain.FlowFragmentAggregate, error) {
	return s.transition(ctx, id, expected, func(a domain.FlowFragmentAggregate) (domain.FlowFragmentAggregate, error) { return a.Delete(at) })
}

// Restore 读取流程片段并按期望修订清除删除标记。
func (s FlowFragmentService) Restore(ctx context.Context, id string, expected domain.Revision, at int64) (domain.FlowFragmentAggregate, error) {
	return s.transition(ctx, id, expected, func(a domain.FlowFragmentAggregate) (domain.FlowFragmentAggregate, error) { return a.Restore(at) })
}

// transition 读取聚合、校验期望修订、应用变更并通过 CAS 保存；配置或 ID 无效时不写入。
func (s FlowFragmentService) transition(ctx context.Context, id string, expected domain.Revision, apply func(domain.FlowFragmentAggregate) (domain.FlowFragmentAggregate, error)) (domain.FlowFragmentAggregate, error) {
	if isNilDependency(s.repository) {
		return domain.FlowFragmentAggregate{}, AutomationConfigurationError()
	}
	if strings.TrimSpace(id) == "" {
		return domain.FlowFragmentAggregate{}, fmt.Errorf("workflow ID is required")
	}
	current, err := s.repository.Load(ctx, id)
	if err != nil {
		return domain.FlowFragmentAggregate{}, fmt.Errorf("load workflow %q: %w", id, err)
	}
	if current.FlowFragment.Revision != expected {
		return domain.FlowFragmentAggregate{}, AutomationRevisionConflictError()
	}
	next, err := apply(current)
	if err != nil {
		return domain.FlowFragmentAggregate{}, err
	}
	result, err := s.repository.SaveAggregate(ctx, expected, next)
	if err != nil {
		return domain.FlowFragmentAggregate{}, fmt.Errorf("persist workflow %q: %w", id, err)
	}
	return result, nil
}
