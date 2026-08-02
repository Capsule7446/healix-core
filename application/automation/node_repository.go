package automation

import (
	"context"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

type NodeRepository interface {
	Load(context.Context, string) (domain.ElementTargetAggregate, error)
	Create(context.Context, domain.ElementTargetAggregate) (domain.ElementTargetAggregate, error)
	SaveAggregate(context.Context, domain.Revision, domain.ElementTargetAggregate) (domain.ElementTargetAggregate, error)
}
