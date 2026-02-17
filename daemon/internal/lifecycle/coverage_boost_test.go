package lifecycle

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"carrier/daemon/internal/runtimecheck"
)

// --- Uninstall flow ---

func TestUninstallNotInstalled(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	svc := newServiceForTest(t, &fakeRunner{}, &fakeChecker{})

	// Uninstall an agent that's registered but not installed succeeds (idempotent reset)
	err := svc.Uninstall(context.Background(), "openclaw")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestUninstallAfterInstall(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	svc := newServiceForTest(t, &fakeRunner{}, &fakeChecker{})

	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Uninstall(context.Background(), "openclaw"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	state, err := svc.Status("openclaw")
	if err != nil {
		t.Fatal(err)
	}
	if state.Install != InstallStateNotInstalled {
		t.Fatalf("expected not_installed, got %s", state.Install)
	}
}

func TestUninstallWhileRunning(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	svc := newServiceForTest(t, &fakeRunner{}, &fakeChecker{})

	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background(), "openclaw"); err != nil {
		t.Fatal(err)
	}

	// Uninstall should stop then uninstall
	if err := svc.Uninstall(context.Background(), "openclaw"); err != nil {
		t.Fatalf("uninstall running agent: %v", err)
	}

	state, _ := svc.Status("openclaw")
	if state.Install != InstallStateNotInstalled {
		t.Fatalf("expected not_installed, got %s", state.Install)
	}
	if state.Runtime != RuntimeStateStopped {
		t.Fatalf("expected stopped, got %s", state.Runtime)
	}
}

func TestUninstallNotFound(t *testing.T) {
	svc := newServiceForTest(t, &fakeRunner{}, &fakeChecker{})
	err := svc.Uninstall(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- RegisterInstance ---

func TestRegisterInstance(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	svc := newServiceForTest(t, &fakeRunner{}, &fakeChecker{})

	instID, err := svc.RegisterInstance("openclaw", "myinst")
	if err != nil {
		t.Fatalf("RegisterInstance: %v", err)
	}
	if instID != "myinst" {
		t.Fatalf("expected myinst, got %s", instID)
	}

	// Should be visible in status
	state, err := svc.Status("myinst")
	if err != nil {
		t.Fatal(err)
	}
	if state.Install != InstallStateNotInstalled {
		t.Fatalf("instance should start as not installed")
	}
}

func TestRegisterInstanceAutoName(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	svc := newServiceForTest(t, &fakeRunner{}, &fakeChecker{})

	instID, err := svc.RegisterInstance("openclaw", "")
	if err != nil {
		t.Fatalf("RegisterInstance auto-name: %v", err)
	}
	if instID == "" || instID == "openclaw" {
		t.Fatalf("expected auto-generated name, got %q", instID)
	}
}

func TestRegisterInstanceDuplicate(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	svc := newServiceForTest(t, &fakeRunner{}, &fakeChecker{})

	_, err := svc.RegisterInstance("openclaw", "dup1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.RegisterInstance("openclaw", "dup1")
	if err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestRegisterInstanceInvalidName(t *testing.T) {
	svc := newServiceForTest(t, &fakeRunner{}, &fakeChecker{})

	_, err := svc.RegisterInstance("openclaw", "../bad")
	if err == nil {
		t.Fatal("expected error for invalid name")
	}
}

func TestRegisterInstanceNotFound(t *testing.T) {
	svc := newServiceForTest(t, &fakeRunner{}, &fakeChecker{})

	_, err := svc.RegisterInstance("nonexistent", "inst1")
	if err == nil {
		t.Fatal("expected error for nonexistent base agent")
	}
}

func TestUnregisterInstance(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	svc := newServiceForTest(t, &fakeRunner{}, &fakeChecker{})

	_, err := svc.RegisterInstance("openclaw", "removeme")
	if err != nil {
		t.Fatal(err)
	}

	err = svc.UnregisterInstance("removeme")
	if err != nil {
		t.Fatalf("UnregisterInstance: %v", err)
	}

	_, err = svc.Status("removeme")
	if err == nil {
		t.Fatal("expected error after unregister")
	}
}

func TestUnregisterInstanceNotFound(t *testing.T) {
	svc := newServiceForTest(t, &fakeRunner{}, &fakeChecker{})
	err := svc.UnregisterInstance("nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- Crash-loop cooldown persistence round-trip ---

func TestCrashLoopCooldownPersistenceRoundTrip(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")
	clock := &fakeClock{current: time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)}

	svc := NewService(nil,
		WithRunner(&fakeRunner{}),
		WithRuntimeChecker(&fakeChecker{}),
		WithDiagnoseDir(t.TempDir()),
		WithNow(clock.Now),
		WithStateFile(statePath),
		WithCrashLoopConfig(2, 5*time.Minute, 3*time.Minute),
	)
	if err := svc.RegisterManifest(sampleManifest()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatal(err)
	}

	// Trigger crash loop
	for i := 0; i < 2; i++ {
		clock.Advance(10 * time.Second)
		svc.updateStateOnStartError("openclaw", errMockFailure)
	}
	svc.saveState()

	// Verify file exists and has cooldown
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var states map[string]PersistedAgentState
	json.Unmarshal(data, &states)
	if !states["openclaw"].CooldownUntil.After(clock.Now()) {
		t.Fatal("expected cooldown in persisted state")
	}

	// Restore into new service
	svc2 := NewService(nil,
		WithRunner(&fakeRunner{}),
		WithRuntimeChecker(&fakeChecker{}),
		WithDiagnoseDir(t.TempDir()),
		WithNow(clock.Now),
		WithStateFile(statePath),
		WithCrashLoopConfig(2, 5*time.Minute, 3*time.Minute),
	)
	if err := svc2.RegisterManifest(sampleManifest()); err != nil {
		t.Fatal(err)
	}

	// Should still be cooling down
	_, state, _ := svc2.getManifestAndState("openclaw")
	if err := svc2.blockIfCrashLoopCoolingDown("openclaw", state); err == nil {
		t.Fatal("expected cooldown to be enforced after restore")
	}

	// Advance past cooldown
	clock.Advance(4 * time.Minute)
	_, state, _ = svc2.getManifestAndState("openclaw")
	if err := svc2.blockIfCrashLoopCoolingDown("openclaw", state); err != nil {
		t.Fatalf("expected cooldown expired, got %v", err)
	}
}

// --- MergedLogs ---

func TestMergedLogs(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	svc := newServiceForTest(t, &fakeRunner{}, &fakeChecker{})

	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatal(err)
	}

	lines := svc.MergedLogs(10)
	// Should return at least the install log line
	if len(lines) == 0 {
		t.Fatal("expected some merged log lines")
	}
}

// --- RunningAgentsCount ---

func TestRunningAgentsCount(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	svc := newServiceForTest(t, &fakeRunner{}, &fakeChecker{})

	if svc.RunningAgentsCount() != 0 {
		t.Fatal("expected 0 running agents")
	}

	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background(), "openclaw"); err != nil {
		t.Fatal(err)
	}
	if svc.RunningAgentsCount() != 1 {
		t.Fatal("expected 1 running agent")
	}
}

// --- AgentName ---

func TestAgentName(t *testing.T) {
	svc := newServiceForTest(t, &fakeRunner{}, &fakeChecker{})
	name := svc.AgentName("openclaw")
	if name != "OpenClaw" {
		t.Fatalf("expected OpenClaw, got %s", name)
	}
	name = svc.AgentName("nonexistent")
	if name != "nonexistent" {
		t.Fatalf("expected fallback to id, got %s", name)
	}
}

// --- boundTail ---

func TestBoundTail(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{0, 0},
		{-5, 0},
		{50, 50},
		{20000, 1000},
	}
	for _, tt := range tests {
		got := boundTail(tt.input)
		if got != tt.want {
			t.Errorf("boundTail(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

// --- Cleanup ---

func TestCleanup(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	svc := newServiceForTest(t, &fakeRunner{}, &fakeChecker{})

	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background(), "openclaw"); err != nil {
		t.Fatal(err)
	}

	// Should not panic
	svc.Cleanup()
}

// --- Uninstall resets crash-loop state ---

func TestUninstallResetsCrashLoopState(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	clock := &fakeClock{current: time.Date(2026, 2, 14, 4, 20, 0, 0, time.UTC)}
	svc := NewService(nil,
		WithRunner(&fakeRunner{}),
		WithRuntimeChecker(&fakeChecker{}),
		WithDiagnoseDir(t.TempDir()),
		WithNow(clock.Now),
		WithCrashLoopConfig(2, 5*time.Minute, 5*time.Minute),
	)
	if err := svc.RegisterManifest(sampleManifest()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatal(err)
	}

	// Trigger crash loop
	for i := 0; i < 2; i++ {
		clock.Advance(10 * time.Second)
		svc.updateStateOnStartError("openclaw", errMockFailure)
	}

	// Uninstall should reset crash loop
	if err := svc.Uninstall(context.Background(), "openclaw"); err != nil {
		t.Fatal(err)
	}

	// Reinstall and verify no cooldown
	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatal(err)
	}
	_, state, _ := svc.getManifestAndState("openclaw")
	if err := svc.blockIfCrashLoopCoolingDown("openclaw", state); err != nil {
		t.Fatalf("expected no cooldown after uninstall+reinstall, got %v", err)
	}
}

// --- Logs with bad agent ID ---

func TestLogsNotFound(t *testing.T) {
	svc := newServiceForTest(t, &fakeRunner{}, &fakeChecker{})
	_, err := svc.Logs("nonexistent", 100)
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- SetMemoryAttachments / GetMemoryAttachments ---

func TestMemoryAttachments(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	svc := newServiceForTest(t, &fakeRunner{}, &fakeChecker{})

	svc.SetMemoryAttachments("openclaw", []string{"file1.txt", "file2.txt"})
	got := svc.GetMemoryAttachments("openclaw")
	if len(got) != 2 || got[0] != "file1.txt" {
		t.Fatalf("expected [file1.txt file2.txt], got %v", got)
	}

	// Nonexistent agent
	got = svc.GetMemoryAttachments("nonexistent")
	if got != nil {
		t.Fatalf("expected nil for nonexistent, got %v", got)
	}
}

// --- MemoryStore ---

func TestMemoryStore(t *testing.T) {
	svc := newServiceForTest(t, &fakeRunner{}, &fakeChecker{})
	// MemoryStore may return nil if not configured; just ensure no panic.
	_ = svc.MemoryStore()
}

// --- formatPreFlightFailures ---

func TestFormatPreFlightFailures(t *testing.T) {
	result := formatPreFlightFailures(runtimecheck.PreFlightResult{
		Passed: false,
		Checks: []runtimecheck.CheckResult{
			{Name: "runtime", Passed: false, Message: "missing binary", Repair: "install it"},
			{Name: "env", Passed: false, Message: "OPENAI_API_KEY not set"},
			{Name: "port", Passed: true, Message: "ok"},
		},
	})
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	if len(result) < 10 {
		t.Fatalf("expected meaningful message, got %q", result)
	}
}
