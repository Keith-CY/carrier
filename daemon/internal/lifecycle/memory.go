package lifecycle

import (
	"fmt"
	"strings"

	"carrier/daemon/internal/memory"
)

// autoMountMemories mounts all memory links for an agent on start.
func (s *Service) autoMountMemories(agentID string) map[string]string {
	if s.memoryStore == nil {
		return nil
	}
	s.mu.RLock()
	links := append([]string(nil), s.memoryLinks[agentID]...)
	s.mu.RUnlock()
	if len(links) == 0 {
		s.applyManifestMemoryPermissions(agentID)
		return nil
	}
	s.applyManifestMemoryPermissions(agentID)

	// Preferred path for issue #1189: deterministic composed effective memory view.
	if err := s.memoryStore.SetAttachmentsFromLinks(agentID, links); err != nil {
		s.appendLog(agentID, fmt.Sprintf("memory attachment setup failed: %v", err))
	} else {
		contract, err := s.memoryStore.PrepareAgentMemory(agentID)
		if err == nil {
			s.appendLog(agentID, fmt.Sprintf("memory effective view prepared (digest=%s)", contract.ViewDigest))
			return contract.Env
		}
		s.appendLog(agentID, fmt.Sprintf("memory view compose failed, falling back to direct mounts: %v", err))
	}

	// Backwards-compatible path when view composition cannot run (for example, no root dir configured).
	for _, memID := range links {
		if _, err := s.memoryStore.Mount(memID, agentID, memory.AccessReadOnly); err != nil {
			s.appendLog(agentID, fmt.Sprintf("memory auto-mount %s failed: %v", memID, err))
		} else {
			s.appendLog(agentID, fmt.Sprintf("memory auto-mounted %s", memID))
		}
	}
	return nil
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
	for _, scope := range m.Memory.Permissions.WriteScopes {
		trimmed := strings.TrimSpace(scope)
		if trimmed == "" {
			continue
		}
		existing := s.memoryStore.ListGrants(agentID)
		found := false
		for _, g := range existing {
			if string(g.Scope) == trimmed && g.RevokedAt == nil {
				found = true
				break
			}
		}
		if !found {
			_, _ = s.memoryStore.GrantScope(agentID, memory.Scope(trimmed), "manifest:"+agentID, "manifest write scope")
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
