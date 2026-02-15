package lifecycle

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// RepairAction represents the repair decision made by the triage engine.
type RepairAction string

const (
	RepairActionRestart  RepairAction = "restart"  // Restart the service
	RepairActionRebind   RepairAction = "rebind"   // Change port/address binding
	RepairActionRollback RepairAction = "rollback" // Rollback to previous version
	RepairActionEscalate RepairAction = "escalate" // Needs human intervention
	RepairActionNoOp     RepairAction = "noop"     // Do nothing
)

// CrashEvidence contains information about a service crash.
type CrashEvidence struct {
	AgentID       string
	ExitCode      int
	LogLines      []string // Recent log output
	CrashTime     time.Time
	CrashCount    int       // Number of crashes in recent window
	LastStartTime time.Time // When the service was last started
}

// RepairDecision is the output of the triage engine.
type RepairDecision struct {
	Action          RepairAction
	Reason          string
	SuggestedParams map[string]string // e.g., {"port": "8081"}
}

// TriageRule defines a single triage rule.
type TriageRule struct {
	Name     string
	Priority int                                                   // Lower number = higher priority
	Evaluate func(evidence *CrashEvidence) (*RepairDecision, bool) // Returns (decision, matched)
}

// PolicyBounds defines limits on repair attempts.
type PolicyBounds struct {
	MaxRestartAttempts  int
	MaxRebindAttempts   int
	MaxRollbackAttempts int
	RepairCooldownSecs  int // Minimum seconds between repair attempts of same type
}

// DefaultPolicyBounds returns sensible default policy bounds.
func DefaultPolicyBounds() PolicyBounds {
	return PolicyBounds{
		MaxRestartAttempts:  3,
		MaxRebindAttempts:   2,
		MaxRollbackAttempts: 1,
		RepairCooldownSecs:  60,
	}
}

// repairAttempt tracks a single repair attempt.
type repairAttempt struct {
	Action    RepairAction
	Timestamp time.Time
}

// TriageEngine evaluates crash evidence and decides on repair actions.
type TriageEngine struct {
	mu           sync.RWMutex
	rules        []TriageRule
	policyBounds PolicyBounds
	// Track repair attempts per agent
	attempts map[string][]repairAttempt
}

// NewTriageEngine creates a new triage engine with default rules and policy bounds.
func NewTriageEngine(policyBounds PolicyBounds) *TriageEngine {
	engine := &TriageEngine{
		rules:        []TriageRule{},
		policyBounds: policyBounds,
		attempts:     make(map[string][]repairAttempt),
	}

	// Add default rules (in priority order)
	engine.AddRule(ruleOOMKilled())
	engine.AddRule(rulePortConflict())
	engine.AddRule(ruleMissingDependency())
	engine.AddRule(ruleGenericCrash())

	return engine
}

// AddRule adds a triage rule to the engine.
// Rules are evaluated in priority order (lower number first).
func (te *TriageEngine) AddRule(rule TriageRule) {
	te.mu.Lock()
	defer te.mu.Unlock()

	// Insert in priority order
	inserted := false
	for i, existingRule := range te.rules {
		if rule.Priority < existingRule.Priority {
			// Insert before this rule
			te.rules = append(te.rules[:i], append([]TriageRule{rule}, te.rules[i:]...)...)
			inserted = true
			break
		}
	}

	if !inserted {
		te.rules = append(te.rules, rule)
	}
}

// Decide evaluates the crash evidence and returns a repair decision.
// First matching rule wins (based on priority order).
func (te *TriageEngine) Decide(evidence *CrashEvidence) RepairDecision {
	te.mu.RLock()
	rules := te.rules
	te.mu.RUnlock()

	// Evaluate rules in priority order
	for _, rule := range rules {
		decision, matched := rule.Evaluate(evidence)
		if matched && decision != nil {
			// Check policy bounds before returning decision
			if !te.isActionAllowed(evidence.AgentID, decision.Action) {
				// Exceeded policy bounds - escalate instead
				return RepairDecision{
					Action: RepairActionEscalate,
					Reason: fmt.Sprintf("policy bounds exceeded for action %s (matched rule: %s)", decision.Action, rule.Name),
				}
			}

			// Record the attempt
			te.recordAttempt(evidence.AgentID, decision.Action)

			return *decision
		}
	}

	// No rule matched - default to no-op
	return RepairDecision{
		Action: RepairActionNoOp,
		Reason: "no triage rule matched",
	}
}

// isActionAllowed checks if the repair action is allowed under policy bounds.
func (te *TriageEngine) isActionAllowed(agentID string, action RepairAction) bool {
	te.mu.RLock()
	defer te.mu.RUnlock()

	attempts, exists := te.attempts[agentID]
	if !exists {
		return true // No previous attempts, allow
	}

	now := time.Now()
	cooldown := time.Duration(te.policyBounds.RepairCooldownSecs) * time.Second

	// Count recent attempts of this action type
	recentAttempts := 0
	for _, attempt := range attempts {
		if attempt.Action == action && now.Sub(attempt.Timestamp) < 24*time.Hour {
			recentAttempts++

			// Check cooldown
			if now.Sub(attempt.Timestamp) < cooldown {
				return false // Still in cooldown period
			}
		}
	}

	// Check max attempts
	var maxAttempts int
	switch action {
	case RepairActionRestart:
		maxAttempts = te.policyBounds.MaxRestartAttempts
	case RepairActionRebind:
		maxAttempts = te.policyBounds.MaxRebindAttempts
	case RepairActionRollback:
		maxAttempts = te.policyBounds.MaxRollbackAttempts
	default:
		return true // No limits on escalate or noop
	}

	return recentAttempts < maxAttempts
}

