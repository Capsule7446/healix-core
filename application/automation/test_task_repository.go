package automation

import (
	"context"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

type TestTaskRepository interface {
	Load(context.Context, string) (domain.TestTaskAggregate, error)
	Create(context.Context, domain.TestTaskAggregate) (domain.TestTaskAggregate, error)
	SaveAggregate(context.Context, domain.Revision, domain.TestTaskAggregate) (domain.TestTaskAggregate, error)
}
