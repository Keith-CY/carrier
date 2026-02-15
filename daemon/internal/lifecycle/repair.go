package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	// ErrAllAttemptsExhausted is returned when all repair attempts for an action have been exhausted
	ErrAllAttemptsExhausted = errors.New("all repair attempts exhausted")
	// ErrNoRepairNeeded is returned when the agent is healthy and no repair is needed
	ErrNoRepairNeeded = errors.New("no repair needed")
)

// RepairOutcome represents a single repair decision with context.
type RepairOutcome struct {
	Type      RepairAction
	AgentID   string
	Reason    string
	Timestamp time.Time
	// SuggestedPort is populated for Rebind actions
	SuggestedPort int
	// BackupPath is populated for Rollback actions
	BackupPath string
}

// RepairConfig holds configuration for repair attempt limits.
type RepairConfig struct {
	// MaxRestartAttempts is the maximum number of restart attempts before escalation
	MaxRestartAttempts int
	// MaxRebindAttempts is the maximum number of rebind attempts before escalation
	MaxRebindAttempts int
	// MaxRollbackAttempts is the maximum number of rollback attempts before escalation
	MaxRollbackAttempts int
}

// DefaultRepairConfig returns the default repair configuration.
func DefaultRepairConfig() RepairConfig {
	return RepairConfig{
		MaxRestartAttempts:  5,
		MaxRebindAttempts:   2,
		MaxRollbackAttempts: 1,
	}
}

// RepairManager tracks repair attempts and executes bounded repair actions.
type RepairManager struct {
	mu      sync.RWMutex
	config  RepairConfig
	service *Service
	// attempts tracks the number of attempts per agent per action type
	// attempts[agentID][actionType] = count
	attempts map[string]map[RepairAction]int
	// lastSuccess tracks the last successful run timestamp per agent
	lastSuccess map[string]time.Time
}

// NewRepairManager creates a new RepairManager with the given configuration.
func NewRepairManager(service *Service, config RepairConfig) *RepairManager {
	return &RepairManager{
		config:      config,
		service:     service,
		attempts:    make(map[string]map[RepairAction]int),
		lastSuccess: make(map[string]time.Time),
	}
}

// NewRepairManagerWithDefaults creates a new RepairManager with default configuration.
func NewRepairManagerWithDefaults(service *Service) *RepairManager {
	return NewRepairManager(service, DefaultRepairConfig())
}

// RecordAttempt increments the attempt counter for a given agent and action type.
func (rm *RepairManager) RecordAttempt(agentID string, actionType RepairAction) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.attempts[agentID] == nil {
		rm.attempts[agentID] = make(map[RepairAction]int)
	}
	rm.attempts[agentID][actionType]++
}

// GetAttemptCount returns the current attempt count for a given agent and action type.
func (rm *RepairManager) GetAttemptCount(agentID string, actionType RepairAction) int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if rm.attempts[agentID] == nil {
		return 0
	}
	return rm.attempts[agentID][actionType]
}

// CanAttempt checks if another attempt is allowed for the given action type.
func (rm *RepairManager) CanAttempt(agentID string, actionType RepairAction) bool {
	count := rm.GetAttemptCount(agentID, actionType)
	switch actionType {
	case RepairActionRestart:
		return count < rm.config.MaxRestartAttempts
	case RepairActionRebind:
		return count < rm.config.MaxRebindAttempts
	case RepairActionRollback:
		return count < rm.config.MaxRollbackAttempts
	default:
		return false
	}
}

// RecordSuccess marks a successful run for an agent, resetting all attempt counters.
func (rm *RepairManager) RecordSuccess(agentID string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Reset all attempt counters for this agent
	delete(rm.attempts, agentID)
	rm.lastSuccess[agentID] = time.Now()
}

// GetLastSuccess returns the timestamp of the last successful run, or zero time if never succeeded.
func (rm *RepairManager) GetLastSuccess(agentID string) time.Time {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return rm.lastSuccess[agentID]
}

// DecideAction determines the next repair action based on the agent state and attempt history.
func (rm *RepairManager) DecideAction(agentID string, state AgentState) RepairOutcome {
	now := time.Now()

	// If agent is healthy, no repair needed
	if state.Runtime == RuntimeStateRunning && state.Health == HealthStateHealthy {
		return RepairOutcome{
			Type:      RepairActionEscalate,
			AgentID:   agentID,
			Reason:    "agent is healthy, no repair needed",
			Timestamp: now,
		}
	}

	// Try restart first if available
	if state.Runtime == RuntimeStateCrashing || state.Runtime == RuntimeStateStopped {
		if rm.CanAttempt(agentID, RepairActionRestart) {
			return RepairOutcome{
				Type:      RepairActionRestart,
				AgentID:   agentID,
				Reason:    fmt.Sprintf("agent in %s state", state.Runtime),
				Timestamp: now,
			}
		}
	}

	// If restart attempts exhausted and there's a port conflict hint, try rebind
	if state.LastError != "" && containsPortConflict(state.LastError) {
		if rm.CanAttempt(agentID, RepairActionRebind) {
			return RepairOutcome{
				Type:          RepairActionRebind,
				AgentID:       agentID,
				Reason:        "port conflict detected",
				Timestamp:     now,
				SuggestedPort: 0, // Service layer should determine available port
			}
		}
	}

	// If other attempts exhausted, try rollback if version info available
	if state.Version != "" && rm.CanAttempt(agentID, RepairActionRollback) {
		return RepairOutcome{
			Type:       RepairActionRollback,
			AgentID:    agentID,
			Reason:     "restart and rebind attempts exhausted",
			Timestamp:  now,
			BackupPath: "", // Service layer should determine backup path
		}
	}

	// All attempts exhausted - escalate
	return RepairOutcome{
		Type:      RepairActionEscalate,
		AgentID:   agentID,
		Reason:    "all repair attempts exhausted",
		Timestamp: now,
	}
}

