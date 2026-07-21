package automation

import (
	"context"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

type WorkflowRepository interface {
	Load(context.Context, string) (domain.WorkflowAggregate, error)
	Create(context.Context, domain.WorkflowAggregate) (domain.WorkflowAggregate, error)
	SaveAggregate(context.Context, domain.Revision, domain.WorkflowAggregate) (domain.WorkflowAggregate, error)
}
