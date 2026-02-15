package lifecycle

import (
	"context"
	"testing"
	"time"

	"carrier/daemon/internal/baseagent"
	"carrier/daemon/internal/manifest"
)

func TestRepairManager_ExecuteRepair_Restart(t *testing.T) {
	ctx := context.Background()
	svc := newTestService()
	config := RepairConfig{
		MaxRestartAttempts:  3,
		MaxRebindAttempts:   2,
		MaxRollbackAttempts: 1,
		SuccessThreshold:    5 * time.Minute,
	}
	rm := NewRepairManager(svc, config)

	// Register and install a test agent
	m := newTestManifest("agent1", nil)
	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("Failed to register manifest: %v", err)
	}
	svc.mu.Lock()
	state := svc.states["agent1"]
	state.Install = InstallStateInstalled
	svc.states["agent1"] = state
	svc.mu.Unlock()

	// Triage result suggesting restart
	triage := baseagent.TriageResult{
		Resolved:         false,
		Summary:          "Process crashed",
		SuggestedActions: []string{"restart"},
	}

	// Execute repair
	result := rm.ExecuteRepair(ctx, "agent1", triage)
	if !result.Success {
		t.Errorf("Expected successful repair, got error: %v", result.Error)
	}
	if result.Action != RepairActionRestart {
		t.Errorf("Expected restart action, got %v", result.Action)
	}

	// Check attempt counter
	attempts := rm.GetAttempts("agent1")
	if attempts.Restarts != 1 {
		t.Errorf("Expected 1 restart attempt, got %d", attempts.Restarts)
	}
}

func TestRepairManager_AttemptLimits(t *testing.T) {
	ctx := context.Background()
	svc := newTestService()
	config := RepairConfig{
		MaxRestartAttempts:  2,
		MaxRebindAttempts:   1,
		MaxRollbackAttempts: 1,
		SuccessThreshold:    5 * time.Minute,
	}
	rm := NewRepairManager(svc, config)

	// Register and install a test agent
	ports := []manifest.PortSpec{
		{Name: "http", Port: 8080, Protocol: "tcp"},
	}
	m := newTestManifest("agent1", ports)
	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("Failed to register manifest: %v", err)
	}
	svc.mu.Lock()
	state := svc.states["agent1"]
	state.Install = InstallStateInstalled
	svc.states["agent1"] = state
	svc.mu.Unlock()

	triage := baseagent.TriageResult{
		Resolved:         false,
		Summary:          "Process crashed",
		SuggestedActions: []string{"restart"},
	}

	// First restart - should succeed
	result := rm.ExecuteRepair(ctx, "agent1", triage)
	if !result.Success {
		t.Fatalf("First restart should succeed: %v", result.Error)
	}
	if result.Action != RepairActionRestart {
		t.Errorf("Expected restart, got %v", result.Action)
	}

	// Second restart - should succeed
	result = rm.ExecuteRepair(ctx, "agent1", triage)
	if !result.Success {
		t.Fatalf("Second restart should succeed: %v", result.Error)
	}
	if result.Action != RepairActionRestart {
		t.Errorf("Expected restart, got %v", result.Action)
	}

	// Third restart - should fallback to rebind
	result = rm.ExecuteRepair(ctx, "agent1", triage)
	if !result.Success {
		t.Fatalf("Fallback to rebind should succeed: %v", result.Error)
	}
	if result.Action != RepairActionRebind {
		t.Errorf("Expected rebind after restart exhausted, got %v", result.Action)
	}

	// Fourth attempt - should fallback to rollback
	result = rm.ExecuteRepair(ctx, "agent1", triage)
	if !result.Success {
		t.Fatalf("Fallback to rollback should succeed: %v", result.Error)
	}
	if result.Action != RepairActionRollback {
		t.Errorf("Expected rollback after rebind exhausted, got %v", result.Action)
	}

	// Fifth attempt - all exhausted, should need intervention
	result = rm.ExecuteRepair(ctx, "agent1", triage)
	if result.Success {
		t.Error("Expected failure when all attempts exhausted")
	}
	if !result.NeedsIntervention {
		t.Error("Expected NeedsIntervention flag when all attempts exhausted")
	}
}

