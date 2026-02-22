package lifecycle

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestStatePersistence_Stop verifies that state is persisted after a successful stop.
func TestStatePersistence_Stop(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	runner := &fakeRunner{}
	checker := &fakeChecker{}
	clock := &fakeClock{current: time.Date(2026, 2, 14, 4, 20, 0, 0, time.UTC)}

	svc := NewService(nil,
		WithRunner(runner),
		WithRuntimeChecker(checker),
		WithDiagnoseDir(t.TempDir()),
		WithNow(clock.Now),
		WithStateFile(statePath),
	)

	if err := svc.RegisterManifest(sampleManifest()); err != nil {
		t.Fatalf("register manifest: %v", err)
	}

	// Install and start
	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := svc.Start(context.Background(), "openclaw"); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Stop should persist state
	clock.current = clock.current.Add(1 * time.Minute)
	if err := svc.Stop(context.Background(), "openclaw"); err != nil {
		t.Fatalf("stop: %v", err)
	}

	// Verify state file was written
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("state file not found: %v", err)
	}

	// Load and verify state
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}

	var states map[string]PersistedAgentState
	if err := json.Unmarshal(data, &states); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}

	state, ok := states["openclaw"]
	if !ok {
		t.Fatal("openclaw state not found in persisted file")
	}

	if state.ID != "openclaw" {
		t.Errorf("expected ID 'openclaw', got %q", state.ID)
	}
	if !state.Installed {
		t.Error("expected Installed=true")
	}
	if state.RuntimeState != string(RuntimeStateStopped) {
		t.Errorf("expected RuntimeState=%q, got %q", RuntimeStateStopped, state.RuntimeState)
	}
	if !state.LastTransition.Equal(clock.current) {
		t.Errorf("expected LastTransition=%v, got %v", clock.current, state.LastTransition)
	}
}

// TestStatePersistence_Crash verifies that state is persisted after a crash.
func TestStatePersistence_Crash(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	runner := &fakeRunner{}
	checker := &fakeChecker{}
	clock := &fakeClock{current: time.Date(2026, 2, 14, 4, 20, 0, 0, time.UTC)}

	svc := NewService(nil,
		WithRunner(runner),
		WithRuntimeChecker(checker),
		WithDiagnoseDir(t.TempDir()),
		WithNow(clock.Now),
		WithStateFile(statePath),
	)

	// Create a manifest with a long-running command; we'll kill it to simulate a crash.
	m := sampleManifest()
	m.Runtime.Start.Command = "sleep 60"
	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("register manifest: %v", err)
	}

	// Install
	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}

	// Start the agent process
	if err := svc.Start(context.Background(), "openclaw"); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Advance clock before killing so there's no data race with monitorProcess.
	clock.Advance(30 * time.Second)

	// Kill the process to simulate a crash.
	_ = svc.processManager.Stop("openclaw")

	// Wait for monitorProcess to detect the exit and persist state.
	time.Sleep(200 * time.Millisecond)

	// Verify state file was written
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("state file not found after crash: %v", err)
	}

	// Load and verify state
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}

	var states map[string]PersistedAgentState
	if err := json.Unmarshal(data, &states); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}

	state, ok := states["openclaw"]
	if !ok {
		t.Fatal("openclaw state not found in persisted file after crash")
	}

	if state.ID != "openclaw" {
		t.Errorf("expected ID 'openclaw', got %q", state.ID)
	}
	if !state.Installed {
		t.Error("expected Installed=true")
	}
	// After crash, the state should be RuntimeStateCrashing
	if state.RuntimeState != string(RuntimeStateCrashing) {
		t.Errorf("expected RuntimeState=%q, got %q", RuntimeStateCrashing, state.RuntimeState)
	}
}

