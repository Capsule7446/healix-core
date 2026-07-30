package automation

import (
	"context"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

type WorkflowRepository interface {
	Load(context.Context, string) (domain.FlowFragmentAggregate, error)
	Create(context.Context, domain.FlowFragmentAggregate) (domain.FlowFragmentAggregate, error)
	SaveAggregate(context.Context, domain.Revision, domain.FlowFragmentAggregate) (domain.FlowFragmentAggregate, error)
}
