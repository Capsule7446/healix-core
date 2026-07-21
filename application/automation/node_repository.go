package automation

import (
	"context"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

type NodeRepository interface {
	Load(context.Context, string) (domain.NodeAggregate, error)
	Create(context.Context, domain.NodeAggregate) (domain.NodeAggregate, error)
	SaveAggregate(context.Context, domain.Revision, domain.NodeAggregate) (domain.NodeAggregate, error)
}