// recordAttempt records a repair attempt for policy tracking.
func (te *TriageEngine) recordAttempt(agentID string, action RepairAction) {
	te.mu.Lock()
	defer te.mu.Unlock()

	te.attempts[agentID] = append(te.attempts[agentID], repairAttempt{
		Action:    action,
		Timestamp: time.Now(),
	})
}

// ResetAttempts clears the repair attempt history for an agent.
// Useful when an agent has been manually fixed or upgraded.
func (te *TriageEngine) ResetAttempts(agentID string) {
	te.mu.Lock()
	defer te.mu.Unlock()

	delete(te.attempts, agentID)
}

// GetAttemptHistory returns the repair attempt history for an agent.
func (te *TriageEngine) GetAttemptHistory(agentID string) []repairAttempt {
	te.mu.RLock()
	defer te.mu.RUnlock()

	attempts, exists := te.attempts[agentID]
	if !exists {
		return []repairAttempt{}
	}

	// Return a copy to prevent mutation
	result := make([]repairAttempt, len(attempts))
	copy(result, attempts)
	return result
}

// --- Default Triage Rules ---

// ruleOOMKilled detects OOM (Out of Memory) kills.
// Exit code 137 = SIGKILL (128 + 9), often used by OOM killer.
func ruleOOMKilled() TriageRule {
	return TriageRule{
		Name:     "oom_killed",
		Priority: 10,
		Evaluate: func(evidence *CrashEvidence) (*RepairDecision, bool) {
			if evidence.ExitCode == 137 {
				// Exit 137 is SIGKILL (128+9), often from OOM killer
				return &RepairDecision{
					Action: RepairActionEscalate,
					Reason: "process killed by OOM (exit 137), needs resource adjustment",
				}, true
			}
			return nil, false
		},
	}
}

// rulePortConflict detects port/address binding conflicts.
func rulePortConflict() TriageRule {
	return TriageRule{
		Name:     "port_conflict",
		Priority: 20,
		Evaluate: func(evidence *CrashEvidence) (*RepairDecision, bool) {
			if evidence.ExitCode != 1 {
				return nil, false
			}

			// Check logs for port conflict indicators
			for _, line := range evidence.LogLines {
				lowerLine := strings.ToLower(line)
				if strings.Contains(lowerLine, "address already in use") ||
					strings.Contains(lowerLine, "bind: address already in use") ||
					strings.Contains(lowerLine, "port already in use") {
					return &RepairDecision{
						Action: RepairActionRebind,
						Reason: "detected port conflict in logs",
						SuggestedParams: map[string]string{
							"strategy": "increment", // Suggest incrementing port number
						},
					}, true
				}
			}

			return nil, false
		},
	}
}

// ruleMissingDependency detects missing dependencies or broken installations.
func ruleMissingDependency() TriageRule {
	return TriageRule{
		Name:     "missing_dependency",
		Priority: 30,
		Evaluate: func(evidence *CrashEvidence) (*RepairDecision, bool) {
			if evidence.ExitCode != 1 && evidence.ExitCode != 127 {
				return nil, false
			}

			// Check logs for dependency errors
			for _, line := range evidence.LogLines {
				lowerLine := strings.ToLower(line)
				if strings.Contains(lowerLine, "no such file") ||
					strings.Contains(lowerLine, "cannot find") ||
					strings.Contains(lowerLine, "not found") ||
					strings.Contains(lowerLine, "missing") ||
					strings.Contains(lowerLine, "failed to load") {
					return &RepairDecision{
						Action: RepairActionRollback,
						Reason: "detected missing dependency or broken installation",
					}, true
				}
			}

			return nil, false
		},
	}
}

// ruleGenericCrash handles generic crashes with backoff logic.
func ruleGenericCrash() TriageRule {
	return TriageRule{
		Name:     "generic_crash",
		Priority: 100, // Lowest priority - catch-all
		Evaluate: func(evidence *CrashEvidence) (*RepairDecision, bool) {
			// Generic crash - restart with exponential backoff
			// Check crash frequency
			uptime := evidence.CrashTime.Sub(evidence.LastStartTime)

			if uptime < 10*time.Second {
				// Crashed very quickly - likely a persistent issue
				if evidence.CrashCount >= 3 {
					return &RepairDecision{
						Action: RepairActionEscalate,
						Reason: "crash loop detected (3+ crashes with <10s uptime)",
					}, true
				}
			}

			return &RepairDecision{
				Action: RepairActionRestart,
				Reason: "generic crash, attempting restart",
				SuggestedParams: map[string]string{
					"backoff": "exponential", // Suggest exponential backoff
				},
			}, true
		},
	}
}
