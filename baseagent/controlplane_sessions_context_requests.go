package baseagent

import (
	"strings"
	"time"
)

func (sm *SessionManager) PendingContextRequests(sessionKey string) []DelegationContextRequest {
	if sm == nil {
		return nil
	}
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return nil
	}
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s, ok := sm.sessions[sessionKey]
	if !ok {
		return nil
	}
	return cloneDelegationContextRequests(s.PendingContextRequests)
}

func (sm *SessionManager) SetPendingContextRequest(sessionKey string, request DelegationContextRequest) {
	if sm == nil {
		return
	}
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return
	}
	now := time.Now().UTC()
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := sm.getOrCreateLocked(sessionKey, now)
	normalized := normalizeDelegationContextRequest(request, request.ContractID, now)
	if normalized.RequestID == "" || normalized.Question == "" {
		return
	}
	replaced := false
	for idx := range s.PendingContextRequests {
		if strings.TrimSpace(s.PendingContextRequests[idx].RequestID) != normalized.RequestID {
			continue
		}
		s.PendingContextRequests[idx] = normalized
		replaced = true
		break
	}
	if !replaced {
		s.PendingContextRequests = append(s.PendingContextRequests, normalized)
	}
	s.UpdatedAt = now
	sm.persistSessionLocked(s)
}

func (sm *SessionManager) ResolveContextRequest(sessionKey string, response DelegationContextResponse) {
	if sm == nil {
		return
	}
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return
	}
	now := time.Now().UTC()
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s, ok := sm.sessions[sessionKey]
	if !ok {
		return
	}
	response = normalizeDelegationContextResponse(response, response.RequestID, now)
	if response.RequestID == "" {
		return
	}
	filtered := make([]DelegationContextRequest, 0, len(s.PendingContextRequests))
	for _, item := range s.PendingContextRequests {
		if strings.TrimSpace(item.RequestID) == response.RequestID {
			continue
		}
		filtered = append(filtered, item)
	}
	s.PendingContextRequests = filtered
	s.ContextRequestAudit = append(s.ContextRequestAudit, response)
	s.UpdatedAt = now
	sm.persistSessionLocked(s)
}

func (sm *SessionManager) ContextRequestAudit(sessionKey string) []DelegationContextResponse {
	if sm == nil {
		return nil
	}
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return nil
	}
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s, ok := sm.sessions[sessionKey]
	if !ok {
		return nil
	}
	return cloneDelegationContextResponses(s.ContextRequestAudit)
}
