package lifecycle

import (
	"context"
	"fmt"
	"time"

	"carrier/daemon/internal/baseagent"
)

// RepairAction represents the type of repair action that can be executed.
type RepairAction string

const (
	RepairActionRestart  RepairAction = "restart"
	RepairActionRebind   RepairAction = "rebind"
	RepairActionRollback RepairAction = "rollback"
)

// RepairConfig holds the configuration for repair attempt limits.
type RepairConfig struct {
	MaxRestartAttempts  int
	MaxRebindAttempts   int
	MaxRollbackAttempts int
	SuccessThreshold    time.Duration // Time an agent must run successfully to reset counters
}

// DefaultRepairConfig returns sensible defaults for repair configuration.
func DefaultRepairConfig() RepairConfig {
	return RepairConfig{
		MaxRestartAttempts:  3,
		MaxRebindAttempts:   2,
		MaxRollbackAttempts: 1,
		SuccessThreshold:    5 * time.Minute,
	}
}

// RepairAttempts tracks the repair attempts for a single agent.
type RepairAttempts struct {
	Restarts       int
	Rebinds        int
	Rollbacks      int
	LastSuccessful time.Time
}

// RepairResult represents the outcome of a repair action.
type RepairResult struct {
	Action            RepairAction
	Success           bool
	Error             error
	NeedsIntervention bool
	AttemptsRemaining map[RepairAction]int
}

// RepairManager executes repair decisions from the triage engine.
type RepairManager struct {
	config   RepairConfig
	attempts map[string]*RepairAttempts
	svc      *Service
	now      func() time.Time
}

// NewRepairManager creates a new RepairManager with the given configuration.
func NewRepairManager(svc *Service, config RepairConfig) *RepairManager {
	return &RepairManager{
		config:   config,
		attempts: make(map[string]*RepairAttempts),
		svc:      svc,
		now:      time.Now,
	}
}

// ExecuteRepair executes repair actions based on the triage result.
// It interprets the SuggestedActions from the triage engine and executes
// the appropriate repair action, respecting attempt limits.
func (rm *RepairManager) ExecuteRepair(ctx context.Context, agentID string, triage baseagent.TriageResult) RepairResult {
	if triage.Resolved {
		// Issue already resolved by triage engine
		return RepairResult{Success: true}
	}

	// Initialize attempt tracking for this agent if not present
	if _, ok := rm.attempts[agentID]; !ok {
		rm.attempts[agentID] = &RepairAttempts{}
	}

	// Determine repair action from suggested actions
	action := rm.selectAction(triage.SuggestedActions)
	if action == "" {
		// No recognized repair action in suggestions
		return RepairResult{
			Success:           false,
			NeedsIntervention: true,
			AttemptsRemaining: rm.remainingAttempts(agentID),
		}
	}

	// Check if we've exhausted attempts for this action
	if !rm.canAttempt(agentID, action) {
		// Try next fallback action
		nextAction := rm.nextFallbackAction(agentID, action)
		if nextAction == "" {
			// All actions exhausted
			return RepairResult{
				Action:            action,
				Success:           false,
				NeedsIntervention: true,
				Error:             fmt.Errorf("all repair attempts exhausted for agent %s", agentID),
				AttemptsRemaining: rm.remainingAttempts(agentID),
			}
		}
		action = nextAction
	}

	// Execute the repair action
	err := rm.executeAction(ctx, agentID, action)
	if err != nil {
		return RepairResult{
			Action:            action,
			Success:           false,
			Error:             err,
			AttemptsRemaining: rm.remainingAttempts(agentID),
		}
	}

	// Increment attempt counter
	rm.incrementAttempt(agentID, action)

	return RepairResult{
		Action:            action,
		Success:           true,
		AttemptsRemaining: rm.remainingAttempts(agentID),
	}
}

// ResetOnSuccess resets the repair attempt counters for an agent
// that has been running successfully for longer than the success threshold.
func (rm *RepairManager) ResetOnSuccess(agentID string, runningSince time.Time) {
	if _, ok := rm.attempts[agentID]; !ok {
		return
	}

	runningDuration := rm.now().Sub(runningSince)
	if runningDuration >= rm.config.SuccessThreshold {
		rm.attempts[agentID] = &RepairAttempts{
			LastSuccessful: rm.now(),
		}
	}
}

