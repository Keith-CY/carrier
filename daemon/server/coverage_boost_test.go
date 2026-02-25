package server

import (
	"context"
	"testing"
	"time"

	"carrier/baseagent"
	"carrier/daemon/internal/catalog"
	"carrier/daemon/internal/lifecycle"
	"carrier/daemon/internal/manifest"
	"carrier/daemon/internal/memory"
)

func newServerCoverageService(t *testing.T) *lifecycle.Service {
	t.Helper()
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatalf("register openclaw manifest: %v", err)
	}
	return svc
}

func TestLifecycleAgentServiceAdapterCoverage(t *testing.T) {
	svc := newServerCoverageService(t)
	adapter := newLifecycleAgentServiceAdapter(svc)

	states := adapter.ListAgents()
	if len(states) == 0 {
		t.Fatal("expected at least one listed agent")
	}

	ctx := context.Background()

	if _, err := adapter.Status("openclaw"); err != nil {
		t.Fatalf("status(openclaw) returned error: %v", err)
	}
	if _, err := adapter.Status("missing-agent"); err == nil {
		t.Fatal("expected status error for missing agent")
	}

	if _, err := adapter.Logs("missing-agent", 10); err == nil {
		t.Fatal("expected logs error for missing agent")
	}
	if _, err := adapter.Upgrade(ctx, "missing-agent"); err == nil {
		t.Fatal("expected upgrade error for missing agent")
	}
	if _, err := adapter.Diagnose("missing-agent"); err == nil {
		t.Fatal("expected diagnose error for missing agent")
	}
	if err := adapter.Install(ctx, "missing-agent"); err == nil {
		t.Fatal("expected install error for missing agent")
	}
	if err := adapter.Uninstall(ctx, "missing-agent"); err == nil {
		t.Fatal("expected uninstall error for missing agent")
	}
	if err := adapter.Start(ctx, "missing-agent"); err == nil {
		t.Fatal("expected start error for missing agent")
	}
	if err := adapter.Stop(ctx, "missing-agent"); err == nil {
		t.Fatal("expected stop error for missing agent")
	}
}

func TestBaseAgentMemoryStoreAdapterCoverage(t *testing.T) {
	if got := newBaseAgentMemoryStoreAdapter(nil); got != nil {
		t.Fatal("expected nil adapter for nil store")
	}

	store := memory.NewStore(memory.WithRootDir(t.TempDir()))
	adapter := newBaseAgentMemoryStoreAdapter(store)
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}

	if err := adapter.Get("missing"); err == nil {
		t.Fatal("expected missing memory get error")
	}

	if err := adapter.Create("mem-a", "Memory A", "v1", baseagent.MemoryTypePerAgent, "openclaw"); err != nil {
		t.Fatalf("create memory failed: %v", err)
	}
	if err := adapter.Get("mem-a"); err != nil {
		t.Fatalf("get created memory failed: %v", err)
	}

	if got := adapter.List(); len(got) == 0 {
		t.Fatal("expected list to include created memory")
	}

	_ = adapter.SetAttachmentsFromLinks("openclaw", []string{"mem-a"})
	_ = adapter.PrepareAgentMemory("openclaw")
	_, _ = adapter.ExportMemory("mem-a", baseagent.ExportOptions{Actor: "test", RequestID: "req-1"})
	_ = adapter.Archive("mem-a")
}

func TestToBaseAgentStateCoverage(t *testing.T) {
	got := toBaseAgentState(lifecycle.AgentState{
		ID:           "openclaw",
		Install:      lifecycle.InstallStateInstalled,
		Runtime:      lifecycle.RuntimeStateRunning,
		Health:       lifecycle.HealthStateHealthy,
		RestartCount: 3,
	})

	if got.ID != "openclaw" || got.Install != "installed" || got.Runtime != "running" || got.Health != "healthy" || got.RestartCount != 3 {
		t.Fatalf("unexpected mapped state: %+v", got)
	}
}

func TestLifecycleAgentServiceAdapterUpgradeSuccess(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	testManifest := manifest.Manifest{
		ID:      "upgrade-agent",
		Name:    "Upgrade Agent",
		Version: "1.0.0",
		Runtime: manifest.RuntimeSpec{
			Type:    manifest.RuntimeTypeLocalBinary,
			Install: manifest.CommandSpec{Command: "echo installed"},
			Upgrade: manifest.CommandSpec{Command: "echo upgraded"},
			Start:   manifest.CommandSpec{Command: "echo started"},
			Stop:    manifest.CommandSpec{Command: "echo stopped"},
		},
		Memory: manifest.MemorySpec{
			Supports:  []manifest.MemoryType{manifest.MemoryTypePerAgent},
			MountPath: t.TempDir(),
		},
	}
	if err := svc.RegisterManifest(testManifest); err != nil {
		t.Fatalf("register manifest: %v", err)
	}
	if err := svc.Install(context.Background(), "upgrade-agent"); err != nil {
		t.Fatalf("install for upgrade test failed: %v", err)
	}

	adapter := newLifecycleAgentServiceAdapter(svc)
	res, err := adapter.Upgrade(context.Background(), "upgrade-agent")
	if err != nil {
		t.Fatalf("upgrade failed: %v", err)
	}
	if res.AgentID != "upgrade-agent" {
		t.Fatalf("unexpected upgrade result: %+v", res)
	}
}

func TestShutdownHelpersCoverage(t *testing.T) {
	svc := newServerCoverageService(t)

	if err := stopAllAgents(context.Background(), svc); err != nil {
		t.Fatalf("stopAllAgents returned error: %v", err)
	}
	if err := shutdownAgents(svc, 50*time.Millisecond); err != nil {
		t.Fatalf("shutdownAgents returned error: %v", err)
	}
}
