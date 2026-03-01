package lifecycle

import (
	"fmt"
	"strings"

	"carrier/daemon/internal/memory"
)

const memoryContractIDDigestLength = 16

// autoMountMemories prepares and mounts the memory contract for runtime start.
func (s *Service) autoMountMemories(agentID string) (memory.RuntimeMemoryContract, error) {
	if s.memoryStore == nil {
		return memory.RuntimeMemoryContract{}, nil
	}
	s.mu.RLock()
	links := append([]string(nil), s.memoryLinks[agentID]...)
	s.mu.RUnlock()
	s.applyManifestMemoryPermissions(agentID)

	if err := s.memoryStore.SetAttachmentsFromLinks(agentID, links); err != nil {
		return memory.RuntimeMemoryContract{}, fmt.Errorf("memory attachment setup failed: %w", err)
	}
	contract, err := s.memoryStore.PrepareAgentMemory(agentID)
	if err != nil {
		return memory.RuntimeMemoryContract{}, fmt.Errorf("memory view compose failed: %w", err)
	}
	s.appendLog(agentID, fmt.Sprintf("memory effective view prepared (digest=%s)", contract.ViewDigest))
	s.setMemoryContractState(agentID, contract.ViewDigest, "")
	return contract, nil
}

func (s *Service) applyManifestMemoryPermissions(agentID string) {
	if s.memoryStore == nil {
		return
	}
	s.mu.RLock()
	m, ok := s.manifests[agentID]
	s.mu.RUnlock()
	if !ok {
		return
	}
	for _, scope := range m.Memory.Permissions.ReadScopes {
		trimmed := strings.TrimSpace(scope)
		if trimmed == "" {
			continue
		}
		_ = s.memoryStore.AttachScope(agentID, memory.Scope(trimmed))
	}
	existingGrants := s.memoryStore.ListGrants(agentID)
	activeGrants := make(map[string]struct{}, len(existingGrants))
	for _, g := range existingGrants {
		if g.RevokedAt == nil {
			activeGrants[strings.TrimSpace(string(g.Scope))] = struct{}{}
		}
	}
	for _, scope := range m.Memory.Permissions.WriteScopes {
		trimmed := strings.TrimSpace(scope)
		if trimmed == "" {
			continue
		}
		if _, found := activeGrants[trimmed]; !found {
			_, _ = s.memoryStore.GrantScope(agentID, memory.Scope(trimmed), "manifest:"+agentID, "manifest write scope")
			activeGrants[trimmed] = struct{}{}
		}
	}
}

// autoUnmountMemories unmounts all memories for an agent on stop.
func (s *Service) autoUnmountMemories(agentID string) {
	if s.memoryStore == nil {
		return
	}
	n := s.memoryStore.UnmountAll(agentID)
	if n > 0 {
		s.appendLog(agentID, fmt.Sprintf("memory auto-unmounted %d memories", n))
	}
}

// SetMemoryAttachments sets the memory links for an agent (alias for setMemoryAttachments).
func (s *Service) SetMemoryAttachments(agentID string, attachments []string) {
	s.setMemoryAttachments(agentID, attachments)
}

// GetMemoryAttachments returns the memory links for an agent (alias for getMemoryAttachments).
func (s *Service) GetMemoryAttachments(agentID string) []string {
	return s.getMemoryAttachments(agentID)
}

func (s *Service) getMemoryAttachments(agentID string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	attachments := s.memoryLinks[agentID]
	return append([]string(nil), attachments...)
}

func (s *Service) setMemoryAttachments(agentID string, attachments []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.memoryLinks[agentID] = append([]string(nil), attachments...)
}

func buildMemoryContractID(agentID, digest string) string {
	short := strings.TrimSpace(digest)
	if len(short) > memoryContractIDDigestLength {
		short = short[:memoryContractIDDigestLength]
	}
	if short == "" {
		short = "empty"
	}
	return fmt.Sprintf("%s-%s", strings.TrimSpace(agentID), short)
}

func (s *Service) setMemoryContractState(agentID, digest, syncError string) {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[agentID]
	if !ok {
		return
	}
	memoryState := &MemoryState{
		ContractID:     buildMemoryContractID(agentID, digest),
		ContractDigest: strings.TrimSpace(digest),
		SyncError:      strings.TrimSpace(syncError),
		SyncedAt:       &now,
	}
	if strings.TrimSpace(syncError) != "" {
		memoryState.SyncState = "error"
	} else {
		memoryState.SyncState = "ready"
	}
	state.Memory = memoryState
	s.states[agentID] = state
}
