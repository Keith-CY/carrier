package lifecycle

import (
	"strings"
	"time"
)

// loadPersistedState restores agent state from the state file.
// Only restores Install and Runtime state for registered agents.
// Verifies that processes marked as "running" are actually alive.
func (s *Service) loadPersistedState() error {
	if s.stateFile == nil {
		return nil
	}

	persisted, err := s.stateFile.Load()
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Stash persisted state so RegisterManifest can apply it when agents
	// are registered (agents may not exist yet at this point).
	s.pendingPersisted = persisted

	for id, pState := range persisted {
		// Only restore state for agents that have been registered
		if state, ok := s.states[id]; ok {
			s.applyPersistedState(id, pState, &state)
			s.states[id] = state
		}
	}

	return nil
}

// applyPersistedState applies a single persisted record to the given AgentState
// and restores crash-loop cooldown/restart data on the service. Caller must hold s.mu.
func (s *Service) applyPersistedState(id string, pState *PersistedAgentState, state *AgentState) {
	if pState.Installed {
		state.Install = InstallStateInstalled
	} else {
		state.Install = InstallStateNotInstalled
	}

	restoredState := RuntimeState(pState.RuntimeState)
	if restoredState == RuntimeStateRunning {
		if !s.processManager.IsRunning(id) {
			restoredState = RuntimeStateStopped
		}
	}
	state.Runtime = restoredState
	state.Isolated = pState.Isolated
	state.LimaInstanceName = strings.TrimSpace(pState.LimaInstanceName)
	state.Memory = cloneMemoryState(pState.Memory)
	state.UpdatedAt = pState.LastTransition

	// Restore crash-loop cooldown state
	if len(pState.Restarts) > 0 {
		s.restarts[id] = make([]time.Time, len(pState.Restarts))
		copy(s.restarts[id], pState.Restarts)
	}
	if !pState.CooldownUntil.IsZero() {
		s.cooldowns[id] = pState.CooldownUntil
	}
}

// saveState persists the current agent states to disk, including crash-loop
// cooldown and restart timestamps so they survive daemon restarts.
func (s *Service) saveState() {
	if s.stateFile == nil {
		return
	}

	s.mu.RLock()
	persisted := make(map[string]PersistedAgentState, len(s.states))
	for id, state := range s.states {
		p := PersistedAgentState{
			ID:               state.ID,
			Installed:        state.Install == InstallStateInstalled,
			RuntimeState:     string(state.Runtime),
			Isolated:         state.Isolated,
			LimaInstanceName: strings.TrimSpace(state.LimaInstanceName),
			Memory:           cloneMemoryState(state.Memory),
			LastTransition:   state.UpdatedAt,
		}
		if restarts, ok := s.restarts[id]; ok && len(restarts) > 0 {
			p.Restarts = make([]time.Time, len(restarts))
			copy(p.Restarts, restarts)
		}
		if cooldownUntil, ok := s.cooldowns[id]; ok && !cooldownUntil.IsZero() {
			p.CooldownUntil = cooldownUntil
		}
		persisted[id] = p
	}
	s.mu.RUnlock()

	_ = s.stateFile.SavePersisted(persisted)
}
