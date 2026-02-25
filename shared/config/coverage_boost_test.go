package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadReadErrorForDirectoryPath(t *testing.T) {
	_, err := Load(t.TempDir())
	if err == nil {
		t.Fatal("expected read error for directory path")
	}
	if !strings.Contains(err.Error(), "config: read") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckConfigPermissionsStatError(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			APIToken: "secret-token",
		},
	}
	missingPath := filepath.Join(t.TempDir(), "missing-config.json")

	_, err := os.Stat(missingPath)
	if err == nil {
		t.Fatal("expected missing file in precondition")
	}

	err = checkConfigPermissions(missingPath, cfg)
	if err == nil {
		t.Fatal("expected stat error")
	}
	if !strings.Contains(err.Error(), "config: stat") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyEnvOverridesIgnoresInvalidNumericValues(t *testing.T) {
	cfg := Default()
	t.Setenv("CARRIER_SERVER_PORT", "0")
	t.Setenv("CARRIER_CRASH_THRESHOLD", "-1")
	t.Setenv("CARRIER_SERVER_API_TOKEN", "token-from-env")

	applyEnvOverrides(&cfg)

	if cfg.Server.Port != 9090 {
		t.Fatalf("expected default port to remain, got %d", cfg.Server.Port)
	}
	if cfg.Lifecycle.CrashThreshold != 3 {
		t.Fatalf("expected default crash threshold to remain, got %d", cfg.Lifecycle.CrashThreshold)
	}
	if cfg.Server.APIToken != "token-from-env" {
		t.Fatalf("expected api token override, got %q", cfg.Server.APIToken)
	}
}

func TestResolveCarrierConfigV2PathUsesHomeDirectory(t *testing.T) {
	t.Setenv("CARRIER_CONFIG", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	userHomeDirFn = os.UserHomeDir
	t.Cleanup(func() { userHomeDirFn = os.UserHomeDir })

	got, err := resolveCarrierConfigV2Path()
	if err != nil {
		t.Fatalf("resolveCarrierConfigV2Path error: %v", err)
	}

	want := filepath.Join(home, ".carrier", "config.v2.json")
	if got != want {
		t.Fatalf("path=%q want=%q", got, want)
	}
}

func TestResolveCarrierConfigV2PathHomeError(t *testing.T) {
	t.Setenv("CARRIER_CONFIG", "")
	userHomeDirFn = func() (string, error) {
		return "", errors.New("home unavailable")
	}
	t.Cleanup(func() { userHomeDirFn = os.UserHomeDir })

	_, err := resolveCarrierConfigV2Path()
	if err == nil || !strings.Contains(err.Error(), "home unavailable") {
		t.Fatalf("expected user home error, got %v", err)
	}
}

func TestLoadCarrierDefaultModelResolvePathError(t *testing.T) {
	t.Setenv("CARRIER_CONFIG", "")
	userHomeDirFn = func() (string, error) {
		return "", errors.New("cannot resolve home")
	}
	t.Cleanup(func() { userHomeDirFn = os.UserHomeDir })

	_, err := LoadCarrierDefaultModel()
	if err == nil || !strings.Contains(err.Error(), "cannot resolve home") {
		t.Fatalf("expected resolve error, got %v", err)
	}
}

func TestLoadCarrierDefaultModelFromPathErrors(t *testing.T) {
	dir := t.TempDir()
	missingPath := filepath.Join(dir, "missing-config.v2.json")
	_, err := loadCarrierDefaultModelFromPath(missingPath)
	if err == nil {
		t.Fatal("expected error for missing file")
	}

	brokenPath := filepath.Join(dir, "broken-config.v2.json")
	if err := os.WriteFile(brokenPath, []byte(`{"default_model":`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = loadCarrierDefaultModelFromPath(brokenPath)
	if err == nil {
		t.Fatal("expected parse error for broken json")
	}
}
