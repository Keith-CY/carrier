package lifecycle

import (
	"testing"
	"time"
)

func TestTriageEngine_OOMKilled(t *testing.T) {
	engine := NewTriageEngine(DefaultPolicyBounds())

	evidence := &CrashEvidence{
		AgentID:       "test-agent",
		ExitCode:      137,
		LogLines:      []string{"fatal error: out of memory"},
		CrashTime:     time.Now(),
		CrashCount:    1,
		LastStartTime: time.Now().Add(-5 * time.Minute),
	}

	decision := engine.Decide(evidence)

	if decision.Action != RepairActionEscalate {
		t.Errorf("Expected escalate for OOM, got %s", decision.Action)
	}

	if decision.Reason == "" {
		t.Error("Expected reason to be set")
	}
}

func TestTriageEngine_PortConflict(t *testing.T) {
	engine := NewTriageEngine(DefaultPolicyBounds())

	evidence := &CrashEvidence{
		AgentID:       "test-agent",
		ExitCode:      1,
		LogLines:      []string{"listen tcp :8080: bind: address already in use"},
		CrashTime:     time.Now(),
		CrashCount:    1,
		LastStartTime: time.Now().Add(-1 * time.Minute),
	}

	decision := engine.Decide(evidence)

	if decision.Action != RepairActionRebind {
		t.Errorf("Expected rebind for port conflict, got %s", decision.Action)
	}

	if decision.SuggestedParams["strategy"] != "increment" {
		t.Error("Expected strategy=increment in suggested params")
	}
}

func TestTriageEngine_MissingDependency(t *testing.T) {
	engine := NewTriageEngine(DefaultPolicyBounds())

	evidence := &CrashEvidence{
		AgentID:       "test-agent",
		ExitCode:      127,
		LogLines:      []string{"error: cannot find required library libfoo.so"},
		CrashTime:     time.Now(),
		CrashCount:    1,
		LastStartTime: time.Now().Add(-30 * time.Second),
	}

	decision := engine.Decide(evidence)

	if decision.Action != RepairActionRollback {
		t.Errorf("Expected rollback for missing dependency, got %s", decision.Action)
	}
}

func TestTriageEngine_GenericCrash(t *testing.T) {
	engine := NewTriageEngine(DefaultPolicyBounds())

	evidence := &CrashEvidence{
		AgentID:       "test-agent",
		ExitCode:      1,
		LogLines:      []string{"unexpected error occurred"},
		CrashTime:     time.Now(),
		CrashCount:    1,
		LastStartTime: time.Now().Add(-2 * time.Minute),
	}

	decision := engine.Decide(evidence)

	if decision.Action != RepairActionRestart {
		t.Errorf("Expected restart for generic crash, got %s", decision.Action)
	}

	if decision.SuggestedParams["backoff"] != "exponential" {
		t.Error("Expected backoff=exponential in suggested params")
	}
}

func TestTriageEngine_CrashLoop(t *testing.T) {
	engine := NewTriageEngine(DefaultPolicyBounds())

	// Simulate a crash loop (3+ crashes with short uptime)
	evidence := &CrashEvidence{
		AgentID:       "test-agent",
		ExitCode:      1,
		LogLines:      []string{"error: something bad happened"},
		CrashTime:     time.Now(),
		CrashCount:    3,
		LastStartTime: time.Now().Add(-5 * time.Second), // Very short uptime
	}

	decision := engine.Decide(evidence)

	if decision.Action != RepairActionEscalate {
		t.Errorf("Expected escalate for crash loop, got %s", decision.Action)
	}
}

func TestTriageEngine_RulePriority(t *testing.T) {
	engine := NewTriageEngine(DefaultPolicyBounds())

	// Exit code 1 with both port conflict and missing dependency indicators
	// Port conflict rule (priority 20) should win over missing dependency (priority 30)
	evidence := &CrashEvidence{
		AgentID:  "test-agent",
		ExitCode: 1,
		LogLines: []string{
			"error: cannot find library",
			"bind: address already in use",
		},
		CrashTime:     time.Now(),
		CrashCount:    1,
		LastStartTime: time.Now().Add(-1 * time.Minute),
	}

	decision := engine.Decide(evidence)

	if decision.Action != RepairActionRebind {
		t.Errorf("Expected rebind (higher priority rule), got %s", decision.Action)
	}
}

