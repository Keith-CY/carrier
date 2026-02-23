package main

import (
	"bytes"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func resetCarrierHomeResolvers(t *testing.T) {
	t.Helper()
	origUserHomeDir := carrierUserHomeDirFunc
	origCurrentUser := carrierCurrentUserFunc
	t.Cleanup(func() {
		carrierUserHomeDirFunc = origUserHomeDir
		carrierCurrentUserFunc = origCurrentUser
	})
}

func TestSaveProviderCredentialAutoWarnsAndContinuesOnStoreFailure(t *testing.T) {
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")

	invalidStorePath := filepath.Join(t.TempDir(), "credentials-dir")
	if err := os.MkdirAll(invalidStorePath, 0o755); err != nil {
		t.Fatalf("mkdir invalid store path: %v", err)
	}
	t.Setenv("CARRIER_CREDENTIAL_STORE", invalidStorePath)

	provider := choiceOption{ID: "openai-codex", Name: "OpenAI Codex"}
	var out bytes.Buffer
	if err := saveProviderCredentialAuto(&out, provider, "test-token"); err != nil {
		t.Fatalf("saveProviderCredentialAuto error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Warning: failed to save OpenAI Codex credential") {
		t.Fatalf("expected warning output, got %q", output)
	}
}

func TestResolveCarrierHomeDirUsesHomeEnv(t *testing.T) {
	resetCarrierHomeResolvers(t)
	t.Setenv("HOME", "/tmp/custom-home")

	carrierUserHomeDirFunc = func() (string, error) {
		return "", errors.New("must not be called")
	}
	carrierCurrentUserFunc = func() (*user.User, error) {
		return nil, errors.New("must not be called")
	}

	home, err := resolveCarrierHomeDir()
	if err != nil {
		t.Fatalf("resolveCarrierHomeDir error: %v", err)
	}
	if home != "/tmp/custom-home" {
		t.Fatalf("home = %q, want /tmp/custom-home", home)
	}
}

func TestResolveCarrierHomeDirFallsBackToCurrentUser(t *testing.T) {
	resetCarrierHomeResolvers(t)
	t.Setenv("HOME", "")

	carrierUserHomeDirFunc = func() (string, error) {
		return "", errors.New("$HOME is not defined")
	}
	fallbackHome := filepath.Join(t.TempDir(), "fallback-home")
	carrierCurrentUserFunc = func() (*user.User, error) {
		return &user.User{HomeDir: fallbackHome}, nil
	}

	home, err := resolveCarrierHomeDir()
	if err != nil {
		t.Fatalf("resolveCarrierHomeDir error: %v", err)
	}
	if home != fallbackHome {
		t.Fatalf("home = %q, want %q", home, fallbackHome)
	}
}

func TestResolveCarrierHomeDirFallsBackToRootWhenUnavailable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-specific fallback")
	}

	resetCarrierHomeResolvers(t)
	t.Setenv("HOME", "")
	t.Setenv("USER", "root")

	carrierUserHomeDirFunc = func() (string, error) {
		return "", errors.New("$HOME is not defined")
	}
	carrierCurrentUserFunc = func() (*user.User, error) {
		return nil, errors.New("lookup user failed")
	}

	home, err := resolveCarrierHomeDir()
	if err != nil {
		t.Fatalf("resolveCarrierHomeDir error: %v", err)
	}
	if home != "/root" {
		t.Fatalf("home = %q, want /root", home)
	}
}
