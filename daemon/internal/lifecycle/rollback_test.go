package lifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"carrier/baseagent"
	"carrier/daemon/internal/commandexec"
	"carrier/daemon/internal/manifest"
)

func TestSnapshotAndRestore(t *testing.T) {
	tmp := t.TempDir()
	stateDir := filepath.Join(tmp, "agents")
	binDir := filepath.Join(tmp, "bin")
	rollbackDir := filepath.Join(tmp, "rollback")
	t.Setenv("CARRIER_AGENT_STATE_DIR", stateDir)
	t.Setenv("CARRIER_AGENT_BINARY_DIR", binDir)
	t.Setenv("CARRIER_ROLLBACK_DIR", rollbackDir)

	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatalf("mkdir bin dir: %v", err)
	}
	statePath := filepath.Join(stateDir, "openclaw.json")
	binPath := filepath.Join(binDir, "openclaw")
	if err := os.WriteFile(statePath, []byte("before"), 0o600); err != nil {
		t.Fatalf("write state: %v", err)
	}
	if err := os.WriteFile(binPath, []byte("binary-before"), 0o700); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	if err := snapshotAgentState("openclaw"); err != nil {
		t.Fatalf("snapshotAgentState error: %v", err)
	}
	if err := os.WriteFile(statePath, []byte("after"), 0o600); err != nil {
		t.Fatalf("mutate state: %v", err)
	}
	if err := os.WriteFile(binPath, []byte("binary-after"), 0o700); err != nil {
		t.Fatalf("mutate binary: %v", err)
	}
	if err := restoreAgentState("openclaw"); err != nil {
		t.Fatalf("restoreAgentState error: %v", err)
	}

	stateRaw, _ := os.ReadFile(statePath)
	if string(stateRaw) != "before" {
		t.Fatalf("state content = %q, want %q", string(stateRaw), "before")
	}
	binRaw, _ := os.ReadFile(binPath)
	if string(binRaw) != "binary-before" {
		t.Fatalf("binary content = %q, want %q", string(binRaw), "binary-before")
	}
}

func TestRollbackOnInstallFailure(t *testing.T) {
	tmp := t.TempDir()
	stateDir := filepath.Join(tmp, "agents")
	rollbackDir := filepath.Join(tmp, "rollback")
	t.Setenv("CARRIER_AGENT_STATE_DIR", stateDir)
	t.Setenv("CARRIER_ROLLBACK_DIR", rollbackDir)

	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	statePath := filepath.Join(stateDir, "openclaw.json")
	if err := os.WriteFile(statePath, []byte("before"), 0o600); err != nil {
		t.Fatalf("write state: %v", err)
	}

	runner := &fakeRunner{}
	runner.onRun = func(command string, call int) (runResult, bool) {
		if strings.Contains(command, "install-openclaw") {
			_ = os.WriteFile(statePath, []byte("mutated"), 0o600)
			return runResult{result: commandexec.Result{ExitCode: 1}, err: errors.New("install failed")}, true
		}
		return runResult{}, false
	}

	svc := NewService(baseagent.NoopTriager{}, WithRunner(runner), WithRuntimeChecker(&fakeChecker{}))
	if err := svc.RegisterManifest(rollbackTestManifest()); err != nil {
		t.Fatalf("register manifest: %v", err)
	}
	err := svc.Install(context.Background(), "openclaw")
	if err == nil {
		t.Fatal("expected install failure")
	}
	stateRaw, readErr := os.ReadFile(statePath)
	if readErr != nil {
		t.Fatalf("read state: %v", readErr)
	}
	if string(stateRaw) != "before" {
		t.Fatalf("state content = %q, want rollback content %q", string(stateRaw), "before")
	}
}

func rollbackTestManifest() manifest.Manifest {
	return manifest.Manifest{
		ID:      "openclaw",
		Name:    "OpenClaw",
		Version: "1.0.0",
		Runtime: manifest.RuntimeSpec{
			Type:    manifest.RuntimeTypeGoCLI,
			Install: manifest.CommandSpec{Command: "install-openclaw"},
			Start:   manifest.CommandSpec{Command: "start-openclaw"},
			Stop:    manifest.CommandSpec{Command: "stop-openclaw"},
		},
		Network: manifest.NetworkSpec{
			Ports: []manifest.PortSpec{{Name: "http", Port: 1}},
			Healthcheck: manifest.HealthcheckSpec{
				Type: "http",
				URL:  "http://127.0.0.1/healthz",
			},
		},
		Env: manifest.EnvSpec{},
		Memory: manifest.MemorySpec{
			Supports:  []manifest.MemoryType{manifest.MemoryTypePerAgent},
			MountPath: "/tmp",
		},
		Upgrade: manifest.UpgradeSpec{Channel: "stable", Strategy: manifest.UpgradeStrategyInPlaceOrReinstall},
		Health:  manifest.HealthSpec{},
	}
}
