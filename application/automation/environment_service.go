package automation

import (
	"context"
	"fmt"
	"strings"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

// EnvironmentService 编排环境聚合的创建和生命周期变更，并通过 EnvironmentRepository 持久化。
type EnvironmentService struct {
	repository EnvironmentRepository
}

// NewEnvironmentService 构造使用给定环境仓储的服务值；仓储为 nil 时由具体操作返回配置错误。
func NewEnvironmentService(repository EnvironmentRepository) EnvironmentService {
	return EnvironmentService{repository: repository}
}

// Create 校验并创建环境，再通过仓储持久化；仓储缺失或持久化失败时返回错误，领域错误保持原错误码。
func (s EnvironmentService) Create(ctx context.Context, value domain.Environment) (domain.Environment, error) {
	if isNilDependency(s.repository) {
		return domain.Environment{}, AutomationConfigurationError()
	}
	created, err := domain.NewEnvironment(value)
	if err != nil {
		// 领域构造器已返回注册错误码，此处保持原错误，避免丢失分类。
		return domain.Environment{}, err
	}
	result, err := s.repository.Create(ctx, created)
	if err != nil {
		return domain.Environment{}, fmt.Errorf("persist environment: %w", err)
	}
	return result, nil
}

// Update 读取环境并按期望修订更新元数据；校验、并发冲突或仓储失败时返回错误。
func (s EnvironmentService) Update(ctx context.Context, id, displayName, baseURL string, variables domain.EnvironmentVariables, expected domain.Revision, at int64) (domain.Environment, error) {
	return s.transition(ctx, id, expected, func(current domain.Environment) (domain.Environment, error) {
		return current.UpdateMetadata(displayName, baseURL, variables, at)
	})
}

// Delete 读取环境并标记为已删除，使用期望修订执行并发更新。
func (s EnvironmentService) Delete(ctx context.Context, id string, expected domain.Revision, at int64) (domain.Environment, error) {
	return s.transition(ctx, id, expected, func(current domain.Environment) (domain.Environment, error) { return current.Delete(at) })
}

// Restore 读取环境并清除删除标记，使用期望修订执行并发更新。
func (s EnvironmentService) Restore(ctx context.Context, id string, expected domain.Revision, at int64) (domain.Environment, error) {
	return s.transition(ctx, id, expected, func(current domain.Environment) (domain.Environment, error) { return current.Restore(at) })
}

// transition 读取环境、校验期望修订、应用生命周期变更并写回仓储；不会在仓储缺失或 ID 无效时执行写入。
func (s EnvironmentService) transition(ctx context.Context, id string, expected domain.Revision, apply func(domain.Environment) (domain.Environment, error)) (domain.Environment, error) {
	if isNilDependency(s.repository) {
		return domain.Environment{}, AutomationConfigurationError()
	}
	if strings.TrimSpace(id) == "" {
		return domain.Environment{}, fmt.Errorf("environment ID is required")
	}
	current, err := s.repository.Load(ctx, id)
	if err != nil {
		return domain.Environment{}, fmt.Errorf("load environment %q: %w", id, err)
	}
	if current.Revision != expected {
		return domain.Environment{}, AutomationRevisionConflictError()
	}
	next, err := apply(current)
	if err != nil {
		return domain.Environment{}, err
	}
	result, err := s.repository.Update(ctx, expected, next)
	if err != nil {
		return domain.Environment{}, fmt.Errorf("persist environment %q: %w", id, err)
	}
	return result, nil
}
