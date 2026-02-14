package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultValues(t *testing.T) {
	cfg := Default()
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected host 127.0.0.1, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("expected log level info, got %s", cfg.Log.Level)
	}
	if cfg.Lifecycle.CrashThreshold != 3 {
		t.Errorf("expected crash threshold 3, got %d", cfg.Lifecycle.CrashThreshold)
	}
}

func TestLoadMissingFile(t *testing.T) {
	cfg, err := Load("/tmp/nonexistent-carrier-config.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("expected default port, got %d", cfg.Server.Port)
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := `{"server":{"host":"0.0.0.0","port":8080},"log":{"level":"debug"}}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("expected log level debug, got %s", cfg.Log.Level)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{bad`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestEnvOverrides(t *testing.T) {
	t.Setenv("CARRIER_SERVER_HOST", "10.0.0.1")
	t.Setenv("CARRIER_SERVER_PORT", "3000")
	t.Setenv("CARRIER_LOG_LEVEL", "error")
	t.Setenv("CARRIER_LOG_FORMAT", "json")
	t.Setenv("CARRIER_CRASH_THRESHOLD", "5")
	t.Setenv("CARRIER_CRASH_WINDOW", "10m")
	t.Setenv("CARRIER_CRASH_COOLDOWN", "15m")

	cfg, err := Load("/tmp/nonexistent-carrier-config.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Host != "10.0.0.1" {
		t.Errorf("expected host 10.0.0.1, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 3000 {
		t.Errorf("expected port 3000, got %d", cfg.Server.Port)
	}
	if cfg.Log.Level != "error" {
		t.Errorf("expected log level error, got %s", cfg.Log.Level)
	}
	if cfg.Log.Format != "json" {
		t.Errorf("expected log format json, got %s", cfg.Log.Format)
	}
	if cfg.Lifecycle.CrashThreshold != 5 {
		t.Errorf("expected crash threshold 5, got %d", cfg.Lifecycle.CrashThreshold)
	}
	if cfg.Lifecycle.CrashWindow != "10m" {
		t.Errorf("expected crash window 10m, got %s", cfg.Lifecycle.CrashWindow)
	}
}

func TestCrashDurations(t *testing.T) {
	cfg := Default()
	w, err := cfg.CrashWindowDuration()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.Minutes() != 5 {
		t.Errorf("expected 5m, got %v", w)
	}
	c, err := cfg.CrashCooldownDuration()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Minutes() != 5 {
		t.Errorf("expected 5m, got %v", c)
	}
}
