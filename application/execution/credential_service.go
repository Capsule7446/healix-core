package execution

import (
	"context"
	"fmt"
	"strings"

	domain "github.com/Capsule7446/healix-core/domain/execution"
)

type CredentialAuthorizer interface {
	AuthorizedCredential(context.Context, WorkerScope, string) (domain.CredentialReference, error)
}

type SecretProvider interface {
	ResolveCredential(context.Context, WorkerScope, domain.CredentialReference) (string, error)
}

type CredentialService struct {
	authorizer CredentialAuthorizer
	provider   SecretProvider
}

func NewCredentialService(authorizer CredentialAuthorizer, provider SecretProvider) CredentialService {
	return CredentialService{authorizer: authorizer, provider: provider}
}

func (s CredentialService) Resolve(ctx context.Context, worker WorkerScope, logicalName string) (string, error) {
	if strings.TrimSpace(worker.RunID) == "" || strings.TrimSpace(worker.ClaimToken) == "" || strings.TrimSpace(logicalName) == "" {
		return "", fmt.Errorf("credential resolution requires fenced worker and logical name")
	}
	reference, err := s.authorizer.AuthorizedCredential(ctx, worker, logicalName)
	if err != nil {
		return "", fmt.Errorf("authorize credential %q for run %q: %w", logicalName, worker.RunID, err)
	}
	if err := reference.Validate(); err != nil {
		return "", fmt.Errorf("validate authorized credential %q: %w", logicalName, err)
	}
	secret, err := s.provider.ResolveCredential(ctx, worker, reference)
	if err != nil {
		return "", fmt.Errorf("resolve authorized credential %q: %w", logicalName, err)
	}
	return secret, nil
}
