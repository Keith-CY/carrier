package lifecycle

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestStableStart_ProcessExitsImmediately verifies that a process which exits
// before the stable-start probe completes is detected and reported as an error.
func TestStableStart_ProcessExitsImmediately(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	runner := &fakeRunner{}
	checker := &fakeChecker{}
	clock := &fakeClock{current: time.Date(2026, 2, 14, 4, 20, 0, 0, time.UTC)}
	pm := &fakeProcessManager{isRunning: make(map[string]bool), pids: make(map[string]int)}

	svc := NewService(nil,
		WithRunner(runner),
		WithRuntimeChecker(checker),
		WithNow(clock.Now),
		WithProcessLogDir(t.TempDir()),
		WithProcessManager(pm),
	)

	m := sampleManifest()
	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("RegisterManifest: %v", err)
	}
	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("Install: %v", err)
	}

	pm.shouldStartSucceed = true
	pm.nextPID = 100

	// Immediately after pm.Start sets isRunning=true, flip it to false
	// to simulate a process that exits before the probe can confirm stability.
	go func() {
		for {
			pm.mu.Lock()
			if pm.isRunning["openclaw"] {
				pm.isRunning["openclaw"] = false
				ch := pm.waitChs["openclaw"]
				delete(pm.waitChs, "openclaw")
				pm.mu.Unlock()
				if ch != nil {
					close(ch)
				}
				return
			}
			pm.mu.Unlock()
			time.Sleep(time.Millisecond)
		}
	}()

	err := svc.Start(context.Background(), "openclaw")
	if err == nil {
		t.Fatal("expected Start to fail for process that exits immediately, got nil")
	}
	if !strings.Contains(err.Error(), "exited") {
		t.Errorf("expected error about process exit, got: %v", err)
	}
}

// TestStableStart_ProcessStaysAlive verifies that a process which remains
// alive through all probe checks is accepted as successfully started.
func TestStableStart_ProcessStaysAlive(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	runner := &fakeRunner{}
	checker := &fakeChecker{}
	clock := &fakeClock{current: time.Date(2026, 2, 14, 4, 20, 0, 0, time.UTC)}
	pm := &fakeProcessManager{isRunning: make(map[string]bool), pids: make(map[string]int)}

	svc := NewService(nil,
		WithRunner(runner),
		WithRuntimeChecker(checker),
		WithNow(clock.Now),
		WithProcessLogDir(t.TempDir()),
		WithProcessManager(pm),
	)

	m := sampleManifest()
	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("RegisterManifest: %v", err)
	}
	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("Install: %v", err)
	}

	pm.shouldStartSucceed = true
	pm.nextPID = 200

	err := svc.Start(context.Background(), "openclaw")
	if err != nil {
		t.Fatalf("expected Start to succeed for healthy process, got: %v", err)
	}

	states := svc.ListAgents()
	found := false
	for _, s := range states {
		if s.ID == "openclaw" && s.Runtime == RuntimeStateRunning {
			found = true
		}
	}
	if !found {
		t.Error("expected agent to be in Running state after successful start")
	}
}
