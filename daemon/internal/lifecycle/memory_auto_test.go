package lifecycle

import (
	"context"
	"strings"
	"testing"

	"carrier/daemon/internal/commandexec"
	"carrier/daemon/internal/memory"
)

type noEnvProcessManager struct {
	isRunning map[string]bool
}

func (n *noEnvProcessManager) Start(agentID string, _ string, _ []string) (int, error) {
	if n.isRunning == nil {
		n.isRunning = map[string]bool{}
	}
	n.isRunning[agentID] = true
	return 100, nil
}

func (n *noEnvProcessManager) Stop(agentID string) error {
	if n.isRunning != nil {
		delete(n.isRunning, agentID)
	}
	return nil
}

func (n *noEnvProcessManager) IsRunning(agentID string) bool {
	return n.isRunning[agentID]
}

func (n *noEnvProcessManager) Wait(agentID string) error {
	return nil
}

func (n *noEnvProcessManager) Cleanup() {}

func TestAutoMountMemoriesComposeAndFallbackPaths(t *testing.T) {
	t.Run("compose path with root dir", func(t *testing.T) {
		store := memory.NewStore(memory.WithRootDir(t.TempDir()))
		svc := NewService(nil, WithMemoryStore(store))
		if err := svc.RegisterManifest(sampleManifest()); err != nil {
			t.Fatalf("register manifest: %v", err)
		}
		if _, err := store.Create("mem-1", "Mem One", "v1", memory.TypePerAgent, "openclaw"); err != nil {
			t.Fatalf("create memory: %v", err)
		}
		svc.setMemoryAttachments("openclaw", []string{"mem-1"})

		contract, err := svc.autoMountMemories("openclaw")
		if err != nil {
			t.Fatalf("auto mount: %v", err)
		}
		if contract.Env["AGENTD_MEMORY_PATH"] == "" {
			t.Fatalf("expected memory env contract")
		}
		lines, err := svc.Logs("openclaw", 200)
		if err != nil {
			t.Fatalf("read logs: %v", err)
		}
		joined := strings.Join(lines, "\n")
		if !strings.Contains(joined, "memory effective view prepared") {
			t.Fatalf("expected compose log, got:\n%s", joined)
		}
	})

	t.Run("error path without root dir", func(t *testing.T) {
		store := memory.NewStore() // no root dir => PrepareAgentMemory fails
		svc := NewService(nil, WithMemoryStore(store))
		if err := svc.RegisterManifest(sampleManifest()); err != nil {
			t.Fatalf("register manifest: %v", err)
		}
		if _, err := store.Create("mem-2", "Mem Two", "v1", memory.TypePerAgent, "openclaw"); err != nil {
			t.Fatalf("create memory: %v", err)
		}
		svc.setMemoryAttachments("openclaw", []string{"mem-2"})

		if _, err := svc.autoMountMemories("openclaw"); err == nil {
			t.Fatalf("expected auto mount error without root dir")
		}
	})
}

func TestAutoUnmountMemories(t *testing.T) {
	store := memory.NewStore()
	svc := NewService(nil, WithMemoryStore(store))
	if err := svc.RegisterManifest(sampleManifest()); err != nil {
		t.Fatalf("register manifest: %v", err)
	}
	if _, err := store.Create("mem-3", "Mem Three", "v1", memory.TypePerAgent, "openclaw"); err != nil {
		t.Fatalf("create memory: %v", err)
	}
	if _, err := store.Mount("mem-3", "openclaw", memory.AccessReadOnly); err != nil {
		t.Fatalf("mount memory: %v", err)
	}

	svc.autoUnmountMemories("openclaw")
	lines, err := svc.Logs("openclaw", 200)
	if err != nil {
		t.Fatalf("read logs: %v", err)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "memory auto-unmounted") {
		t.Fatalf("expected auto-unmounted log, got:\n%s", joined)
	}
}

