package automation

import (
	"context"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

// ExecutionFlowRepository 定义执行流程聚合的读取、创建和基于期望修订的保存端口。
type ExecutionFlowRepository interface {
	// Load 根据执行流程 ID 读取聚合；不存在或存储失败时返回错误。
	Load(context.Context, string) (domain.ExecutionFlowAggregate, error)
	// Create 持久化新执行流程聚合并返回宿主确认的值。
	Create(context.Context, domain.ExecutionFlowAggregate) (domain.ExecutionFlowAggregate, error)
	// SaveAggregate 按期望修订执行 CAS 更新，成功时返回保存后的聚合。
	SaveAggregate(context.Context, domain.Revision, domain.ExecutionFlowAggregate) (domain.ExecutionFlowAggregate, error)
}