func TestTriageEngine_PolicyBounds_MaxRestarts(t *testing.T) {
	bounds := PolicyBounds{
		MaxRestartAttempts:  2,
		MaxRebindAttempts:   2,
		MaxRollbackAttempts: 1,
		RepairCooldownSecs:  1, // Short cooldown for testing
	}
	engine := NewTriageEngine(bounds)

	evidence := &CrashEvidence{
		AgentID:       "test-agent",
		ExitCode:      1,
		LogLines:      []string{"generic error"},
		CrashTime:     time.Now(),
		CrashCount:    1,
		LastStartTime: time.Now().Add(-1 * time.Minute),
	}

	// First two attempts should succeed
	decision1 := engine.Decide(evidence)
	if decision1.Action != RepairActionRestart {
		t.Errorf("First attempt: expected restart, got %s", decision1.Action)
	}

	time.Sleep(1100 * time.Millisecond) // Wait for cooldown

	decision2 := engine.Decide(evidence)
	if decision2.Action != RepairActionRestart {
		t.Errorf("Second attempt: expected restart, got %s", decision2.Action)
	}

	time.Sleep(1100 * time.Millisecond) // Wait for cooldown

	// Third attempt should escalate (exceeded max)
	decision3 := engine.Decide(evidence)
	if decision3.Action != RepairActionEscalate {
		t.Errorf("Third attempt: expected escalate, got %s", decision3.Action)
	}
}

func TestTriageEngine_PolicyBounds_Cooldown(t *testing.T) {
	bounds := PolicyBounds{
		MaxRestartAttempts:  5,
		MaxRebindAttempts:   2,
		MaxRollbackAttempts: 1,
		RepairCooldownSecs:  2, // 2 second cooldown
	}
	engine := NewTriageEngine(bounds)

	evidence := &CrashEvidence{
		AgentID:       "test-agent",
		ExitCode:      1,
		LogLines:      []string{"generic error"},
		CrashTime:     time.Now(),
		CrashCount:    1,
		LastStartTime: time.Now().Add(-1 * time.Minute),
	}

	// First attempt should succeed
	decision1 := engine.Decide(evidence)
	if decision1.Action != RepairActionRestart {
		t.Errorf("First attempt: expected restart, got %s", decision1.Action)
	}

	// Immediate second attempt should be blocked by cooldown
	decision2 := engine.Decide(evidence)
	if decision2.Action != RepairActionEscalate {
		t.Errorf("Immediate retry: expected escalate (cooldown), got %s", decision2.Action)
	}

	// Wait for cooldown to expire
	time.Sleep(2100 * time.Millisecond)

	// Now it should work again
	decision3 := engine.Decide(evidence)
	if decision3.Action != RepairActionRestart {
		t.Errorf("After cooldown: expected restart, got %s", decision3.Action)
	}
}

func TestTriageEngine_ResetAttempts(t *testing.T) {
	bounds := PolicyBounds{
		MaxRestartAttempts:  1,
		MaxRebindAttempts:   2,
		MaxRollbackAttempts: 1,
		RepairCooldownSecs:  1,
	}
	engine := NewTriageEngine(bounds)

	evidence := &CrashEvidence{
		AgentID:       "test-agent",
		ExitCode:      1,
		LogLines:      []string{"generic error"},
		CrashTime:     time.Now(),
		CrashCount:    1,
		LastStartTime: time.Now().Add(-1 * time.Minute),
	}

	// First attempt
	decision1 := engine.Decide(evidence)
	if decision1.Action != RepairActionRestart {
		t.Errorf("First attempt: expected restart, got %s", decision1.Action)
	}

	time.Sleep(1100 * time.Millisecond)

	// Second attempt should escalate (max=1)
	decision2 := engine.Decide(evidence)
	if decision2.Action != RepairActionEscalate {
		t.Errorf("Second attempt: expected escalate, got %s", decision2.Action)
	}

	// Reset attempts
	engine.ResetAttempts("test-agent")

	// Should work again now
	decision3 := engine.Decide(evidence)
	if decision3.Action != RepairActionRestart {
		t.Errorf("After reset: expected restart, got %s", decision3.Action)
	}
}

