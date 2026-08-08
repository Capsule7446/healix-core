package automation

import (
	"context"
	"fmt"
	"strings"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

// ExecutionFlowService 编排执行流程的创建和版本发布。
type ExecutionFlowService struct{ repository ExecutionFlowRepository }

// NewExecutionFlowService 构造执行流程服务；仓储为 nil 时由具体操作返回配置错误。
func NewExecutionFlowService(repository ExecutionFlowRepository) ExecutionFlowService {
	return ExecutionFlowService{repository: repository}
}

// Create 校验并创建执行流程，再通过仓储持久化；领域错误保持原错误码。
func (s ExecutionFlowService) Create(ctx context.Context, task domain.ExecutionFlow, initial domain.ExecutionFlowVersion) (domain.ExecutionFlowAggregate, error) {
	if isNilDependency(s.repository) {
		return domain.ExecutionFlowAggregate{}, AutomationConfigurationError()
	}
	aggregate, err := domain.NewExecutionFlow(task, initial)
	if err != nil {
		// 领域构造器已返回注册错误码，此处保持原错误，避免丢失分类。
		return domain.ExecutionFlowAggregate{}, err
	}
	result, err := s.repository.Create(ctx, aggregate)
	if err != nil {
		return domain.ExecutionFlowAggregate{}, fmt.Errorf("persist test task: %w", err)
	}
	return result, nil
}

// PublishVersion 读取执行流程并按期望修订发布版本，通过 CAS 保存；修订冲突或仓储失败时返回错误。
func (s ExecutionFlowService) PublishVersion(
	ctx context.Context,
	taskID string,
	expected domain.Revision,
	publication domain.ExecutionFlowVersionPublication,
) (domain.ExecutionFlowAggregate, error) {
	if isNilDependency(s.repository) {
		return domain.ExecutionFlowAggregate{}, AutomationConfigurationError()
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
		return domain.ExecutionFlowAggregate{}, AutomationRevisionConflictError()
	}
	published, err := current.PublishVersion(publication)
	if err != nil {
		return domain.ExecutionFlowAggregate{}, err
	}
	result, err := s.repository.SaveAggregate(ctx, expected, published)
	if err != nil {
		return domain.ExecutionFlowAggregate{}, fmt.Errorf("persist test task %q: %w", taskID, err)
	}
	return result, nil
}
