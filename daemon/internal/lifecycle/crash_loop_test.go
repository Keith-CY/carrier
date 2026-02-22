package lifecycle

import (
	"context"
	"testing"
	"time"

	"carrier/daemon/internal/baseagent"
	"carrier/daemon/internal/manifest"
)

// testManifest creates a minimal valid manifest for testing.
func testManifest(id string) manifest.Manifest {
	return manifest.Manifest{
		ID:      id,
		Name:    "Test Agent",
		Version: "1.0.0",
		Runtime: manifest.RuntimeSpec{
			Type:    manifest.RuntimeTypeLocalBinary,
			Install: manifest.CommandSpec{Command: "echo install"},
			Start:   manifest.CommandSpec{Command: "echo 'start'"},
			Stop:    manifest.CommandSpec{Command: "echo 'stop'"},
		},
		Memory: manifest.MemorySpec{
			MountPath: "/tmp/test-memory",
			Supports:  []manifest.MemoryType{manifest.MemoryTypePerAgent},
		},
	}
}

// TestCrashLoopDetector_NormalCrash verifies that a single crash doesn't trigger crash-loop detection.
func TestCrashLoopDetector_NormalCrash(t *testing.T) {
	now := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	mockNow := func() time.Time { return now }

	svc := NewService(
		baseagent.NoopTriager{},
		WithNow(mockNow),
		WithCrashLoopConfig(3, 5*time.Minute, 5*time.Minute),
	)

	m := testManifest("test-agent")
	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("RegisterManifest failed: %v", err)
	}

	// Install the agent
	if err := svc.Install(context.Background(), "test-agent"); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Simulate a single crash
	svc.updateStateOnStartError("test-agent", errMockFailure)

	state, err := svc.Status("test-agent")
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}

	// Should be in crashing state, not crash-loop
	if state.Runtime != RuntimeStateCrashing {
		t.Errorf("Expected state %q, got %q", RuntimeStateCrashing, state.Runtime)
	}

	// Should still be able to start (no cooldown)
	_, stateCheck, err := svc.getManifestAndState("test-agent")
	if err != nil {
		t.Fatalf("getManifestAndState failed: %v", err)
	}
	if err := svc.blockIfCrashLoopCoolingDown("test-agent", stateCheck); err != nil {
		t.Errorf("Expected no cooldown after single crash, got error: %v", err)
	}
}

// TestCrashLoopDetector_DetectsCrashLoop verifies that N crashes within the time window trigger crash-loop state.
func TestCrashLoopDetector_DetectsCrashLoop(t *testing.T) {
	now := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	mockNow := func() time.Time { return now }

	svc := NewService(
		baseagent.NoopTriager{},
		WithNow(mockNow),
		WithCrashLoopConfig(3, 5*time.Minute, 5*time.Minute),
	)

	m := testManifest("test-agent")
	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("RegisterManifest failed: %v", err)
	}

	// Install the agent
	if err := svc.Install(context.Background(), "test-agent"); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Simulate 3 crashes within the window
	for i := 0; i < 3; i++ {
		now = now.Add(30 * time.Second) // Advance time by 30s between crashes
		svc.updateStateOnStartError("test-agent", errMockFailure)
	}

	state, err := svc.Status("test-agent")
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}

	// Should be in crash-loop state
	if state.Runtime != RuntimeStateCrashLoop {
		t.Errorf("Expected state %q, got %q", RuntimeStateCrashLoop, state.Runtime)
	}

	// Should have a cooldown set
	svc.mu.RLock()
	cooldownUntil, hasCooldown := svc.cooldowns["test-agent"]
	svc.mu.RUnlock()

	if !hasCooldown {
		t.Fatal("Expected cooldown to be set after crash-loop detection")
	}

	expectedCooldown := now.Add(5 * time.Minute)
	if !cooldownUntil.Equal(expectedCooldown) {
		t.Errorf("Expected cooldown until %v, got %v", expectedCooldown, cooldownUntil)
	}

	// Should block restart attempts
	_, stateCheck, err := svc.getManifestAndState("test-agent")
	if err != nil {
		t.Fatalf("getManifestAndState failed: %v", err)
	}
	if err := svc.blockIfCrashLoopCoolingDown("test-agent", stateCheck); err == nil {
		t.Error("Expected blockIfCrashLoopCoolingDown to return error during cooldown")
	} else if err != ErrCrashLoop && !isWrappedError(err, ErrCrashLoop) {
		t.Errorf("Expected ErrCrashLoop, got: %v", err)
	}
}

