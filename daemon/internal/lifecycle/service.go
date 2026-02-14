package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"carrier/daemon/internal/baseagent"
	"carrier/daemon/internal/manifest"
)

var (
	ErrAgentNotFound   = errors.New("agent not found")
	ErrNotInstalled    = errors.New("agent is not installed")
	ErrAlreadyRunning  = errors.New("agent already running")
	ErrAlreadyStopped  = errors.New("agent already stopped")
)

type Service struct {
	mu        sync.RWMutex
	states    map[string]AgentState
	manifests map[string]manifest.Manifest
	triager   baseagent.Triager
}

func NewService(triager baseagent.Triager) *Service {
	if triager == nil {
		triager = baseagent.NoopTriager{}
	}

	return &Service{
		states:    make(map[string]AgentState),
		manifests: make(map[string]manifest.Manifest),
		triager:   triager,
	}
}

func (s *Service) RegisterManifest(m manifest.Manifest) error {
	if err := m.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.manifests[m.ID] = m
	s.states[m.ID] = AgentState{
		ID:        m.ID,
		Version:   m.Version,
		Install:   InstallStateNotInstalled,
		Runtime:   RuntimeStateStopped,
		Health:    HealthStateUnknown,
		UpdatedAt: time.Now(),
	}

	return nil
}

func (s *Service) ListAgents() []AgentState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]string, 0, len(s.states))
	for id := range s.states {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := make([]AgentState, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.states[id])
	}

	return out
}

func (s *Service) Install(agentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.states[agentID]
	if !ok {
		return ErrAgentNotFound
	}

	state.Install = InstallStateInstalled
	state.Runtime = RuntimeStateStopped
	state.Health = HealthStateUnknown
	state.LastError = ""
	state.UpdatedAt = time.Now()
	s.states[agentID] = state

	return nil
}

func (s *Service) Start(agentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.states[agentID]
	if !ok {
		return ErrAgentNotFound
	}
	if state.Install != InstallStateInstalled {
		return ErrNotInstalled
	}
	if state.Runtime == RuntimeStateRunning {
		return ErrAlreadyRunning
	}

	state.Runtime = RuntimeStateRunning
	state.Health = HealthStateHealthy
	state.LastError = ""
	state.UpdatedAt = time.Now()
	s.states[agentID] = state

	return nil
}

func (s *Service) Stop(agentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.states[agentID]
	if !ok {
		return ErrAgentNotFound
	}
	if state.Runtime == RuntimeStateStopped {
		return ErrAlreadyStopped
	}

	state.Runtime = RuntimeStateStopped
	state.Health = HealthStateUnknown
	state.UpdatedAt = time.Now()
	s.states[agentID] = state

	return nil
}

func (s *Service) Status(agentID string) (AgentState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, ok := s.states[agentID]
	if !ok {
		return AgentState{}, ErrAgentNotFound
	}

	return state, nil
}

func (s *Service) Diagnose(agentID string) (string, error) {
	s.mu.RLock()
	_, ok := s.states[agentID]
	s.mu.RUnlock()
	if !ok {
		return "", ErrAgentNotFound
	}

	now := time.Now().UTC().Format("2006-01-02T15-04-05Z")
	return fmt.Sprintf("%s-diagnose-%s.zip", agentID, now), nil
}

func (s *Service) HandleFailure(ctx context.Context, agentID, lastError string) (baseagent.TriageResult, error) {
	s.mu.Lock()
	state, ok := s.states[agentID]
	if !ok {
		s.mu.Unlock()
		return baseagent.TriageResult{}, ErrAgentNotFound
	}

	state.Runtime = RuntimeStateCrashing
	state.Health = HealthStateUnhealthy
	state.LastError = lastError
	state.UpdatedAt = time.Now()
	s.states[agentID] = state
	s.mu.Unlock()

	return s.triager.Analyze(ctx, baseagent.Evidence{
		AgentID:   agentID,
		LastError: lastError,
	})
}
