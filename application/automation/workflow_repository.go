package automation

import (
	"context"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

// FlowFragmentRepository 定义流程片段聚合的读取、创建和按期望修订保存端口。
type FlowFragmentRepository interface {
	// Load 根据流程片段 ID 读取聚合；不存在或存储失败时返回错误。
	Load(context.Context, string) (domain.FlowFragmentAggregate, error)
	// Create 持久化新流程片段聚合并返回宿主确认的值。
	Create(context.Context, domain.FlowFragmentAggregate) (domain.FlowFragmentAggregate, error)
	// SaveAggregate 按期望修订执行 CAS 更新，成功时返回保存后的聚合。
	SaveAggregate(context.Context, domain.Revision, domain.FlowFragmentAggregate) (domain.FlowFragmentAggregate, error)
}
