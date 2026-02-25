package lifecycle

import (
	"context"
	"strings"
	"testing"
	"time"

	"carrier/baseagent"
	"carrier/daemon/internal/manifest"
)

func TestRepairManager_RecordAttempt(t *testing.T) {
	svc := NewService(baseagent.NoopTriager{})
	rm := NewRepairManagerWithDefaults(svc)

	agentID := "test-agent"

	// Initially should be 0
	if count := rm.GetAttemptCount(agentID, RepairActionRestart); count != 0 {
		t.Errorf("expected 0 attempts, got %d", count)
	}

	// Record first attempt
	rm.RecordAttempt(agentID, RepairActionRestart)
	if count := rm.GetAttemptCount(agentID, RepairActionRestart); count != 1 {
		t.Errorf("expected 1 attempt, got %d", count)
	}

	// Record second attempt
	rm.RecordAttempt(agentID, RepairActionRestart)
	if count := rm.GetAttemptCount(agentID, RepairActionRestart); count != 2 {
		t.Errorf("expected 2 attempts, got %d", count)
	}

	// Different action type should be independent
	if count := rm.GetAttemptCount(agentID, RepairActionRebind); count != 0 {
		t.Errorf("expected 0 rebind attempts, got %d", count)
	}
}

func TestRepairManager_CanAttempt(t *testing.T) {
	svc := NewService(baseagent.NoopTriager{})
	config := RepairConfig{
		MaxRestartAttempts:  3,
		MaxRebindAttempts:   2,
		MaxRollbackAttempts: 1,
	}
	rm := NewRepairManager(svc, config)

	agentID := "test-agent"

	// Should be able to attempt initially
	if !rm.CanAttempt(agentID, RepairActionRestart) {
		t.Error("should be able to restart initially")
	}

	// Record attempts up to limit
	for i := 0; i < 3; i++ {
		rm.RecordAttempt(agentID, RepairActionRestart)
	}

	// Should no longer be able to attempt
	if rm.CanAttempt(agentID, RepairActionRestart) {
		t.Error("should not be able to restart after max attempts")
	}

	// Different action should still be allowed
	if !rm.CanAttempt(agentID, RepairActionRebind) {
		t.Error("should be able to rebind even after restart limit")
	}
}

func TestRepairManager_RecordSuccess(t *testing.T) {
	svc := NewService(baseagent.NoopTriager{})
	rm := NewRepairManagerWithDefaults(svc)

	agentID := "test-agent"

	// Record some attempts
	rm.RecordAttempt(agentID, RepairActionRestart)
	rm.RecordAttempt(agentID, RepairActionRestart)
	rm.RecordAttempt(agentID, RepairActionRebind)

	if count := rm.GetAttemptCount(agentID, RepairActionRestart); count != 2 {
		t.Errorf("expected 2 restart attempts, got %d", count)
	}

	// Record success
	beforeSuccess := time.Now()
	rm.RecordSuccess(agentID)
	afterSuccess := time.Now()

	// All attempts should be reset
	if count := rm.GetAttemptCount(agentID, RepairActionRestart); count != 0 {
		t.Errorf("expected 0 restart attempts after success, got %d", count)
	}
	if count := rm.GetAttemptCount(agentID, RepairActionRebind); count != 0 {
		t.Errorf("expected 0 rebind attempts after success, got %d", count)
	}

	// Last success should be recorded
	lastSuccess := rm.GetLastSuccess(agentID)
	if lastSuccess.Before(beforeSuccess) || lastSuccess.After(afterSuccess) {
		t.Errorf("last success timestamp out of expected range: %v", lastSuccess)
	}
}

func TestRepairManager_DecideAction_Healthy(t *testing.T) {
	svc := NewService(baseagent.NoopTriager{})
	rm := NewRepairManagerWithDefaults(svc)

	agentID := "test-agent"
	state := AgentState{
		ID:      agentID,
		Runtime: RuntimeStateRunning,
		Health:  HealthStateHealthy,
	}

	action := rm.DecideAction(agentID, state)

	if action.Type != RepairActionEscalate {
		t.Errorf("expected escalate for healthy agent, got %s", action.Type)
	}
	if action.Reason != "agent is healthy, no repair needed" {
		t.Errorf("unexpected reason: %s", action.Reason)
	}
}

