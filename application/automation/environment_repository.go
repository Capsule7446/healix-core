package automation

import (
	"context"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

// EnvironmentRepository 定义环境聚合的读取、创建和乐观并发更新端口。
type EnvironmentRepository interface {
	// Load 根据环境 ID 读取聚合；不存在或存储失败时返回错误。
	Load(context.Context, string) (domain.Environment, error)
	// Create 持久化新环境并返回宿主确认的聚合。
	Create(context.Context, domain.Environment) (domain.Environment, error)
	// Update 按期望修订执行环境聚合的并发更新并返回结果。
	Update(context.Context, domain.Revision, domain.Environment) (domain.Environment, error)
}
