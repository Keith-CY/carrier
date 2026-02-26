package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCarrierCommandRoutesReset(t *testing.T) {
	cmd, args, err := parseCarrierCommand([]string{"carrier", "reset"})
	if err != nil {
		t.Fatalf("parseCarrierCommand(reset) error: %v", err)
	}
	if cmd != "reset" {
		t.Fatalf("command = %q, want reset", cmd)
	}
	if len(args) != 0 {
		t.Fatalf("args = %v, want empty", args)
	}
}

func TestRunResetRemovesCarrierGeneratedData(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")

	customDir := filepath.Join(tmpHome, "custom")
	customConfig := filepath.Join(customDir, "carrier-config.json")
	customOnboardConfig := filepath.Join(customDir, "onboard-config.json")
	customInstances := filepath.Join(customDir, "instances.json")
	customCredentials := filepath.Join(customDir, "credentials.json")
	customRunDir := filepath.Join(customDir, "run")
	customLogDir := filepath.Join(customDir, "logs")

	t.Setenv("CARRIER_CONFIG", customConfig)
	t.Setenv("CARRIER_ONBOARD_CONFIG", customOnboardConfig)
	t.Setenv("CARRIER_INSTANCE_STORE", customInstances)
	t.Setenv("CARRIER_CREDENTIAL_STORE", customCredentials)
	t.Setenv("CARRIER_BOOTSTRAP_RUN_DIR", customRunDir)
	t.Setenv("CARRIER_BOOTSTRAP_LOG_DIR", customLogDir)

	mustWriteFile(t, filepath.Join(tmpHome, ".carrier", "config.v2.json"), []byte("{}"))
	mustWriteFile(t, filepath.Join(tmpHome, ".carrier", "instances.json"), []byte("{}"))
	mustWriteFile(t, filepath.Join(tmpHome, ".carrier", "credentials.json"), []byte("{}"))
	mustWriteFile(t, filepath.Join(tmpHome, ".carrier", "run", "daemon.pid"), []byte("123"))
	mustWriteFile(t, filepath.Join(tmpHome, ".carrier", "logs", "daemon.log"), []byte("log"))
	mustWriteFile(t, filepath.Join(tmpHome, ".carrier", "agents", "openclaw.json"), []byte("{}"))
	mustWriteFile(t, filepath.Join(tmpHome, ".picoclaw", "config.json"), []byte("{}"))
	mustWriteFile(t, filepath.Join(tmpHome, ".openclaw", "config.json"), []byte("{}"))
	mustWriteFile(t, filepath.Join(tmpHome, ".zeroclaw", "config.json"), []byte("{}"))

	mustWriteFile(t, customConfig, []byte("{}"))
	mustWriteFile(t, customOnboardConfig, []byte("{}"))
	mustWriteFile(t, customInstances, []byte("{}"))
	mustWriteFile(t, customCredentials, []byte("{}"))
	mustWriteFile(t, filepath.Join(customRunDir, "daemon.pid"), []byte("123"))
	mustWriteFile(t, filepath.Join(customLogDir, "daemon.log"), []byte("log"))

	originalRunStop := runStopFlow
	t.Cleanup(func() {
		runStopFlow = originalRunStop
	})
	stopCalled := 0
	runStopFlow = func(out io.Writer) error {
		stopCalled++
		_, _ = io.WriteString(out, "stub stop\n")
		return nil
	}

	var out bytes.Buffer
	if err := runReset(&out); err != nil {
		t.Fatalf("runReset error: %v", err)
	}
	if stopCalled != 1 {
		t.Fatalf("runStopFlow call count = %d, want 1", stopCalled)
	}
	if !strings.Contains(out.String(), "reset complete") {
		t.Fatalf("reset output should report completion, got: %q", out.String())
	}

	for _, path := range []string{
		filepath.Join(tmpHome, ".carrier"),
		filepath.Join(tmpHome, ".picoclaw"),
		filepath.Join(tmpHome, ".openclaw"),
		filepath.Join(tmpHome, ".zeroclaw"),
		customConfig,
		customOnboardConfig,
		customInstances,
		customCredentials,
		customRunDir,
		customLogDir,
	} {
		assertPathRemoved(t, path)
	}
}

func mustWriteFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertPathRemoved(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("path %s still exists or stat failed: %v", path, err)
	}
}
