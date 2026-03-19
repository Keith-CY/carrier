package baseagent

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultSessionMaxMessages = 48
	maxSessionSummaryBytes    = 4096
	maxCompactNoteContentSize = 140
	defaultPendingApprovalTTL = 15 * time.Minute
	maxApprovalAuditEntries   = 32
)

type ConversationMessage struct {
	Role      string
	Content   string
	Timestamp time.Time
}

type PendingToolApproval struct {
	ID          string
	ToolName    string
	Arguments   map[string]any
	RequestedAt time.Time
	ExpiresAt   time.Time
	ResolvedAt  time.Time
	Decision    string
	Reason      string
	RuleID      string
}

const (
	approvalDecisionConfirmed = "confirmed"
	approvalDecisionRejected  = "rejected"
	approvalDecisionExpired   = "expired"
)

type ConversationSession struct {
	Key                string
	Messages           []ConversationMessage
	StructuredMessages []StructuredToolMessage
	Summary            string
	PendingApproval    *PendingToolApproval
	PendingApprovals   []*PendingToolApproval
	ApprovalAudit      []*PendingToolApproval
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type SessionStats struct {
	Key           string
	MessageCount  int
	SummaryLength int
	UpdatedAt     time.Time
}

type SessionManager struct {
	mu          sync.RWMutex
	sessions    map[string]*ConversationSession
	maxMessages int
	storageDir  string
}

func NewSessionManager(maxMessages int) *SessionManager {
	if maxMessages <= 0 {
		maxMessages = defaultSessionMaxMessages
	}
	return &SessionManager{
		sessions:    map[string]*ConversationSession{},
		maxMessages: maxMessages,
	}
}

func (sm *SessionManager) AddMessage(sessionKey, role, content string) {
	if sm == nil {
		return
	}
	sessionKey = strings.TrimSpace(sessionKey)
	role = strings.TrimSpace(strings.ToLower(role))
	if sessionKey == "" || role == "" {
		return
	}

	now := time.Now().UTC()
	sm.mu.Lock()
	defer sm.mu.Unlock()

	s := sm.getOrCreateLocked(sessionKey, now)
	sm.appendStructuredMessageLocked(s, StructuredToolMessage{
		Role:    role,
		Content: strings.TrimSpace(content),
	}, now)
	s.UpdatedAt = now
	sm.compactLocked(s)
	sm.persistSessionLocked(s)
}

func (sm *SessionManager) AddStructuredToolMessage(sessionKey string, msg StructuredToolMessage) {
	if sm == nil {
		return
	}
	sessionKey = strings.TrimSpace(sessionKey)
	msg = normalizeStructuredToolMessage(msg)
	if sessionKey == "" || msg.Role == "" {
		return
	}

	now := time.Now().UTC()
	sm.mu.Lock()
	defer sm.mu.Unlock()

	s := sm.getOrCreateLocked(sessionKey, now)
	sm.appendStructuredMessageLocked(s, msg, now)
	s.UpdatedAt = now
	sm.compactLocked(s)
	sm.persistSessionLocked(s)
}

func (sm *SessionManager) History(sessionKey string) []ConversationMessage {
	if sm == nil {
		return nil
	}
	sessionKey = strings.TrimSpace(sessionKey)
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s, ok := sm.sessions[sessionKey]
	if !ok {
		return nil
	}
	out := make([]ConversationMessage, len(s.Messages))
	copy(out, s.Messages)
	return out
}

func (sm *SessionManager) StructuredHistory(sessionKey string) []StructuredToolMessage {
	if sm == nil {
		return nil
	}
	sessionKey = strings.TrimSpace(sessionKey)
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s, ok := sm.sessions[sessionKey]
	if !ok {
		return nil
	}
	return cloneStructuredToolMessages(s.StructuredMessages)
}

func (sm *SessionManager) Summary(sessionKey string) string {
	if sm == nil {
		return ""
	}
	sessionKey = strings.TrimSpace(sessionKey)
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s, ok := sm.sessions[sessionKey]
	if !ok {
		return ""
	}
	return s.Summary
}

func (sm *SessionManager) SetSummary(sessionKey, summary string) {
	if sm == nil {
		return
	}
	sessionKey = strings.TrimSpace(sessionKey)
	sm.mu.Lock()
	defer sm.mu.Unlock()
	now := time.Now().UTC()
	s := sm.getOrCreateLocked(sessionKey, now)
	s.Summary = truncateSummary(summary)
	s.UpdatedAt = now
	sm.persistSessionLocked(s)
}

func (sm *SessionManager) ListStats(limit int) []SessionStats {
	if sm == nil {
		return nil
	}
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	stats := make([]SessionStats, 0, len(sm.sessions))
	for _, s := range sm.sessions {
		stats = append(stats, SessionStats{
			Key:           s.Key,
			MessageCount:  len(s.Messages),
			SummaryLength: len(s.Summary),
			UpdatedAt:     s.UpdatedAt,
		})
	}
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].UpdatedAt.After(stats[j].UpdatedAt)
	})
	if limit > 0 && len(stats) > limit {
		stats = stats[:limit]
	}
	return stats
}

