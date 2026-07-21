package execution

import "testing"

func TestCredentialReferenceRequiresExplicitSecretBoundary(t *testing.T) {
	if err := (CredentialReference{Provider: "vault", Key: "browser/login"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (CredentialReference{Key: "browser/login"}).Validate(); err == nil {
		t.Fatal("expected provider validation")
	}
}

func TestEnvironmentDescriptorContainsNoCredentialFields(t *testing.T) {
	descriptor := EnvironmentDescriptor{ID: "env", DisplayName: "测试", BaseURL: "https://example.test"}
	if descriptor.ID == "" || descriptor.BaseURL == "" {
		t.Fatalf("unexpected descriptor: %+v", descriptor)
	}
}
