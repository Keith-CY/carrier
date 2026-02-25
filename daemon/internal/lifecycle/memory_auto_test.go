package lifecycle

import (
	"strings"
	"testing"

	"carrier/daemon/internal/memory"
)

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

		svc.autoMountMemories("openclaw")
		lines, err := svc.Logs("openclaw", 200)
		if err != nil {
			t.Fatalf("read logs: %v", err)
		}
		joined := strings.Join(lines, "\n")
		if !strings.Contains(joined, "memory effective view prepared") {
			t.Fatalf("expected compose log, got:\n%s", joined)
		}
	})

	t.Run("fallback path without root dir", func(t *testing.T) {
		store := memory.NewStore() // no root dir => PrepareAgentMemory falls back
		svc := NewService(nil, WithMemoryStore(store))
		if err := svc.RegisterManifest(sampleManifest()); err != nil {
			t.Fatalf("register manifest: %v", err)
		}
		if _, err := store.Create("mem-2", "Mem Two", "v1", memory.TypePerAgent, "openclaw"); err != nil {
			t.Fatalf("create memory: %v", err)
		}
		svc.setMemoryAttachments("openclaw", []string{"mem-2"})

		svc.autoMountMemories("openclaw")
		lines, err := svc.Logs("openclaw", 200)
		if err != nil {
			t.Fatalf("read logs: %v", err)
		}
		joined := strings.Join(lines, "\n")
		if !strings.Contains(joined, "falling back to direct mounts") {
			t.Fatalf("expected fallback log, got:\n%s", joined)
		}
		if !strings.Contains(joined, "memory auto-mounted mem-2") {
			t.Fatalf("expected auto-mounted log, got:\n%s", joined)
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
	svc.autoMountMemories("openclaw")
	svc.autoUnmountMemories("openclaw")
}