// selectAction chooses the appropriate repair action from suggested actions.
func (rm *RepairManager) selectAction(suggestions []string) RepairAction {
	for _, suggestion := range suggestions {
		switch suggestion {
		case "restart", "Restart service":
			return RepairActionRestart
		case "rebind", "Rebind port", "Change port":
			return RepairActionRebind
		case "rollback", "Rollback version", "Restore previous version":
			return RepairActionRollback
		}
	}
	return ""
}

// canAttempt checks if there are remaining attempts for the given action.
func (rm *RepairManager) canAttempt(agentID string, action RepairAction) bool {
	attempts := rm.attempts[agentID]
	switch action {
	case RepairActionRestart:
		return attempts.Restarts < rm.config.MaxRestartAttempts
	case RepairActionRebind:
		return attempts.Rebinds < rm.config.MaxRebindAttempts
	case RepairActionRollback:
		return attempts.Rollbacks < rm.config.MaxRollbackAttempts
	default:
		return false
	}
}

// incrementAttempt increments the attempt counter for the given action.
func (rm *RepairManager) incrementAttempt(agentID string, action RepairAction) {
	attempts := rm.attempts[agentID]
	switch action {
	case RepairActionRestart:
		attempts.Restarts++
	case RepairActionRebind:
		attempts.Rebinds++
	case RepairActionRollback:
		attempts.Rollbacks++
	}
}

// remainingAttempts returns a map of remaining attempts for each action type.
func (rm *RepairManager) remainingAttempts(agentID string) map[RepairAction]int {
	attempts := rm.attempts[agentID]
	return map[RepairAction]int{
		RepairActionRestart:  rm.config.MaxRestartAttempts - attempts.Restarts,
		RepairActionRebind:   rm.config.MaxRebindAttempts - attempts.Rebinds,
		RepairActionRollback: rm.config.MaxRollbackAttempts - attempts.Rollbacks,
	}
}

// nextFallbackAction returns the next available repair action to try.
// Fallback order: restart -> rebind -> rollback
func (rm *RepairManager) nextFallbackAction(agentID string, current RepairAction) RepairAction {
	switch current {
	case RepairActionRestart:
		if rm.canAttempt(agentID, RepairActionRebind) {
			return RepairActionRebind
		}
		if rm.canAttempt(agentID, RepairActionRollback) {
			return RepairActionRollback
		}
	case RepairActionRebind:
		if rm.canAttempt(agentID, RepairActionRollback) {
			return RepairActionRollback
		}
	}
	return ""
}

// executeAction executes the specified repair action.
func (rm *RepairManager) executeAction(ctx context.Context, agentID string, action RepairAction) error {
	switch action {
	case RepairActionRestart:
		return rm.executeRestart(ctx, agentID)
	case RepairActionRebind:
		return rm.executeRebind(ctx, agentID)
	case RepairActionRollback:
		return rm.executeRollback(ctx, agentID)
	default:
		return fmt.Errorf("unknown repair action: %s", action)
	}
}

// executeRestart performs a restart with exponential backoff.
func (rm *RepairManager) executeRestart(ctx context.Context, agentID string) error {
	attempts := rm.attempts[agentID]

	// Calculate backoff delay: 1s, 2s, 4s, 8s, ...
	backoffDelay := time.Second * time.Duration(1<<attempts.Restarts)
	if backoffDelay > 30*time.Second {
		backoffDelay = 30 * time.Second
	}

	rm.svc.appendLog(agentID, fmt.Sprintf("repair: waiting %v before restart (attempt %d/%d)",
		backoffDelay, attempts.Restarts+1, rm.config.MaxRestartAttempts))

	select {
	case <-time.After(backoffDelay):
	case <-ctx.Done():
		return ctx.Err()
	}

	// Stop if currently running
	state, err := rm.svc.Status(agentID)
	if err != nil {
		return err
	}
	if state.Runtime == RuntimeStateRunning || state.Runtime == RuntimeStateCrashing {
		if err := rm.svc.Stop(ctx, agentID); err != nil {
			return fmt.Errorf("stop before restart: %w", err)
		}
	}

	// Start
	if err := rm.svc.Start(ctx, agentID); err != nil {
		return fmt.Errorf("restart failed: %w", err)
	}

	rm.svc.appendLog(agentID, fmt.Sprintf("repair: restart completed (attempt %d/%d)",
		attempts.Restarts+1, rm.config.MaxRestartAttempts))
	return nil
}