func TestRepairManager_ResetOnSuccess(t *testing.T) {
	svc := newTestService()
	config := DefaultRepairConfig()
	rm := NewRepairManager(svc, config)

	agentID := "agent1"
	rm.attempts[agentID] = &RepairAttempts{
		Restarts:  2,
		Rebinds:   1,
		Rollbacks: 1,
	}

	// Running for less than threshold - should not reset
	runningSince := time.Now().Add(-3 * time.Minute)
	rm.ResetOnSuccess(agentID, runningSince)

	attempts := rm.GetAttempts(agentID)
	if attempts.Restarts != 2 {
		t.Errorf("Should not reset before threshold, expected 2 restarts, got %d", attempts.Restarts)
	}

	// Running for longer than threshold - should reset
	runningSince = time.Now().Add(-6 * time.Minute)
	rm.ResetOnSuccess(agentID, runningSince)

	attempts = rm.GetAttempts(agentID)
	if attempts.Restarts != 0 {
		t.Errorf("Expected reset to 0 restarts, got %d", attempts.Restarts)
	}
	if attempts.Rebinds != 0 {
		t.Errorf("Expected reset to 0 rebinds, got %d", attempts.Rebinds)
	}
	if attempts.Rollbacks != 0 {
		t.Errorf("Expected reset to 0 rollbacks, got %d", attempts.Rollbacks)
	}
}

func TestRepairManager_SelectAction(t *testing.T) {
	svc := newTestService()
	rm := NewRepairManager(svc, DefaultRepairConfig())

	tests := []struct {
		name        string
		suggestions []string
		expected    RepairAction
	}{
		{
			name:        "restart suggestion",
			suggestions: []string{"restart"},
			expected:    RepairActionRestart,
		},
		{
			name:        "restart service suggestion",
			suggestions: []string{"Restart service"},
			expected:    RepairActionRestart,
		},
		{
			name:        "rebind suggestion",
			suggestions: []string{"Rebind port"},
			expected:    RepairActionRebind,
		},
		{
			name:        "rollback suggestion",
			suggestions: []string{"Rollback version"},
			expected:    RepairActionRollback,
		},
		{
			name:        "multiple suggestions - first match",
			suggestions: []string{"Check logs", "restart", "rebind"},
			expected:    RepairActionRestart,
		},
		{
			name:        "no recognized suggestion",
			suggestions: []string{"Check logs", "Call support"},
			expected:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := rm.selectAction(tt.suggestions)
			if action != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, action)
			}
		})
	}
}

func TestRepairManager_RemainingAttempts(t *testing.T) {
	svc := newTestService()
	config := RepairConfig{
		MaxRestartAttempts:  3,
		MaxRebindAttempts:   2,
		MaxRollbackAttempts: 1,
	}
	rm := NewRepairManager(svc, config)

	agentID := "agent1"
	rm.attempts[agentID] = &RepairAttempts{
		Restarts:  1,
		Rebinds:   0,
		Rollbacks: 0,
	}

	remaining := rm.remainingAttempts(agentID)

	if remaining[RepairActionRestart] != 2 {
		t.Errorf("Expected 2 remaining restarts, got %d", remaining[RepairActionRestart])
	}
	if remaining[RepairActionRebind] != 2 {
		t.Errorf("Expected 2 remaining rebinds, got %d", remaining[RepairActionRebind])
	}
	if remaining[RepairActionRollback] != 1 {
		t.Errorf("Expected 1 remaining rollback, got %d", remaining[RepairActionRollback])
	}
}

func TestRepairManager_ExecuteRestart_WithBackoff(t *testing.T) {
	ctx := context.Background()
	svc := newTestService()
	config := RepairConfig{
		MaxRestartAttempts: 3,
	}
	rm := NewRepairManager(svc, config)

	// Register and install a test agent
	m := newTestManifest("agent1", nil)
	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("Failed to register manifest: %v", err)
	}
	svc.mu.Lock()
	state := svc.states["agent1"]
	state.Install = InstallStateInstalled
	svc.states["agent1"] = state
	svc.mu.Unlock()

	// Initialize attempts
	rm.attempts["agent1"] = &RepairAttempts{Restarts: 0}

	// First restart - 1s backoff
	start := time.Now()
	err := rm.executeRestart(ctx, "agent1")
	duration := time.Since(start)

	if err != nil {
		t.Errorf("Restart failed: %v", err)
	}
	if duration < time.Second {
		t.Errorf("Expected at least 1s backoff, got %v", duration)
	}

	// Increment for next test
	rm.attempts["agent1"].Restarts = 1

	// Second restart - 2s backoff
	start = time.Now()
	err = rm.executeRestart(ctx, "agent1")
	duration = time.Since(start)

	if err != nil {
		t.Errorf("Restart failed: %v", err)
	}
	if duration < 2*time.Second {
		t.Errorf("Expected at least 2s backoff, got %v", duration)
	}
}

