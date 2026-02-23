package credentialstore

import (
	"errors"
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
