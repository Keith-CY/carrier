package gateway

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildCarrierDefaultProviderInfo_CredentialReadError(t *testing.T) {
	tmp := t.TempDir()
	storePath := filepath.Join(tmp, "credentials.json")
	if err := os.WriteFile(storePath, []byte("{malformed"), 0o600); err != nil {
		t.Fatalf("write malformed credential store: %v", err)
	}
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	t.Setenv("CARRIER_CREDENTIAL_STORE", storePath)
	writeGatewayDefaultProviderConfig(t, "openai", "openai/gpt-5.1-codex", "OPENAI_API_KEY")

	info := buildCarrierDefaultProviderInfo()
	if info["configured"] != true || info["available"] != true {
		t.Fatalf("expected configured+available true, got %+v", info)
	}
	if info["reusable"] != false || info["has_saved_credential"] != false {
		t.Fatalf("expected non-reusable when credential load fails, got %+v", info)
	}
	if info["reason"] != "failed to read saved credential" {
		t.Fatalf("expected failed-to-read reason, got %+v", info)
	}
	if _, ok := info["credential_error"].(string); !ok {
		t.Fatalf("expected credential_error message, got %+v", info)
	}
}

func TestBuildCarrierDefaultProviderInfo_NoSavedCredential(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
	writeGatewayDefaultProviderConfig(t, "openai", "openai/gpt-5.1-codex", "OPENAI_API_KEY")

	info := buildCarrierDefaultProviderInfo()
	if info["configured"] != true || info["available"] != true {
		t.Fatalf("expected configured+available true, got %+v", info)
	}
	if info["has_saved_credential"] != false || info["reusable"] != false {
		t.Fatalf("expected no saved credential metadata, got %+v", info)
	}
	if info["reason"] != "no saved credential" {
		t.Fatalf("expected no-saved-credential reason, got %+v", info)
	}
}
