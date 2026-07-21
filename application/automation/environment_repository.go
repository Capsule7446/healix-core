package automation

import (
	"context"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

type EnvironmentRepository interface {
	Load(context.Context, string) (domain.Environment, error)
	Create(context.Context, domain.Environment) (domain.Environment, error)
	Update(context.Context, domain.Revision, domain.Environment) (domain.Environment, error)
}
