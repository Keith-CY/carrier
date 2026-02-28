package lifecycle

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
)

// instanceNamePattern allows alphanumeric characters, hyphens, underscores, and dots.
var instanceNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// RegisterInstance creates a new instance entry based on an existing agent manifest.
// The instance gets its own independent lifecycle state and can be managed via instanceID.
func (s *Service) RegisterInstance(baseAgentID, instanceName string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.manifests[baseAgentID]
	if !ok {
		return "", ErrAgentNotFound
	}

	if instanceName == "" {
		instanceName = generateInstanceName(baseAgentID)
	} else if !instanceNamePattern.MatchString(instanceName) {
		return "", fmt.Errorf("invalid instance name %q: must match [a-zA-Z0-9._-]", instanceName)
	}

	// Check for duplicate instance name
	if _, exists := s.states[instanceName]; exists {
		return "", fmt.Errorf("instance %q already exists", instanceName)
	}

	// Create a manifest copy for the instance
	instanceManifest := m
	instanceManifest.ID = instanceName
	instanceManifest.Name = fmt.Sprintf("%s (%s)", m.Name, instanceName)

	s.manifests[instanceName] = instanceManifest
	s.states[instanceName] = AgentState{
		ID:        instanceName,
		Name:      instanceManifest.Name,
		Version:   m.Version,
		Install:   InstallStateNotInstalled,
		Runtime:   RuntimeStateStopped,
		Health:    HealthStateUnknown,
		Ports:     []int{},
		UpdatedAt: s.now(),
	}
	s.logs[instanceName] = nil
	s.restarts[instanceName] = nil
	s.cooldowns[instanceName] = s.now().Add(-1) // no cooldown
	s.backoffStates[instanceName] = BackoffState{}

	return instanceName, nil
}

// generateInstanceName creates a name like "openclaw-a3f2".
func generateInstanceName(baseID string) string {
	buf := make([]byte, 2)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return fmt.Sprintf("%s-%s", baseID, hex.EncodeToString(buf))
}

// UnregisterInstance removes an instance from the service state.
// The instance must be stopped and not installed.
func (s *Service) UnregisterInstance(instanceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.states[instanceID]
	if !ok {
		return ErrAgentNotFound
	}

	if state.Runtime == RuntimeStateRunning {
		return ErrAlreadyRunning
	}

	delete(s.states, instanceID)
	delete(s.manifests, instanceID)
	delete(s.logs, instanceID)
	delete(s.restarts, instanceID)
	delete(s.cooldowns, instanceID)
	delete(s.backoffStates, instanceID)

	return nil
}