// TestCrashLoopDetector_WindowExpiry verifies that old crashes don't count toward crash-loop detection.
func TestCrashLoopDetector_WindowExpiry(t *testing.T) {
	now := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	var currentTime time.Time
	mockNow := func() time.Time { return currentTime }

	svc := NewService(
		baseagent.NoopTriager{},
		WithNow(mockNow),
		WithCrashLoopConfig(3, 5*time.Minute, 5*time.Minute),
	)

	m := testManifest("test-agent")
	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("RegisterManifest failed: %v", err)
	}

	// Install the agent
	currentTime = now
	if err := svc.Install(context.Background(), "test-agent"); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Crash 1: at T+0
	currentTime = now
	svc.updateStateOnStartError("test-agent", errMockFailure)

	// Crash 2: at T+1min (still within window)
	currentTime = now.Add(1 * time.Minute)
	svc.updateStateOnStartError("test-agent", errMockFailure)

	// Wait 7 minutes (outside the 5-minute window from both crashes)
	currentTime = now.Add(7 * time.Minute)

	// Crash 3: at T+7min (first two crashes should be expired)
	// Window at T+7min: starts at T+2min, so crashes at T+0 and T+1min are both expired
	svc.updateStateOnStartError("test-agent", errMockFailure)

	state, err := svc.Status("test-agent")
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}

	// Should be in crashing state, NOT crash-loop (old crashes expired)
	if state.Runtime != RuntimeStateCrashing {
		t.Errorf("Expected state %q after window expiry, got %q", RuntimeStateCrashing, state.Runtime)
	}

	// Should NOT have a cooldown
	svc.mu.RLock()
	cooldownTime, hasCooldown := svc.cooldowns["test-agent"]
	restarts := svc.restarts["test-agent"]
	restartCount := len(restarts)
	svc.mu.RUnlock()

	if hasCooldown && !cooldownTime.IsZero() {
		t.Errorf("Expected no cooldown after window expiry, but found cooldown until %v. Restarts: %v", cooldownTime, restarts)
	}

	// Should only have 1 restart in history (the latest one)
	if restartCount != 1 {
		t.Errorf("Expected 1 restart in history, got %d: %v", restartCount, restarts)
	}
}

// TestCrashLoopDetector_CooldownExpiry verifies that restart is allowed after cooldown expires.
func TestCrashLoopDetector_CooldownExpiry(t *testing.T) {
	now := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	var currentTime time.Time
	mockNow := func() time.Time { return currentTime }

	svc := NewService(
		baseagent.NoopTriager{},
		WithNow(mockNow),
		WithCrashLoopConfig(3, 5*time.Minute, 5*time.Minute),
	)

	m := testManifest("test-agent")
	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("RegisterManifest failed: %v", err)
	}

	// Install the agent
	currentTime = now
	if err := svc.Install(context.Background(), "test-agent"); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Trigger crash-loop (3 crashes within window)
	for i := 0; i < 3; i++ {
		currentTime = now.Add(time.Duration(i) * 30 * time.Second)
		svc.updateStateOnStartError("test-agent", errMockFailure)
	}

	// Verify crash-loop is active
	_, stateCheck, err := svc.getManifestAndState("test-agent")
	if err != nil {
		t.Fatalf("getManifestAndState failed: %v", err)
	}
	if err := svc.blockIfCrashLoopCoolingDown("test-agent", stateCheck); err == nil {
		t.Fatal("Expected cooldown to be active")
	}

	// Advance time past cooldown (5 minutes + 1 second)
	currentTime = currentTime.Add(5*time.Minute + 1*time.Second)

	// Should now be allowed to restart
	_, stateCheck, err = svc.getManifestAndState("test-agent")
	if err != nil {
		t.Fatalf("getManifestAndState failed: %v", err)
	}
	if err := svc.blockIfCrashLoopCoolingDown("test-agent", stateCheck); err != nil {
		t.Errorf("Expected restart to be allowed after cooldown expiry, got error: %v", err)
	}

	// Cooldown and restart history should be cleared
	svc.mu.RLock()
	_, hasCooldown := svc.cooldowns["test-agent"]
	_, hasRestarts := svc.restarts["test-agent"]
	svc.mu.RUnlock()

	if hasCooldown {
		t.Error("Expected cooldown to be cleared after expiry")
	}
	if hasRestarts {
		t.Error("Expected restart history to be cleared after cooldown expiry")
	}
}

// TestCrashLoopDetector_CustomConfig verifies that custom crash-loop configuration works.
func TestCrashLoopDetector_CustomConfig(t *testing.T) {
	now := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	mockNow := func() time.Time { return now }

	// Use custom config: 5 crashes in 10 minutes
	svc := NewService(
		baseagent.NoopTriager{},
		WithNow(mockNow),
		WithCrashLoopConfig(5, 10*time.Minute, 10*time.Minute),
	)

	m := testManifest("test-agent")
	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("RegisterManifest failed: %v", err)
	}

	// Install the agent
	if err := svc.Install(context.Background(), "test-agent"); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Simulate 4 crashes (below threshold)
	for i := 0; i < 4; i++ {
		now = now.Add(30 * time.Second)
		svc.updateStateOnStartError("test-agent", errMockFailure)
	}

	state, err := svc.Status("test-agent")
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}

	// Should still be in crashing state (not crash-loop yet)
	if state.Runtime != RuntimeStateCrashing {
		t.Errorf("Expected state %q with 4 crashes (threshold=5), got %q", RuntimeStateCrashing, state.Runtime)
	}

	// One more crash should trigger crash-loop
	now = now.Add(30 * time.Second)
	svc.updateStateOnStartError("test-agent", errMockFailure)

	state, err = svc.Status("test-agent")
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}

	// Should now be in crash-loop state
	if state.Runtime != RuntimeStateCrashLoop {
		t.Errorf("Expected state %q after 5 crashes (threshold=5), got %q", RuntimeStateCrashLoop, state.Runtime)
	}
}

// Helper error for testing
var errMockFailure = errTest("mock start failure")

type errTest string

func (e errTest) Error() string { return string(e) }

// isWrappedError checks if err wraps target
func isWrappedError(err, target error) bool {
	for err != nil {
		if err == target {
			return true
		}
		// Simple unwrapping - production code would use errors.Unwrap
		if e, ok := err.(interface{ Unwrap() error }); ok {
			err = e.Unwrap()
		} else {
			return false
		}
	}
	return false
}
