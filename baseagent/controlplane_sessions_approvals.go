package baseagent

import (
	"strings"
	"time"
)

func (sm *SessionManager) PendingApproval(sessionKey string) *PendingToolApproval {
	pending := sm.PendingApprovals(sessionKey)
	if len(pending) == 0 {
		return nil
	}
	return pending[0]
}

func (sm *SessionManager) PendingApprovals(sessionKey string) []*PendingToolApproval {
	if sm == nil {
		return nil
	}
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return nil
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	s, ok := sm.sessions[sessionKey]
	if !ok {
		return nil
	}
	now := time.Now().UTC()
	if sm.pruneExpiredApprovalsLocked(s, now) {
		s.UpdatedAt = now
		sm.persistSessionLocked(s)
	}
	return clonePendingToolApprovals(s.PendingApprovals)
}

func (sm *SessionManager) ConsumePendingApproval(sessionKey, approvalID string) (*PendingToolApproval, bool) {
	if sm == nil {
		return nil, false
	}
	sessionKey = strings.TrimSpace(sessionKey)
	approvalID = strings.TrimSpace(approvalID)
	if sessionKey == "" || approvalID == "" {
		return nil, false
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	s, ok := sm.sessions[sessionKey]
	if !ok {
		return nil, false
	}
	now := time.Now().UTC()
	sm.pruneExpiredApprovalsLocked(s, now)
	for idx, pending := range s.PendingApprovals {
		if strings.TrimSpace(pending.ID) != approvalID {
			continue
		}
		consumed := clonePendingToolApproval(pending)
		s.PendingApprovals = append(s.PendingApprovals[:idx], s.PendingApprovals[idx+1:]...)
		s.PendingApproval = nil
		s.UpdatedAt = now
		sm.persistSessionLocked(s)
		return consumed, true
	}
	return nil, false
}

func (sm *SessionManager) SetPendingApproval(sessionKey string, pending *PendingToolApproval) {
	if sm == nil {
		return
	}
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()
	now := time.Now().UTC()
	s := sm.getOrCreateLocked(sessionKey, now)
	normalized := normalizePendingToolApproval(pending, now)
	if normalized == nil {
		return
	}
	replaced := false
	for idx, existing := range s.PendingApprovals {
		if strings.TrimSpace(existing.ID) != normalized.ID {
			continue
		}
		s.PendingApprovals[idx] = normalized
		replaced = true
		break
	}
	if !replaced {
		s.PendingApprovals = append(s.PendingApprovals, normalized)
	}
	sm.pruneExpiredApprovalsLocked(s, now)
	s.PendingApproval = nil
	s.UpdatedAt = now
	sm.persistSessionLocked(s)
}

func (sm *SessionManager) SetPendingApprovals(sessionKey string, pendings []*PendingToolApproval) {
	if sm == nil {
		return
	}
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()
	now := time.Now().UTC()
	s := sm.getOrCreateLocked(sessionKey, now)
	normalizedList := make([]*PendingToolApproval, 0, len(pendings))
	for _, pending := range pendings {
		normalized := normalizePendingToolApproval(pending, now)
		if normalized == nil {
			continue
		}
		normalizedList = append(normalizedList, normalized)
	}
	s.PendingApprovals = normalizedList
	sm.pruneExpiredApprovalsLocked(s, now)
	s.PendingApproval = nil
	s.UpdatedAt = now
	sm.persistSessionLocked(s)
}

func (sm *SessionManager) ClearPendingApproval(sessionKey string) {
	if sm == nil {
		return
	}
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()
	s, ok := sm.sessions[sessionKey]
	if !ok || len(s.PendingApprovals) == 0 {
		return
	}
	s.PendingApprovals = nil
	s.PendingApproval = nil
	s.UpdatedAt = time.Now().UTC()
	sm.persistSessionLocked(s)
}

func (sm *SessionManager) ApprovalAudit(sessionKey string) []*PendingToolApproval {
	if sm == nil {
		return nil
	}
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return nil
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	s, ok := sm.sessions[sessionKey]
	if !ok {
		return nil
	}
	now := time.Now().UTC()
	if sm.pruneExpiredApprovalsLocked(s, now) {
		s.UpdatedAt = now
		sm.persistSessionLocked(s)
	}
	return clonePendingToolApprovals(s.ApprovalAudit)
}

func (sm *SessionManager) RecordApprovalDecision(sessionKey string, pending *PendingToolApproval, decision string) {
	if sm == nil {
		return
	}
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()
	now := time.Now().UTC()
	s := sm.getOrCreateLocked(sessionKey, now)
	sm.appendApprovalAuditLocked(s, resolvedPendingToolApproval(pending, decision, now))
	s.PendingApproval = nil
	s.UpdatedAt = now
	sm.persistSessionLocked(s)
}

func clonePendingToolApproval(pending *PendingToolApproval) *PendingToolApproval {
	if pending == nil {
		return nil
	}
	return &PendingToolApproval{
		ID:          strings.TrimSpace(pending.ID),
		ToolName:    strings.TrimSpace(pending.ToolName),
		Arguments:   cloneToolSchema(pending.Arguments),
		RequestedAt: pending.RequestedAt,
		ExpiresAt:   pending.ExpiresAt,
		ResolvedAt:  pending.ResolvedAt,
		Decision:    strings.TrimSpace(pending.Decision),
		Reason:      strings.TrimSpace(pending.Reason),
		RuleID:      strings.TrimSpace(pending.RuleID),
	}
}

func clonePendingToolApprovals(pending []*PendingToolApproval) []*PendingToolApproval {
	if len(pending) == 0 {
		return nil
	}
	out := make([]*PendingToolApproval, 0, len(pending))
	for _, item := range pending {
		if cloned := clonePendingToolApproval(item); cloned != nil {
			out = append(out, cloned)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizePendingToolApproval(pending *PendingToolApproval, now time.Time) *PendingToolApproval {
	normalized := clonePendingToolApproval(pending)
	if normalized == nil {
		return nil
	}
	if normalized.ID == "" || normalized.ToolName == "" {
		return nil
	}
	if normalized.RequestedAt.IsZero() {
		normalized.RequestedAt = now
	}
	if normalized.ExpiresAt.IsZero() {
		normalized.ExpiresAt = normalized.RequestedAt.Add(defaultPendingApprovalTTL)
	}
	normalized.ResolvedAt = time.Time{}
	normalized.Decision = ""
	return normalized
}

func resolvedPendingToolApproval(pending *PendingToolApproval, decision string, now time.Time) *PendingToolApproval {
	normalized := clonePendingToolApproval(pending)
	if normalized == nil {
		return nil
	}
	normalized.Decision = strings.TrimSpace(decision)
	if normalized.ResolvedAt.IsZero() {
		normalized.ResolvedAt = now
	}
	return normalized
}

func (sm *SessionManager) appendApprovalAuditLocked(session *ConversationSession, pending *PendingToolApproval) {
	if session == nil || pending == nil {
		return
	}
	session.ApprovalAudit = append(session.ApprovalAudit, pending)
	if len(session.ApprovalAudit) > maxApprovalAuditEntries {
		session.ApprovalAudit = clonePendingToolApprovals(session.ApprovalAudit[len(session.ApprovalAudit)-maxApprovalAuditEntries:])
	}
}

func (sm *SessionManager) pruneExpiredApprovalsLocked(session *ConversationSession, now time.Time) bool {
	if session == nil || len(session.PendingApprovals) == 0 {
		return false
	}
	changed := false
	remaining := make([]*PendingToolApproval, 0, len(session.PendingApprovals))
	for _, pending := range session.PendingApprovals {
		normalized := normalizePendingToolApproval(pending, now)
		if normalized == nil {
			changed = true
			continue
		}
		if !normalized.ExpiresAt.IsZero() && !normalized.ExpiresAt.After(now) {
			sm.appendApprovalAuditLocked(session, resolvedPendingToolApproval(normalized, approvalDecisionExpired, now))
			changed = true
			continue
		}
		remaining = append(remaining, normalized)
	}
	session.PendingApprovals = remaining
	session.PendingApproval = nil
	return changed
}
