package lifecycle

import (
	"fmt"

	"carrier/daemon/internal/memory"
)

// autoMountMemories mounts all memory links for an agent on start.
func (s *Service) autoMountMemories(agentID string) {
	if s.memoryStore == nil {
		return
	}
	s.mu.RLock()
	links := append([]string(nil), s.memoryLinks[agentID]...)
	s.mu.RUnlock()
	if len(links) == 0 {
		return
	}

	// Preferred path for issue #1189: deterministic composed effective memory view.
	if err := s.memoryStore.SetAttachmentsFromLinks(agentID, links); err != nil {
		s.appendLog(agentID, fmt.Sprintf("memory attachment setup failed: %v", err))
	} else {
		contract, err := s.memoryStore.PrepareAgentMemory(agentID)
		if err == nil {
			s.appendLog(agentID, fmt.Sprintf("memory effective view prepared (digest=%s)", contract.ViewDigest))
			return
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
