package lifecycle

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"carrier/baseagent"
	"carrier/daemon/internal/manifest"
)

type hotReloadProcessManager struct {
	running    map[string]bool
	startCount int
	stopCount  int
	signalSent os.Signal
}

func (pm *hotReloadProcessManager) Start(agentID string, _ string, _ []string) (int, error) {
	if pm.running == nil {
		pm.running = map[string]bool{}
	}
	pm.running[agentID] = true
	pm.startCount++
	return 1234, nil
}

func (pm *hotReloadProcessManager) Stop(agentID string) error {
	if pm.running == nil {
		pm.running = map[string]bool{}
	}
	pm.running[agentID] = false
	pm.stopCount++
	return nil
}

func (pm *hotReloadProcessManager) IsRunning(agentID string) bool {
	if pm.running == nil {
		return false
	}
	return pm.running[agentID]
}

func (pm *hotReloadProcessManager) Wait(string) error { return nil }
func (pm *hotReloadProcessManager) Cleanup()          {}
func (pm *hotReloadProcessManager) Signal(_ string, sig os.Signal) error {
	pm.signalSent = sig
	return nil
}

func TestHotReloadSendsSignal(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("CARRIER_AGENT_CONFIG_DIR", cfgDir)

	pm := &hotReloadProcessManager{running: map[string]bool{"openclaw": true}}
	svc := NewService(baseagent.NoopTriager{}, WithProcessManager(pm), WithRuntimeChecker(&fakeChecker{}))
	if err := svc.RegisterManifest(hotReloadManifest(true)); err != nil {
		t.Fatalf("register manifest: %v", err)
	}
	svc.mu.Lock()
	state := svc.states["openclaw"]
	state.Install = InstallStateInstalled
	state.Runtime = RuntimeStateRunning
	state.StartedAt = ptrTime(time.Now())
	svc.states["openclaw"] = state
	svc.mu.Unlock()

	if err := svc.HotReloadConfig("openclaw", map[string]string{"log_level": "debug"}); err != nil {
		t.Fatalf("HotReloadConfig error: %v", err)
	}
	if pm.signalSent == nil {
		t.Fatal("expected SIGHUP to be sent")
	}
	if pm.stopCount != 0 {
		t.Fatalf("stopCount = %d, want 0", pm.stopCount)
	}
}

func TestGracefulRestartWhenNotSupported(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("CARRIER_AGENT_CONFIG_DIR", cfgDir)

	pm := &hotReloadProcessManager{running: map[string]bool{"openclaw": true}}
	svc := NewService(baseagent.NoopTriager{}, WithProcessManager(pm), WithRuntimeChecker(&fakeChecker{}))
	if err := svc.RegisterManifest(hotReloadManifest(false)); err != nil {
		t.Fatalf("register manifest: %v", err)
	}
	svc.mu.Lock()
	state := svc.states["openclaw"]
	state.Install = InstallStateInstalled
	state.Runtime = RuntimeStateRunning
	state.StartedAt = ptrTime(time.Now())
	svc.states["openclaw"] = state
	svc.mu.Unlock()

	if err := svc.HotReloadConfig("openclaw", map[string]string{"temperature": "0.3"}); err != nil {
		t.Fatalf("HotReloadConfig error: %v", err)
	}
	if pm.stopCount == 0 || pm.startCount == 0 {
		t.Fatalf("expected graceful restart, stopCount=%d startCount=%d", pm.stopCount, pm.startCount)
	}
	configPath := filepath.Join(cfgDir, "openclaw.json")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected config file to be updated: %v", err)
	}
}

func hotReloadManifest(withCapability bool) manifest.Manifest {
	caps := []string{"chat"}
	if withCapability {
		caps = append(caps, "hot_reload")
	}
	return manifest.Manifest{
		ID:           "openclaw",
		Name:         "OpenClaw",
		Version:      "1.0.0",
		Capabilities: caps,
		Runtime: manifest.RuntimeSpec{
			Type:    manifest.RuntimeTypeGoCLI,
			Install: manifest.CommandSpec{Command: "install-openclaw"},
			Start:   manifest.CommandSpec{Command: "start-openclaw"},
			Stop:    manifest.CommandSpec{Command: "stop-openclaw"},
		},
		Network: manifest.NetworkSpec{
			Ports: []manifest.PortSpec{},
			Healthcheck: manifest.HealthcheckSpec{
				Type: "http",
				URL:  "http://127.0.0.1/healthz",
			},
		},
		Memory: manifest.MemorySpec{Supports: []manifest.MemoryType{manifest.MemoryTypePerAgent}, MountPath: "/tmp"},
		Upgrade: manifest.UpgradeSpec{
			Channel:  "stable",
			Strategy: manifest.UpgradeStrategyInPlaceOrReinstall,
		},
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