// ExecuteRepair executes the given repair action and records the attempt.
func (rm *RepairManager) ExecuteRepair(action RepairOutcome) error {
	if action.Type == RepairActionEscalate {
		return ErrAllAttemptsExhausted
	}

	// Record the attempt before execution
	rm.RecordAttempt(action.AgentID, action.Type)

	switch action.Type {
	case RepairActionRestart:
		return rm.executeRestart(action)
	case RepairActionRebind:
		return rm.executeRebind(action)
	case RepairActionRollback:
		return rm.executeRollback(action)
	default:
		return fmt.Errorf("unknown repair action type: %s", action.Type)
	}
}

// executeRestart stops and starts the agent.
func (rm *RepairManager) executeRestart(action RepairOutcome) error {
	ctx := context.Background()

	// Stop the agent (ignore error if already stopped)
	_ = rm.service.Stop(ctx, action.AgentID)

	// Start the agent
	if err := rm.service.Start(ctx, action.AgentID); err != nil {
		return fmt.Errorf("restart failed: %w", err)
	}

	return nil
}

// executeRebind suggests a new port for the agent.
// This is a placeholder - actual implementation would need port allocation logic.
func (rm *RepairManager) executeRebind(action RepairOutcome) error {
	// In a real implementation, this would:
	// 1. Find an available port
	// 2. Update the agent's configuration
	// 3. Restart the agent with the new port
	//
	// For now, we return an error indicating this needs to be implemented
	// at the service layer with actual port allocation logic.
	return fmt.Errorf("rebind action requires service-layer port allocation (suggested port: %d)", action.SuggestedPort)
}

// executeRollback restores the agent to a previous version.
func (rm *RepairManager) executeRollback(action RepairOutcome) error {
	// In a real implementation, this would:
	// 1. Stop the agent
	// 2. Restore files from backup
	// 3. Start the agent with the previous version
	//
	// The Service already has upgrade functionality with backups,
	// so this would leverage that infrastructure.
	if action.BackupPath == "" {
		return fmt.Errorf("rollback requires a valid backup path")
	}

	ctx := context.Background()

	// Stop the agent first
	if err := rm.service.Stop(ctx, action.AgentID); err != nil && err != ErrAlreadyStopped {
		return fmt.Errorf("rollback stop failed: %w", err)
	}

	// The actual rollback logic would go here
	// For now, return an error indicating it needs service-layer support
	return fmt.Errorf("rollback action requires service-layer backup restoration from: %s", action.BackupPath)
}

// RepairLoop performs a complete repair cycle: decide, execute, and handle the result.
func (rm *RepairManager) RepairLoop(agentID string) error {
	// Get current state
	state, err := rm.service.Status(agentID)
	if err != nil {
		return fmt.Errorf("failed to get agent state: %w", err)
	}

	// Decide action
	action := rm.DecideAction(agentID, state)

	// If escalation needed, return error
	if action.Type == RepairActionEscalate {
		if action.Reason == "agent is healthy, no repair needed" {
			return ErrNoRepairNeeded
		}
		return ErrAllAttemptsExhausted
	}

	// Execute repair
	if err := rm.ExecuteRepair(action); err != nil {
		return fmt.Errorf("repair execution failed: %w", err)
	}

	return nil
}

// containsPortConflict checks if an error message indicates a port conflict.
func containsPortConflict(errMsg string) bool {
	keywords := []string{
		"port conflict",
		"address already in use",
		"bind: address already in use",
		"port is already allocated",
	}
	for _, keyword := range keywords {
		if contains(errMsg, keyword) {
			return true
		}
	}
	return false
}

// contains performs a case-insensitive substring check.
func contains(s, substr string) bool {
	// Simple case-insensitive check without importing strings to lowercase
	// For production, you'd want to use strings.Contains(strings.ToLower(s), strings.ToLower(substr))
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			c1 := s[i+j]
			c2 := substr[j]
			// Case-insensitive comparison
			if c1 >= 'A' && c1 <= 'Z' {
				c1 = c1 + ('a' - 'A')
			}
			if c2 >= 'A' && c2 <= 'Z' {
				c2 = c2 + ('a' - 'A')
			}
			if c1 != c2 {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
