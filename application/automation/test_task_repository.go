package automation

import (
	"context"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

type TestTaskRepository interface {
	Load(context.Context, string) (domain.ExecutionFlowAggregate, error)
	Create(context.Context, domain.ExecutionFlowAggregate) (domain.ExecutionFlowAggregate, error)
	SaveAggregate(context.Context, domain.Revision, domain.ExecutionFlowAggregate) (domain.ExecutionFlowAggregate, error)
}