func TestRepairManager_DecideAction_Crashing(t *testing.T) {
	svc := NewService(baseagent.NoopTriager{})
	rm := NewRepairManagerWithDefaults(svc)

	agentID := "test-agent"
	state := AgentState{
		ID:      agentID,
		Runtime: RuntimeStateCrashing,
		Health:  HealthStateUnhealthy,
	}

	action := rm.DecideAction(agentID, state)

	if action.Type != RepairActionRestart {
		t.Errorf("expected restart for crashing agent, got %s", action.Type)
	}
}

func TestRepairManager_DecideAction_PortConflict(t *testing.T) {
	svc := NewService(baseagent.NoopTriager{})
	rm := NewRepairManagerWithDefaults(svc)

	agentID := "test-agent"

	// Exhaust restart attempts
	for i := 0; i < 5; i++ {
		rm.RecordAttempt(agentID, RepairActionRestart)
	}

	state := AgentState{
		ID:        agentID,
		Runtime:   RuntimeStateCrashing,
		Health:    HealthStateUnhealthy,
		LastError: "failed to start: bind: address already in use",
	}

	action := rm.DecideAction(agentID, state)

	if action.Type != RepairActionRebind {
		t.Errorf("expected rebind for port conflict, got %s", action.Type)
	}
}

func TestRepairManager_DecideAction_Rollback(t *testing.T) {
	svc := NewService(baseagent.NoopTriager{})
	rm := NewRepairManagerWithDefaults(svc)

	agentID := "test-agent"

	// Exhaust restart and rebind attempts
	for i := 0; i < 5; i++ {
		rm.RecordAttempt(agentID, RepairActionRestart)
	}
	for i := 0; i < 2; i++ {
		rm.RecordAttempt(agentID, RepairActionRebind)
	}

	state := AgentState{
		ID:      agentID,
		Version: "1.2.0",
		Runtime: RuntimeStateCrashing,
		Health:  HealthStateUnhealthy,
	}

	action := rm.DecideAction(agentID, state)

	if action.Type != RepairActionRollback {
		t.Errorf("expected rollback after other attempts exhausted, got %s", action.Type)
	}
}

func TestRepairManager_DecideAction_AllExhausted(t *testing.T) {
	svc := NewService(baseagent.NoopTriager{})
	rm := NewRepairManagerWithDefaults(svc)

	agentID := "test-agent"

	// Exhaust all repair attempts
	for i := 0; i < 5; i++ {
		rm.RecordAttempt(agentID, RepairActionRestart)
	}
	for i := 0; i < 2; i++ {
		rm.RecordAttempt(agentID, RepairActionRebind)
	}
	rm.RecordAttempt(agentID, RepairActionRollback)

	state := AgentState{
		ID:      agentID,
		Version: "1.2.0",
		Runtime: RuntimeStateCrashing,
		Health:  HealthStateUnhealthy,
	}

	action := rm.DecideAction(agentID, state)

	if action.Type != RepairActionEscalate {
		t.Errorf("expected escalate after all attempts exhausted, got %s", action.Type)
	}
	if action.Reason != "all repair attempts exhausted" {
		t.Errorf("unexpected reason: %s", action.Reason)
	}
}

