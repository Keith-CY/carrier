package credentialstore

import (
	"encoding/json"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"testing"
)

func resetHomeResolvers(t *testing.T) {
	t.Helper()
	origUserHomeDir := userHomeDirFunc
	origCurrentUser := currentUserFunc
	t.Cleanup(func() {
		userHomeDirFunc = origUserHomeDir
		currentUserFunc = origCurrentUser
	})
}

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

func TestSaveProviderCredentialCanonicalizesOpenAIAliases(t *testing.T) {
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	storePath := filepath.Join(t.TempDir(), "credentials.json")
	t.Setenv("CARRIER_CREDENTIAL_STORE", storePath)

	backend, err := SaveProviderCredential("claude-code", "alias-token")
	if err != nil {
		t.Fatalf("SaveProviderCredential error: %v", err)
	}
	if backend != "local-file" {
		t.Fatalf("backend = %q, want local-file", backend)
	}

	raw, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("read credential file: %v", err)
	}
	var payload struct {
		Providers map[string]string `json:"providers"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("parse credentials file: %v", err)
	}
	if token := payload.Providers["openai-codex"]; token != "alias-token" {
		t.Fatalf("payload openai-codex token = %q, want %q", token, "alias-token")
	}
	if _, ok := payload.Providers["claude-code"]; ok {
		t.Fatalf("expected normalized credential key, found legacy alias key")
	}
}

func TestLoadProviderCredentialPrefersCanonicalOpenAICodexEntryOrAlias(t *testing.T) {
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	storePath := filepath.Join(t.TempDir(), "credentials.json")
	t.Setenv("CARRIER_CREDENTIAL_STORE", storePath)

	if err := os.WriteFile(storePath, []byte(`{"providers":{"claude-code":"legacy-token"}}`), 0o600); err != nil {
		t.Fatalf("seed credentials file: %v", err)
	}

	value, backend, ok, err := LoadProviderCredential("openai-codex")
	if err != nil {
		t.Fatalf("LoadProviderCredential error: %v", err)
	}
	if !ok {
		t.Fatal("expected credential to load from alias fallback")
	}
	if backend != "local-file" {
		t.Fatalf("loaded backend = %q, want local-file", backend)
	}
	if value != "legacy-token" {
		t.Fatalf("value = %q, want legacy-token", value)
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

func TestCredentialStorePathFallsBackToCurrentUserHome(t *testing.T) {
	t.Setenv("CARRIER_CREDENTIAL_STORE", "")
	t.Setenv("HOME", "")
	resetHomeResolvers(t)

	userHomeDirFunc = func() (string, error) {
		return "", errors.New("$HOME is not defined")
	}
	fallbackHome := filepath.Join(t.TempDir(), "fallback-home")
	currentUserFunc = func() (*user.User, error) {
		return &user.User{HomeDir: fallbackHome}, nil
	}

	path, err := credentialStorePath()
	if err != nil {
		t.Fatalf("credentialStorePath error: %v", err)
	}
	want := filepath.Join(fallbackHome, ".carrier", "credentials.json")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestCredentialStorePathFallsBackToRootWhenHomeUnavailable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix fallback path assertion")
	}

	t.Setenv("CARRIER_CREDENTIAL_STORE", "")
	t.Setenv("HOME", "")
	t.Setenv("USER", "root")
	resetHomeResolvers(t)

	userHomeDirFunc = func() (string, error) {
		return "", errors.New("$HOME is not defined")
	}
	currentUserFunc = func() (*user.User, error) {
		return nil, errors.New("lookup user failed")
	}

	path, err := credentialStorePath()
	if err != nil {
		t.Fatalf("credentialStorePath error: %v", err)
	}
	want := filepath.Join("/root", ".carrier", "credentials.json")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}
