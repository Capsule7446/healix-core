package execution

import (
	"context"
	"errors"
	"testing"

	domain "github.com/Capsule7446/healix-core/domain/execution"
)

type credentialAuthorizerFake struct {
	reference domain.CredentialReference
	err       error
	worker    WorkerScope
	name      string
}

func (fake *credentialAuthorizerFake) AuthorizedCredential(_ context.Context, worker WorkerScope, name string) (domain.CredentialReference, error) {
	fake.worker, fake.name = worker, name
	return fake.reference, fake.err
}

type secretProviderFake struct {
	secret    string
	err       error
	worker    WorkerScope
	reference domain.CredentialReference
}

func (fake *secretProviderFake) ResolveCredential(_ context.Context, worker WorkerScope, reference domain.CredentialReference) (string, error) {
	fake.worker = worker
	fake.reference = reference
	return fake.secret, fake.err
}

func TestCredentialServiceResolvesOnlyAuthorizedReference(t *testing.T) {
	reference := domain.CredentialReference{Provider: "vault", Key: "browser/login"}
	authorizer := &credentialAuthorizerFake{reference: reference}
	provider := &secretProviderFake{secret: "secret"}

	worker := WorkerScope{RunID: "run-1", ClaimToken: "claim"}
	secret, err := NewCredentialService(authorizer, provider).Resolve(context.Background(), worker, "login")
	if err != nil || secret != "secret" {
		t.Fatalf("resolve = %q, %v", secret, err)
	}
	if authorizer.worker != worker || authorizer.name != "login" || provider.worker != worker || provider.reference != reference {
		t.Fatalf("authorization boundary was not preserved")
	}
}

func TestCredentialServiceRejectsInvalidRequestsAndReferences(t *testing.T) {
	cases := []struct {
		name       string
		worker     WorkerScope
		logical    string
		authorizer *credentialAuthorizerFake
		provider   *secretProviderFake
	}{
		{name: "missing run", worker: WorkerScope{ClaimToken: "claim"}, logical: "login", authorizer: &credentialAuthorizerFake{}, provider: &secretProviderFake{}},
		{name: "missing claim", worker: WorkerScope{RunID: "run"}, logical: "login", authorizer: &credentialAuthorizerFake{}, provider: &secretProviderFake{}},
		{name: "missing logical name", worker: WorkerScope{RunID: "run", ClaimToken: "claim"}, authorizer: &credentialAuthorizerFake{}, provider: &secretProviderFake{}},
		{name: "invalid authorized reference", worker: WorkerScope{RunID: "run", ClaimToken: "claim"}, logical: "login", authorizer: &credentialAuthorizerFake{}, provider: &secretProviderFake{}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewCredentialService(test.authorizer, test.provider).Resolve(context.Background(), test.worker, test.logical); err == nil {
				t.Fatal("invalid credential request accepted")
			}
		})
	}
}

func TestCredentialServiceWrapsBoundaryErrors(t *testing.T) {
	authorizeErr := errors.New("denied")
	if _, err := NewCredentialService(&credentialAuthorizerFake{err: authorizeErr}, &secretProviderFake{}).Resolve(context.Background(), WorkerScope{RunID: "run", ClaimToken: "claim"}, "login"); !errors.Is(err, authorizeErr) {
		t.Fatalf("authorization error = %v", err)
	}
	providerErr := errors.New("vault unavailable")
	reference := domain.CredentialReference{Provider: "vault", Key: "login"}
	if _, err := NewCredentialService(&credentialAuthorizerFake{reference: reference}, &secretProviderFake{err: providerErr}).Resolve(context.Background(), WorkerScope{RunID: "run", ClaimToken: "claim"}, "login"); !errors.Is(err, providerErr) {
		t.Fatalf("provider error = %v", err)
	}
}