func TestAutoMountUnmountNoMemoryStore(t *testing.T) {
	svc := NewService(nil)
	// should not panic
	if _, err := svc.autoMountMemories("openclaw"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	svc.autoUnmountMemories("openclaw")
}

func TestAutoMountAppliesManifestMemoryPermissions(t *testing.T) {
	store := memory.NewStore(memory.WithRootDir(t.TempDir()))
	svc := NewService(nil, WithMemoryStore(store))
	m := sampleManifest()
	m.Memory.Permissions.ReadScopes = []string{"shared:profile"}
	m.Memory.Permissions.WriteScopes = []string{"shared:profile"}
	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("register manifest: %v", err)
	}

	if _, err := svc.autoMountMemories("openclaw"); err != nil {
		t.Fatalf("auto mount: %v", err)
	}
	scopes := store.InstanceScopes("openclaw")
	if len(scopes) == 0 || scopes[0] != memory.Scope("shared:profile") {
		t.Fatalf("expected shared:profile scope attachment, got=%v", scopes)
	}
	grants := store.ListGrants("openclaw")
	if len(grants) == 0 {
		t.Fatalf("expected write grant from manifest permissions")
	}
}

func TestStartInjectsPreparedMemoryEnvBeforeProcessStart(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	store := memory.NewStore(memory.WithRootDir(t.TempDir()))
	pm := &fakeProcessManager{
		isRunning:          make(map[string]bool),
		pids:               make(map[string]int),
		shouldStartSucceed: true,
		nextPID:            42,
	}
	runner := &fakeRunner{
		results: map[string]runResult{
			"install-openclaw": {result: commandexec.Result{ExitCode: 0}},
		},
	}
	svc := NewService(nil, WithMemoryStore(store), WithProcessManager(pm), WithRunner(runner))
	if err := svc.RegisterManifest(sampleManifest()); err != nil {
		t.Fatalf("register manifest: %v", err)
	}
	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}
	if _, err := store.Create("mem-env", "Env Mem", "v1", memory.TypePerAgent, "openclaw"); err != nil {
		t.Fatalf("create memory: %v", err)
	}
	svc.setMemoryAttachments("openclaw", []string{"mem-env"})

	if err := svc.Start(context.Background(), "openclaw"); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		_ = svc.Stop(context.Background(), "openclaw")
	})

	pm.mu.Lock()
	defer pm.mu.Unlock()
	if pm.lastEnv["AGENTD_MEMORY_PATH"] == "" {
		t.Fatalf("expected AGENTD_MEMORY_PATH to be injected")
	}
	if pm.lastEnv["AGENTD_MEMORY_WRITE_PATH"] == "" {
		t.Fatalf("expected AGENTD_MEMORY_WRITE_PATH to be injected")
	}
}

func TestStartFailsWhenMemoryEnvRequiredButProcessManagerLacksStartWithEnv(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	store := memory.NewStore(memory.WithRootDir(t.TempDir()))
	runner := &fakeRunner{
		results: map[string]runResult{
			"install-openclaw": {result: commandexec.Result{ExitCode: 0}},
		},
	}
	svc := NewService(nil, WithMemoryStore(store), WithProcessManager(&noEnvProcessManager{}), WithRunner(runner))
	if err := svc.RegisterManifest(sampleManifest()); err != nil {
		t.Fatalf("register manifest: %v", err)
	}
	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}
	if _, err := store.Create("mem-env-required", "Env Mem", "v1", memory.TypePerAgent, "openclaw"); err != nil {
		t.Fatalf("create memory: %v", err)
	}
	svc.setMemoryAttachments("openclaw", []string{"mem-env-required"})

	err := svc.Start(context.Background(), "openclaw")
	if err == nil {
		t.Fatalf("expected start failure when StartWithEnv is unavailable")
	}
	if !strings.Contains(err.Error(), "StartWithEnv") {
		t.Fatalf("expected StartWithEnv error, got: %v", err)
	}
}