func TestTriageEngine_GetAttemptHistory(t *testing.T) {
	bounds := PolicyBounds{
		MaxRestartAttempts:  5,
		MaxRebindAttempts:   2,
		MaxRollbackAttempts: 1,
		RepairCooldownSecs:  1, // 1 second cooldown
	}
	engine := NewTriageEngine(bounds)

	// Initially empty
	history := engine.GetAttemptHistory("test-agent")
	if len(history) != 0 {
		t.Errorf("Expected empty history, got %d attempts", len(history))
	}

	// Make some decisions
	evidence := &CrashEvidence{
		AgentID:       "test-agent",
		ExitCode:      1,
		LogLines:      []string{"generic error"},
		CrashTime:     time.Now(),
		CrashCount:    1,
		LastStartTime: time.Now().Add(-1 * time.Minute),
	}

	engine.Decide(evidence)
	time.Sleep(1100 * time.Millisecond) // Wait for cooldown
	engine.Decide(evidence)

	// Check history
	history = engine.GetAttemptHistory("test-agent")
	if len(history) != 2 {
		t.Errorf("Expected 2 attempts in history, got %d", len(history))
	}

	for _, attempt := range history {
		if attempt.Action != RepairActionRestart {
			t.Errorf("Expected restart in history, got %s", attempt.Action)
		}
	}
}

func TestTriageEngine_AddCustomRule(t *testing.T) {
	engine := NewTriageEngine(DefaultPolicyBounds())

	// Add a custom high-priority rule
	customRule := TriageRule{
		Name:     "custom_test_rule",
		Priority: 5, // Higher priority than OOM rule (10)
		Evaluate: func(evidence *CrashEvidence) (*RepairDecision, bool) {
			if evidence.ExitCode == 42 {
				return &RepairDecision{
					Action: RepairActionNoOp,
					Reason: "custom rule matched",
				}, true
			}
			return nil, false
		},
	}

	engine.AddRule(customRule)

	// Test that custom rule fires
	evidence := &CrashEvidence{
		AgentID:       "test-agent",
		ExitCode:      42,
		LogLines:      []string{},
		CrashTime:     time.Now(),
		CrashCount:    1,
		LastStartTime: time.Now().Add(-1 * time.Minute),
	}

	decision := engine.Decide(evidence)

	if decision.Action != RepairActionNoOp {
		t.Errorf("Expected noop from custom rule, got %s", decision.Action)
	}

	if decision.Reason != "custom rule matched" {
		t.Errorf("Expected custom reason, got %s", decision.Reason)
	}
}

func TestTriageEngine_NoMatchingRule(t *testing.T) {
	// Create engine with no default rules
	engine := &TriageEngine{
		rules:        []TriageRule{},
		policyBounds: DefaultPolicyBounds(),
		attempts:     make(map[string][]repairAttempt),
	}

	evidence := &CrashEvidence{
		AgentID:       "test-agent",
		ExitCode:      99,
		LogLines:      []string{},
		CrashTime:     time.Now(),
		CrashCount:    1,
		LastStartTime: time.Now().Add(-1 * time.Minute),
	}

	decision := engine.Decide(evidence)

	if decision.Action != RepairActionNoOp {
		t.Errorf("Expected noop when no rules match, got %s", decision.Action)
	}
}

func TestTriageEngine_MultipleAgents(t *testing.T) {
	bounds := PolicyBounds{
		MaxRestartAttempts:  1,
		MaxRebindAttempts:   2,
		MaxRollbackAttempts: 1,
		RepairCooldownSecs:  1,
	}
	engine := NewTriageEngine(bounds)

	evidence1 := &CrashEvidence{
		AgentID:       "agent-1",
		ExitCode:      1,
		LogLines:      []string{"error"},
		CrashTime:     time.Now(),
		CrashCount:    1,
		LastStartTime: time.Now().Add(-1 * time.Minute),
	}

	evidence2 := &CrashEvidence{
		AgentID:       "agent-2",
		ExitCode:      1,
		LogLines:      []string{"error"},
		CrashTime:     time.Now(),
		CrashCount:    1,
		LastStartTime: time.Now().Add(-1 * time.Minute),
	}

	// Both agents should be able to restart once
	decision1 := engine.Decide(evidence1)
	if decision1.Action != RepairActionRestart {
		t.Errorf("Agent 1: expected restart, got %s", decision1.Action)
	}

	decision2 := engine.Decide(evidence2)
	if decision2.Action != RepairActionRestart {
		t.Errorf("Agent 2: expected restart, got %s", decision2.Action)
	}

	// Verify attempts are tracked separately
	history1 := engine.GetAttemptHistory("agent-1")
	history2 := engine.GetAttemptHistory("agent-2")

	if len(history1) != 1 || len(history2) != 1 {
		t.Errorf("Expected 1 attempt per agent, got %d and %d", len(history1), len(history2))
	}
}
