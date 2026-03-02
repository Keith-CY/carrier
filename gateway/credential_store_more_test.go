package gateway

import (
	"path/filepath"
	"testing"
)

func TestCredentialStoreWrappersFileBackend(t *testing.T) {
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(t.TempDir(), "credentials.json"))

	value, backend, ok, err := loadProviderCredential("openai")
	if err != nil {
		t.Fatalf("loadProviderCredential unexpected error: %v", err)
	}
	if ok {
		t.Fatalf("expected no credential yet, got ok=true")
	}
	if value != "" || backend != "" {
		t.Fatalf("expected empty return on missing credential, got value=%q backend=%q", value, backend)
	}

	savedBackend, err := saveProviderCredential("openai", "token-1")
	if err != nil {
		t.Fatalf("saveProviderCredential error: %v", err)
	}
	if savedBackend != "local-file" {
		t.Fatalf("expected local-file backend, got %q", savedBackend)
	}

	loadedValue, loadedBackend, ok, err := loadProviderCredential("openai")
	if err != nil {
		t.Fatalf("loadProviderCredential after save error: %v", err)
	}
	if !ok {
		t.Fatalf("expected credential to exist after save")
	}
	if loadedValue != "token-1" || loadedBackend != "local-file" {
		t.Fatalf("unexpected loaded credential: value=%q backend=%q", loadedValue, loadedBackend)
	}
}