func TestRepairManager_ExecuteRepair_Restart(t *testing.T) {
	svc := NewService(baseagent.NoopTriager{})
	m := manifest.Manifest{
		ID:      "test-agent",
		Name:    "Test Agent",
		Version: "1.0.0",
		Runtime: manifest.RuntimeSpec{
			Type: manifest.RuntimeTypeLocalBinary,
			Install: manifest.CommandSpec{
				Command: "echo installed",
			},
			Start: manifest.CommandSpec{
				Command: "sleep 1000",
			},
			Stop: manifest.CommandSpec{
				Command: "echo stop",
			},
		},
		Memory: manifest.MemorySpec{
			MountPath: "/tmp/test-repair",
			Supports:  []manifest.MemoryType{manifest.MemoryTypePerAgent},
		},
	}

	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("failed to register manifest: %v", err)
	}

	ctx := context.Background()
	if err := svc.Install(ctx, m.ID); err != nil {
		t.Fatalf("failed to install: %v", err)
	}

	rm := NewRepairManagerWithDefaults(svc)

	action := RepairOutcome{
		Type:      RepairActionRestart,
		AgentID:   m.ID,
		Reason:    "test",
		Timestamp: time.Now(),
	}

	// Execute restart
	if err := rm.ExecuteRepair(action); err != nil {
		t.Errorf("restart repair failed: %v", err)
	}

	// Check attempt was recorded
	if count := rm.GetAttemptCount(m.ID, RepairActionRestart); count != 1 {
		t.Errorf("expected 1 restart attempt recorded, got %d", count)
	}

	// Cleanup
	_ = svc.Stop(ctx, m.ID)
}

func TestRepairManager_ExecuteRepair_Escalate(t *testing.T) {
	svc := NewService(baseagent.NoopTriager{})
	rm := NewRepairManagerWithDefaults(svc)

	action := RepairOutcome{
		Type:      RepairActionEscalate,
		AgentID:   "test-agent",
		Reason:    "all attempts exhausted",
		Timestamp: time.Now(),
	}

	err := rm.ExecuteRepair(action)
	if err != ErrAllAttemptsExhausted {
		t.Errorf("expected ErrAllAttemptsExhausted, got %v", err)
	}
}

func TestRepairManager_ExecuteRepair_Rebind(t *testing.T) {
	svc := NewService(baseagent.NoopTriager{})
	rm := NewRepairManagerWithDefaults(svc)

	action := RepairOutcome{
		Type:          RepairActionRebind,
		AgentID:       "test-agent",
		Reason:        "port conflict",
		SuggestedPort: 18080,
		Timestamp:     time.Now(),
	}

	err := rm.ExecuteRepair(action)
	if err == nil {
		t.Fatal("expected rebind to return placeholder service-layer error")
	}
	if !strings.Contains(err.Error(), "service-layer port allocation") {
		t.Fatalf("unexpected rebind error: %v", err)
	}
	if count := rm.GetAttemptCount(action.AgentID, RepairActionRebind); count != 1 {
		t.Fatalf("expected 1 rebind attempt recorded, got %d", count)
	}
}

func TestRepairManager_ExecuteRepair_Rollback_MissingBackupPath(t *testing.T) {
	svc := NewService(baseagent.NoopTriager{})
	rm := NewRepairManagerWithDefaults(svc)

	action := RepairOutcome{
		Type:      RepairActionRollback,
		AgentID:   "test-agent",
		Reason:    "version rollback",
		Timestamp: time.Now(),
	}

	err := rm.ExecuteRepair(action)
	if err == nil {
		t.Fatal("expected rollback without backup path to fail")
	}
	if !strings.Contains(err.Error(), "valid backup path") {
		t.Fatalf("unexpected rollback error: %v", err)
	}
	if count := rm.GetAttemptCount(action.AgentID, RepairActionRollback); count != 1 {
		t.Fatalf("expected 1 rollback attempt recorded, got %d", count)
	}
}

func TestRepairManager_ExecuteRepair_Rollback_StopFailure(t *testing.T) {
	svc := NewService(baseagent.NoopTriager{})
	rm := NewRepairManagerWithDefaults(svc)

	action := RepairOutcome{
		Type:       RepairActionRollback,
		AgentID:    "missing-agent",
		Reason:     "rollback requested",
		BackupPath: "/tmp/backup.tar.gz",
		Timestamp:  time.Now(),
	}

	err := rm.ExecuteRepair(action)
	if err == nil {
		t.Fatal("expected rollback stop failure for missing agent")
	}
	if !strings.Contains(err.Error(), "rollback stop failed") {
		t.Fatalf("unexpected rollback stop error: %v", err)
	}
}

