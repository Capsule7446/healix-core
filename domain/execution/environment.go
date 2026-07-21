package execution

import "errors"

// EnvironmentDescriptor is safe to expose to planning and read-side consumers.
type EnvironmentDescriptor struct {
	ID          string
	DisplayName string
	BaseURL     string
	UpdatedAt   int64
}

// CredentialReference identifies secret material without carrying its value.
type CredentialReference struct {
	Provider string
	Key      string
}

func (r CredentialReference) Validate() error {
	if r.Provider == "" || r.Key == "" {
		return errors.New("credential reference requires provider and key")
	}
	return nil
}

// SecretResolver is the execution-only boundary for obtaining credentials.
type SecretResolver interface {
	ResolveCredential(CredentialReference) (string, error)
}