func (sm *SessionManager) getOrCreateLocked(sessionKey string, now time.Time) *ConversationSession {
	if s, ok := sm.sessions[sessionKey]; ok {
		return s
	}
	s := &ConversationSession{
		Key:                sessionKey,
		Messages:           []ConversationMessage{},
		StructuredMessages: []StructuredToolMessage{},
		PendingApprovals:   []*PendingToolApproval{},
		ApprovalAudit:      []*PendingToolApproval{},
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	sm.sessions[sessionKey] = s
	return s
}

func (sm *SessionManager) compactLocked(s *ConversationSession) {
	if s == nil || len(s.Messages) <= sm.maxMessages {
		return
	}
	dropCount := len(s.Messages) - sm.maxMessages
	dropped := make([]ConversationMessage, dropCount)
	copy(dropped, s.Messages[:dropCount])
	s.Messages = append([]ConversationMessage(nil), s.Messages[dropCount:]...)
	if len(s.StructuredMessages) <= dropCount {
		s.StructuredMessages = nil
	} else {
		s.StructuredMessages = append([]StructuredToolMessage(nil), s.StructuredMessages[dropCount:]...)
	}

	compactNote := buildSessionCompactNote(dropped)
	if compactNote == "" {
		return
	}
	if s.Summary == "" {
		s.Summary = compactNote
	} else {
		s.Summary = truncateSummary(s.Summary + "\n" + compactNote)
	}
}

func buildSessionCompactNote(dropped []ConversationMessage) string {
	if len(dropped) == 0 {
		return ""
	}
	lines := make([]string, 0, 8)
	lines = append(lines, fmt.Sprintf("[Compacted %d earlier messages]", len(dropped)))
	start := len(dropped) - 6
	if start < 0 {
		start = 0
	}
	for _, msg := range dropped[start:] {
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		if len(content) > maxCompactNoteContentSize {
			content = content[:maxCompactNoteContentSize] + "..."
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", msg.Role, content))
	}
	return strings.Join(lines, "\n")
}

func truncateSummary(summary string) string {
	summary = strings.TrimSpace(summary)
	if len(summary) <= maxSessionSummaryBytes {
		return summary
	}
	return summary[:maxSessionSummaryBytes] + "..."
}

func (sm *SessionManager) appendStructuredMessageLocked(s *ConversationSession, msg StructuredToolMessage, now time.Time) {
	if s == nil {
		return
	}
	msg = normalizeStructuredToolMessage(msg)
	if msg.Role == "" {
		return
	}
	s.Messages = append(s.Messages, ConversationMessage{
		Role:      msg.Role,
		Content:   strings.TrimSpace(msg.Content),
		Timestamp: now,
	})
	s.StructuredMessages = append(s.StructuredMessages, msg)
}