// TestStatePersistence_CrashLoopCooldownSurvivesRestart verifies that crash-loop
// cooldown state is persisted and restored after a simulated daemon restart.
func TestStatePersistence_CrashLoopCooldownSurvivesRestart(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	checker := &fakeChecker{}
	clock := &fakeClock{current: time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)}

	svc := NewService(nil,
		WithRunner(&fakeRunner{}),
		WithRuntimeChecker(checker),
		WithDiagnoseDir(t.TempDir()),
		WithNow(clock.Now),
		WithStateFile(statePath),
		WithCrashLoopConfig(3, 5*time.Minute, 5*time.Minute),
	)

	m := sampleManifest()
	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("register manifest: %v", err)
	}
	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}

	// Trigger crash-loop: 3 failures within window
	for i := 0; i < 3; i++ {
		clock.Advance(10 * time.Second)
		svc.updateStateOnStartError("openclaw", errMockFailure)
	}
	// Persist state including crash-loop cooldown
	svc.saveState()

	// Verify cooldown is active in original service
	svc.mu.RLock()
	_, hasCooldown := svc.cooldowns["openclaw"]
	svc.mu.RUnlock()
	if !hasCooldown {
		t.Fatal("expected cooldown to be active after crash-loop")
	}

	// Simulate daemon restart: create a new Service with the same state file.
	svc2 := NewService(nil,
		WithRunner(&fakeRunner{}),
		WithRuntimeChecker(checker),
		WithDiagnoseDir(t.TempDir()),
		WithNow(clock.Now),
		WithStateFile(statePath),
		WithCrashLoopConfig(3, 5*time.Minute, 5*time.Minute),
	)
	if err := svc2.RegisterManifest(m); err != nil {
		t.Fatalf("register manifest on restart: %v", err)
	}

	// Cooldown should still be enforced after daemon restart
	_, stateCheck, err := svc2.getManifestAndState("openclaw")
	if err != nil {
		t.Fatalf("getManifestAndState: %v", err)
	}
	if err := svc2.blockIfCrashLoopCoolingDown("openclaw", stateCheck); err == nil {
		t.Error("expected crash-loop cooldown to be enforced after daemon restart, but start was allowed")
	}

	// Verify restart timestamps were also restored
	svc2.mu.RLock()
	restartCount := len(svc2.restarts["openclaw"])
	svc2.mu.RUnlock()
	if restartCount != 3 {
		t.Errorf("expected 3 restart timestamps after restore, got %d", restartCount)
	}

	// Advance past cooldown — should be allowed
	clock.Advance(6 * time.Minute)
	_, stateCheck, _ = svc2.getManifestAndState("openclaw")
	if err := svc2.blockIfCrashLoopCoolingDown("openclaw", stateCheck); err != nil {
		t.Errorf("expected cooldown to expire after waiting, got: %v", err)
	}
}

// TestStatePersistence_MultipleAgents verifies state is persisted for multiple agents.
func TestStatePersistence_MultipleAgents(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	runner := &fakeRunner{}
	checker := &fakeChecker{}
	clock := &fakeClock{current: time.Date(2026, 2, 14, 4, 20, 0, 0, time.UTC)}

	svc := NewService(nil,
		WithRunner(runner),
		WithRuntimeChecker(checker),
		WithDiagnoseDir(t.TempDir()),
		WithNow(clock.Now),
		WithStateFile(statePath),
	)

	// Register two manifests
	if err := svc.RegisterManifest(sampleManifest()); err != nil {
		t.Fatalf("register openclaw: %v", err)
	}

	m2 := sampleManifest()
	m2.ID = "agent2"
	if err := svc.RegisterManifest(m2); err != nil {
		t.Fatalf("register agent2: %v", err)
	}

	// Install both
	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("install openclaw: %v", err)
	}
	if err := svc.Install(context.Background(), "agent2"); err != nil {
		t.Fatalf("install agent2: %v", err)
	}

	// Start openclaw
	if err := svc.Start(context.Background(), "openclaw"); err != nil {
		t.Fatalf("start openclaw: %v", err)
	}

	clock.current = clock.current.Add(1 * time.Minute)

	// Stop openclaw
	if err := svc.Stop(context.Background(), "openclaw"); err != nil {
		t.Fatalf("stop openclaw: %v", err)
	}

	// Load and verify both agents are persisted
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}

	var states map[string]PersistedAgentState
	if err := json.Unmarshal(data, &states); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}

	if len(states) != 2 {
		t.Fatalf("expected 2 agents in state file, got %d", len(states))
	}

	if _, ok := states["openclaw"]; !ok {
		t.Error("openclaw not found in state")
	}
	if _, ok := states["agent2"]; !ok {
		t.Error("agent2 not found in state")
	}

	// Verify openclaw is stopped, agent2 is installed but not started
	if states["openclaw"].RuntimeState != string(RuntimeStateStopped) {
		t.Errorf("openclaw should be stopped, got %q", states["openclaw"].RuntimeState)
	}
	if states["agent2"].RuntimeState != string(RuntimeStateStopped) {
		t.Errorf("agent2 should be stopped (never started), got %q", states["agent2"].RuntimeState)
	}
}
