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
)

// ConversationMessage is a normalized message representation stored per session.
type ConversationMessage struct {
	Role      string
	Content   string
	Timestamp time.Time
}

// ConversationSession contains the per-session transcript and compacted summary.
type ConversationSession struct {
	Key       string
	Messages  []ConversationMessage
	Summary   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// SessionStats summarizes session utilization.
type SessionStats struct {
	Key           string
	MessageCount  int
	SummaryLength int
	UpdatedAt     time.Time
}

// SessionManager stores and compacts chat sessions in memory.
type SessionManager struct {
	mu          sync.RWMutex
	sessions    map[string]*ConversationSession
	maxMessages int
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
	s.Messages = append(s.Messages, ConversationMessage{
		Role:      role,
		Content:   strings.TrimSpace(content),
		Timestamp: now,
	})
	s.UpdatedAt = now
	sm.compactLocked(s)
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
		Key:       sessionKey,
		Messages:  []ConversationMessage{},
		CreatedAt: now,
		UpdatedAt: now,
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
