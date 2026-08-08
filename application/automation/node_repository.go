package automation

import (
	"context"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

// NodeRepository 定义元素目标聚合的读取、创建和基于期望修订的保存端口。
type NodeRepository interface {
	// Load 根据元素目标 ID 读取聚合；不存在或存储失败时返回错误。
	Load(context.Context, string) (domain.ElementTargetAggregate, error)
	// Create 持久化新元素目标聚合并返回宿主确认的值。
	Create(context.Context, domain.ElementTargetAggregate) (domain.ElementTargetAggregate, error)
	// SaveAggregate 按期望修订执行 CAS 更新，成功时返回保存后的聚合。
	SaveAggregate(context.Context, domain.Revision, domain.ElementTargetAggregate) (domain.ElementTargetAggregate, error)
}