func TestRepairManager_ExecuteRepair_Rollback_AlreadyStoppedBranch(t *testing.T) {
	svc := NewService(baseagent.NoopTriager{})
	rm := NewRepairManagerWithDefaults(svc)

	m := manifest.Manifest{
		ID:      "rollback-agent",
		Name:    "Rollback Agent",
		Version: "1.0.0",
		Runtime: manifest.RuntimeSpec{
			Type: manifest.RuntimeTypeLocalBinary,
			Install: manifest.CommandSpec{
				Command: "echo installed",
			},
			Start: manifest.CommandSpec{
				Command: "sleep 1000",
			},
			Stop: manifest.CommandSpec{
				Command: "echo stop",
			},
		},
		Memory: manifest.MemorySpec{
			MountPath: "/tmp/test-repair-rollback",
			Supports:  []manifest.MemoryType{manifest.MemoryTypePerAgent},
		},
	}

	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("failed to register manifest: %v", err)
	}

	// Agent is in stopped state, so Stop() returns ErrAlreadyStopped.
	action := RepairOutcome{
		Type:       RepairActionRollback,
		AgentID:    m.ID,
		Reason:     "rollback requested",
		BackupPath: "/tmp/rollback-agent-backup.tar.gz",
		Timestamp:  time.Now(),
	}

	err := rm.ExecuteRepair(action)
	if err == nil {
		t.Fatal("expected placeholder rollback restoration error")
	}
	if !strings.Contains(err.Error(), "backup restoration") || !strings.Contains(err.Error(), action.BackupPath) {
		t.Fatalf("unexpected rollback placeholder error: %v", err)
	}
}

func TestRepairManager_RepairLoop_Healthy(t *testing.T) {
	svc := NewService(baseagent.NoopTriager{})
	m := manifest.Manifest{
		ID:      "test-agent",
		Name:    "Test Agent",
		Version: "1.0.0",
		Runtime: manifest.RuntimeSpec{
			Type: manifest.RuntimeTypeLocalBinary,
			Install: manifest.CommandSpec{
				Command: "echo installed",
			},
			Start: manifest.CommandSpec{
				Command: "sleep 1000",
			},
			Stop: manifest.CommandSpec{
				Command: "echo stop",
			},
		},
		Memory: manifest.MemorySpec{
			MountPath: "/tmp/test-repair-healthy",
			Supports:  []manifest.MemoryType{manifest.MemoryTypePerAgent},
		},
	}

	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("failed to register manifest: %v", err)
	}

	ctx := context.Background()
	if err := svc.Install(ctx, m.ID); err != nil {
		t.Fatalf("failed to install: %v", err)
	}

	if err := svc.Start(ctx, m.ID); err != nil {
		t.Fatalf("failed to start: %v", err)
	}

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Manually set state to healthy for test
	svc.mu.Lock()
	state := svc.states[m.ID]
	state.Runtime = RuntimeStateRunning
	state.Health = HealthStateHealthy
	svc.states[m.ID] = state
	svc.mu.Unlock()

	rm := NewRepairManagerWithDefaults(svc)

	err := rm.RepairLoop(m.ID)
	if err != ErrNoRepairNeeded {
		t.Errorf("expected ErrNoRepairNeeded for healthy agent, got %v", err)
	}

	// Cleanup
	_ = svc.Stop(ctx, m.ID)
}