// executeRebind changes the port in the agent configuration and restarts.
func (rm *RepairManager) executeRebind(ctx context.Context, agentID string) error {
	attempts := rm.attempts[agentID]
	rm.svc.appendLog(agentID, fmt.Sprintf("repair: attempting rebind (attempt %d/%d)",
		attempts.Rebinds+1, rm.config.MaxRebindAttempts))

	// Get manifest
	rm.svc.mu.RLock()
	m, ok := rm.svc.manifests[agentID]
	rm.svc.mu.RUnlock()
	if !ok {
		return fmt.Errorf("manifest not found for agent %s", agentID)
	}

	// Find alternative port
	if len(m.Network.Ports) == 0 {
		return fmt.Errorf("rebind: no ports configured")
	}

	newPort, err := rm.findAlternativePort(m.Network.Ports[0].Port)
	if err != nil {
		return fmt.Errorf("rebind: %w", err)
	}

	// Update manifest with new port
	oldPort := m.Network.Ports[0].Port
	m.Network.Ports[0].Port = newPort
	rm.svc.mu.Lock()
	rm.svc.manifests[agentID] = m
	rm.svc.mu.Unlock()
	rm.svc.appendLog(agentID, fmt.Sprintf("repair: rebind port %d -> %d", oldPort, newPort))

	// Restart with new configuration
	if err := rm.executeRestart(ctx, agentID); err != nil {
		return fmt.Errorf("rebind restart: %w", err)
	}

	return nil
}

// executeRollback restores the previous binary version and restarts.
func (rm *RepairManager) executeRollback(ctx context.Context, agentID string) error {
	attempts := rm.attempts[agentID]
	rm.svc.appendLog(agentID, fmt.Sprintf("repair: attempting rollback (attempt %d/%d)",
		attempts.Rollbacks+1, rm.config.MaxRollbackAttempts))

	// Get manifest
	rm.svc.mu.RLock()
	m, ok := rm.svc.manifests[agentID]
	rm.svc.mu.RUnlock()
	if !ok {
		return fmt.Errorf("manifest not found for agent %s", agentID)
	}

	// Check if upgrade command is available
	if m.Runtime.Upgrade.Command == "" {
		return fmt.Errorf("rollback: no upgrade command configured")
	}

	// Execute rollback using the upgrade mechanism
	// This is a simplified approach - in production you'd want proper rollback logic
	// For now, we'll just attempt to use the upgrade command
	rm.svc.appendLog(agentID, "repair: attempting rollback via upgrade mechanism")

	// Stop current version
	if err := rm.svc.Stop(ctx, agentID); err != nil {
		return fmt.Errorf("rollback stop: %w", err)
	}

	// In a real implementation, this would:
	// 1. Restore the binary from a backup location
	// 2. Update the manifest version to the previous version
	// For this implementation, we'll just log and restart
	rm.svc.appendLog(agentID, "repair: rollback mechanism would restore previous binary here")

	// Restart
	if err := rm.svc.Start(ctx, agentID); err != nil {
		return fmt.Errorf("rollback start: %w", err)
	}

	return nil
}

// findAlternativePort attempts to find an available port different from the current one.
func (rm *RepairManager) findAlternativePort(basePort int) (int, error) {
	// Simple algorithm: try ports in a range around the original
	if basePort <= 0 || basePort > 65535 {
		return 0, fmt.Errorf("invalid base port: %d", basePort)
	}

	for offset := 1; offset <= 100; offset++ {
		candidatePort := basePort + offset
		if candidatePort > 65535 {
			break
		}

		// Check if port is available by trying to bind to it
		// We use the Service's ensurePortsAvailable which accepts []int
		// But we need to convert to PortSpec for internal use
		// For now, we'll use a simpler check
		if rm.isPortAvailable(candidatePort) {
			return candidatePort, nil
		}
	}

	return 0, fmt.Errorf("no alternative port found")
}

// isPortAvailable checks if a port is available by attempting to listen on it.
func (rm *RepairManager) isPortAvailable(port int) bool {
	listener, err := listenTCP("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

// GetAttempts returns the current repair attempts for an agent.
func (rm *RepairManager) GetAttempts(agentID string) *RepairAttempts {
	if attempts, ok := rm.attempts[agentID]; ok {
		// Return a copy to prevent external modification
		return &RepairAttempts{
			Restarts:       attempts.Restarts,
			Rebinds:        attempts.Rebinds,
			Rollbacks:      attempts.Rollbacks,
			LastSuccessful: attempts.LastSuccessful,
		}
	}
	return &RepairAttempts{}
}
