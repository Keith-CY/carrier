package credentialstore

import (
	"path/filepath"
	"testing"
)

func TestSaveAndLoadProviderCredentialFileBackend(t *testing.T) {
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(t.TempDir(), "credentials.json"))

	backend, err := SaveProviderCredential("openai", "test-token")
	if err != nil {
		t.Fatalf("SaveProviderCredential error: %v", err)
	}
	if backend != "local-file" {
		t.Fatalf("backend = %q, want local-file", backend)
	}

	value, loadedBackend, ok, err := LoadProviderCredential("openai")
	if err != nil {
		t.Fatalf("LoadProviderCredential error: %v", err)
	}
	if !ok {
		t.Fatal("expected credential to exist")
	}
	if value != "test-token" {
		t.Fatalf("value = %q, want test-token", value)
	}
	if loadedBackend != "local-file" {
		t.Fatalf("loaded backend = %q, want local-file", loadedBackend)
	}
}

func TestLoadProviderCredentialMissingReturnsNotFound(t *testing.T) {
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(t.TempDir(), "credentials.json"))

	value, backend, ok, err := LoadProviderCredential("openai")
	if err != nil {
		t.Fatalf("LoadProviderCredential error: %v", err)
	}
	if ok {
		t.Fatal("expected no saved credential")
	}
	if value != "" || backend != "" {
		t.Fatalf("expected empty value/backend, got value=%q backend=%q", value, backend)
	}
}