func TestRepairManager_RepairLoop_Exhausted(t *testing.T) {
	svc := NewService(baseagent.NoopTriager{})
	m := manifest.Manifest{
		ID:      "test-agent",
		Name:    "Test Agent",
		Version: "1.0.0",
		Runtime: manifest.RuntimeSpec{
			Type: manifest.RuntimeTypeLocalBinary,
			Install: manifest.CommandSpec{
				Command: "echo installed",
			},
			Start: manifest.CommandSpec{
				Command: "sleep 1000",
			},
			Stop: manifest.CommandSpec{
				Command: "echo stop",
			},
		},
		Memory: manifest.MemorySpec{
			MountPath: "/tmp/test-repair-exhausted",
			Supports:  []manifest.MemoryType{manifest.MemoryTypePerAgent},
		},
	}

	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("failed to register manifest: %v", err)
	}

	ctx := context.Background()
	if err := svc.Install(ctx, m.ID); err != nil {
		t.Fatalf("failed to install: %v", err)
	}

	rm := NewRepairManagerWithDefaults(svc)

	// Exhaust all attempts
	for i := 0; i < 5; i++ {
		rm.RecordAttempt(m.ID, RepairActionRestart)
	}
	for i := 0; i < 2; i++ {
		rm.RecordAttempt(m.ID, RepairActionRebind)
	}
	rm.RecordAttempt(m.ID, RepairActionRollback)

	err := rm.RepairLoop(m.ID)
	if err != ErrAllAttemptsExhausted {
		t.Errorf("expected ErrAllAttemptsExhausted, got %v", err)
	}
}

func TestContainsPortConflict(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		expected bool
	}{
		{
			name:     "exact match",
			errMsg:   "port conflict detected",
			expected: true,
		},
		{
			name:     "address already in use",
			errMsg:   "failed to bind: address already in use",
			expected: true,
		},
		{
			name:     "case insensitive",
			errMsg:   "ERROR: Port Conflict on 8080",
			expected: true,
		},
		{
			name:     "no conflict",
			errMsg:   "some other error",
			expected: false,
		},
		{
			name:     "empty string",
			errMsg:   "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsPortConflict(tt.errMsg)
			if result != tt.expected {
				t.Errorf("containsPortConflict(%q) = %v, expected %v", tt.errMsg, result, tt.expected)
			}
		})
	}
}

func TestRepairConfig_Defaults(t *testing.T) {
	config := DefaultRepairConfig()

	if config.MaxRestartAttempts != 5 {
		t.Errorf("expected default restart attempts to be 5, got %d", config.MaxRestartAttempts)
	}
	if config.MaxRebindAttempts != 2 {
		t.Errorf("expected default rebind attempts to be 2, got %d", config.MaxRebindAttempts)
	}
	if config.MaxRollbackAttempts != 1 {
		t.Errorf("expected default rollback attempts to be 1, got %d", config.MaxRollbackAttempts)
	}
}

func TestRepairManager_MultipleAgents(t *testing.T) {
	svc := NewService(baseagent.NoopTriager{})
	rm := NewRepairManagerWithDefaults(svc)

	agent1 := "agent-1"
	agent2 := "agent-2"

	// Record attempts for agent1
	rm.RecordAttempt(agent1, RepairActionRestart)
	rm.RecordAttempt(agent1, RepairActionRestart)

	// Record attempts for agent2
	rm.RecordAttempt(agent2, RepairActionRestart)

	// Attempts should be independent
	if count := rm.GetAttemptCount(agent1, RepairActionRestart); count != 2 {
		t.Errorf("expected agent1 to have 2 restart attempts, got %d", count)
	}
	if count := rm.GetAttemptCount(agent2, RepairActionRestart); count != 1 {
		t.Errorf("expected agent2 to have 1 restart attempt, got %d", count)
	}

	// Recording success for agent1 should not affect agent2
	rm.RecordSuccess(agent1)
	if count := rm.GetAttemptCount(agent1, RepairActionRestart); count != 0 {
		t.Errorf("expected agent1 restart attempts to be reset to 0, got %d", count)
	}
	if count := rm.GetAttemptCount(agent2, RepairActionRestart); count != 1 {
		t.Errorf("expected agent2 restart attempts to remain 1, got %d", count)
	}
}