func TestRepairManager_ExecuteRebind(t *testing.T) {
	ctx := context.Background()
	svc := newTestService()
	config := DefaultRepairConfig()
	rm := NewRepairManager(svc, config)

	// Register and install a test agent with a port
	ports := []manifest.PortSpec{
		{Name: "http", Port: 8080, Protocol: "tcp"},
	}
	m := newTestManifest("agent1", ports)
	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("Failed to register manifest: %v", err)
	}
	svc.mu.Lock()
	state := svc.states["agent1"]
	state.Install = InstallStateInstalled
	svc.states["agent1"] = state
	svc.mu.Unlock()

	rm.attempts["agent1"] = &RepairAttempts{}

	// Execute rebind
	err := rm.executeRebind(ctx, "agent1")
	if err != nil {
		t.Errorf("Rebind failed: %v", err)
	}

	// Check that port was changed
	svc.mu.RLock()
	updatedManifest := svc.manifests["agent1"]
	svc.mu.RUnlock()

	if len(updatedManifest.Network.Ports) == 0 {
		t.Fatal("Expected ports to be configured")
	}
	if updatedManifest.Network.Ports[0].Port == 8080 {
		t.Errorf("Expected port to change from 8080, but it didn't")
	}
}

func TestRepairManager_Rollback_NoBackup(t *testing.T) {
	ctx := context.Background()
	svc := newTestService()
	config := DefaultRepairConfig()
	rm := NewRepairManager(svc, config)

	// Register agent without backup path
	m := manifest.Manifest{
		ID:      "agent1",
		Name:    "Test Agent",
		Version: "1.0.0",
		Runtime: manifest.RuntimeSpec{
			Start: manifest.CommandSpec{Command: "echo running"},
		},
	}
	_ = svc.RegisterManifest(m)
	svc.mu.Lock()
	state := svc.states["agent1"]
	state.Install = InstallStateInstalled
	svc.states["agent1"] = state
	svc.mu.Unlock()

	rm.attempts["agent1"] = &RepairAttempts{}

	// Execute rollback - should fail due to missing backup
	err := rm.executeRollback(ctx, "agent1")
	if err == nil {
		t.Error("Expected error when rolling back without backup path")
	}
}

func TestRepairManager_ResolvedTriage(t *testing.T) {
	ctx := context.Background()
	svc := newTestService()
	rm := NewRepairManager(svc, DefaultRepairConfig())

	// Triage that's already resolved
	triage := baseagent.TriageResult{
		Resolved: true,
		Summary:  "Issue resolved automatically",
	}

	result := rm.ExecuteRepair(ctx, "agent1", triage)
	if !result.Success {
		t.Error("Expected success for already-resolved triage")
	}
	if result.Action != "" {
		t.Errorf("Expected no action for resolved triage, got %v", result.Action)
	}

	// Should not create attempt tracking
	if _, exists := rm.attempts["agent1"]; exists {
		t.Error("Should not track attempts for resolved triage")
	}
}

func TestRepairManager_GetAttempts_Copy(t *testing.T) {
	svc := newTestService()
	rm := NewRepairManager(svc, DefaultRepairConfig())

	agentID := "agent1"
	rm.attempts[agentID] = &RepairAttempts{
		Restarts: 2,
		Rebinds:  1,
	}

	// Get attempts
	attempts := rm.GetAttempts(agentID)

	// Modify the returned copy
	attempts.Restarts = 999

	// Original should be unchanged
	if rm.attempts[agentID].Restarts != 2 {
		t.Error("GetAttempts should return a copy, not the original")
	}
}

func TestRepairManager_NoRecognizedAction(t *testing.T) {
	ctx := context.Background()
	svc := newTestService()
	rm := NewRepairManager(svc, DefaultRepairConfig())

	triage := baseagent.TriageResult{
		Resolved:         false,
		Summary:          "Unknown issue",
		SuggestedActions: []string{"Check logs", "Call support"},
	}

	result := rm.ExecuteRepair(ctx, "agent1", triage)
	if result.Success {
		t.Error("Expected failure when no recognized action")
	}
	if !result.NeedsIntervention {
		t.Error("Expected NeedsIntervention when no action recognized")
	}
}

// Helper function for fixed time in tests
func fixedTime(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// newTestService creates a test service with minimal setup
func newTestService() *Service {
	return NewService(baseagent.NoopTriager{})
}

// newTestManifest creates a valid test manifest
func newTestManifest(id string, ports []manifest.PortSpec) manifest.Manifest {
	return manifest.Manifest{
		ID:      id,
		Name:    "Test Agent",
		Version: "1.0.0",
		Runtime: manifest.RuntimeSpec{
			Type:    manifest.RuntimeTypeLocalBinary,
			Install: manifest.CommandSpec{Command: "true"},
			Upgrade: manifest.CommandSpec{Command: "true"},
			Start:   manifest.CommandSpec{Command: "sleep 60"},
			Stop:    manifest.CommandSpec{Command: "killall sleep || true"},
		},
		Network: manifest.NetworkSpec{
			Ports: ports,
		},
		Memory: manifest.MemorySpec{
			Supports:  []manifest.MemoryType{manifest.MemoryTypePerAgent},
			MountPath: "/tmp/memory",
		},
		Env: manifest.EnvSpec{},
	}
}
