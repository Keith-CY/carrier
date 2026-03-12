package baseagent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// ConversationMessage is a normalized message representation stored per session.
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

// ConversationSession contains the per-session transcript and compacted summary.
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

func NewSessionManagerWithStorage(maxMessages int, storageDir string) *SessionManager {
	sm := NewSessionManager(maxMessages)
	sm.storageDir = strings.TrimSpace(storageDir)
	if sm.storageDir == "" {
		return sm
	}
	_ = os.MkdirAll(sm.storageDir, 0o755)
	sm.loadFromStorage()
	return sm
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

func (sm *SessionManager) loadFromStorage() {
	if sm == nil || strings.TrimSpace(sm.storageDir) == "" {
		return
	}
	entries, err := os.ReadDir(sm.storageDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(sm.storageDir, entry.Name()))
		if err != nil {
			continue
		}
		var session ConversationSession
		if err := json.Unmarshal(raw, &session); err != nil {
			continue
		}
		filename, ok := sessionStorageFilename(session.Key)
		if !ok || entry.Name() != filename {
			continue
		}
		normalizeLoadedConversationSession(&session)
		session.Summary = truncateSummary(session.Summary)
		now := time.Now().UTC()
		if sm.pruneExpiredApprovalsLocked(&session, now) {
			session.UpdatedAt = now
		}
		sm.sessions[session.Key] = &session
	}
}

func (sm *SessionManager) persistSessionLocked(session *ConversationSession) {
	if sm == nil || session == nil || strings.TrimSpace(sm.storageDir) == "" {
		return
	}
	persisted := *session
	persisted.PendingApproval = nil
	persisted.PendingApprovals = clonePendingToolApprovals(session.PendingApprovals)
	persisted.ApprovalAudit = clonePendingToolApprovals(session.ApprovalAudit)
	raw, err := json.MarshalIndent(&persisted, "", "  ")
	if err != nil {
		return
	}
	filename, ok := sessionStorageFilename(session.Key)
	if !ok {
		return
	}
	_ = os.WriteFile(filepath.Join(sm.storageDir, filename), raw, 0o600)
}

func sessionStorageFilename(sessionKey string) (string, bool) {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" || sessionKey == "." || sessionKey == ".." {
		return "", false
	}
	if strings.Contains(sessionKey, "/") || strings.Contains(sessionKey, "\\") {
		return "", false
	}

	filename := strings.NewReplacer(":", "_").Replace(sessionKey)
	filename = strings.TrimSpace(filename)
	if filename == "" || filename == "." || filename == ".." {
		return "", false
	}
	return filename + ".json", true
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

func normalizeLoadedConversationSession(session *ConversationSession) {
	if session == nil {
		return
	}
	if len(session.StructuredMessages) == 0 && len(session.Messages) > 0 {
		session.StructuredMessages = structuredMessagesFromConversationHistory(session.Messages)
	}
	if len(session.Messages) == 0 && len(session.StructuredMessages) > 0 {
		session.Messages = flattenStructuredMessages(session.StructuredMessages)
	}
	if len(session.StructuredMessages) > 0 && len(session.Messages) > len(session.StructuredMessages) {
		session.Messages = append([]ConversationMessage(nil), session.Messages[:len(session.StructuredMessages)]...)
	}
	if len(session.Messages) > 0 && len(session.StructuredMessages) > len(session.Messages) {
		session.StructuredMessages = cloneStructuredToolMessages(session.StructuredMessages[:len(session.Messages)])
	}
	session.StructuredMessages = cloneStructuredToolMessages(session.StructuredMessages)
	if len(session.PendingApprovals) == 0 && session.PendingApproval != nil {
		session.PendingApprovals = []*PendingToolApproval{clonePendingToolApproval(session.PendingApproval)}
	}
	session.PendingApproval = nil
	session.PendingApprovals = clonePendingToolApprovals(session.PendingApprovals)
	session.ApprovalAudit = clonePendingToolApprovals(session.ApprovalAudit)
}

func normalizeStructuredToolMessage(msg StructuredToolMessage) StructuredToolMessage {
	msg.Role = strings.TrimSpace(strings.ToLower(msg.Role))
	msg.Content = strings.TrimSpace(msg.Content)
	msg.Attachments = cloneAttachmentRefs(msg.Attachments)
	msg.ContentBlocks = cloneContentBlocks(msg.ContentBlocks)
	msg.ToolCallID = strings.TrimSpace(msg.ToolCallID)
	msg.ToolName = strings.TrimSpace(msg.ToolName)
	msg.ToolPolicyReason = strings.TrimSpace(msg.ToolPolicyReason)
	msg.ToolPolicyRuleID = strings.TrimSpace(msg.ToolPolicyRuleID)
	msg.ToolCalls = cloneStructuredToolCalls(msg.ToolCalls)
	if msg.ToolResultStatus != "" {
		msg.ToolResultStatus = ExecutionToolResultStatus(strings.TrimSpace(string(msg.ToolResultStatus)))
	}
	return msg
}

func structuredMessagesFromConversationHistory(history []ConversationMessage) []StructuredToolMessage {
	out := make([]StructuredToolMessage, 0, len(history))
	for _, msg := range history {
		role := strings.TrimSpace(strings.ToLower(msg.Role))
		if role == "" {
			continue
		}
		out = append(out, StructuredToolMessage{
			Role:    role,
			Content: strings.TrimSpace(msg.Content),
		})
	}
	return out
}

func flattenStructuredMessages(history []StructuredToolMessage) []ConversationMessage {
	out := make([]ConversationMessage, 0, len(history))
	for _, msg := range history {
		normalized := normalizeStructuredToolMessage(msg)
		if normalized.Role == "" {
			continue
		}
		out = append(out, ConversationMessage{
			Role:    normalized.Role,
			Content: normalized.Content,
		})
	}
	return out
}

func cloneStructuredToolMessages(history []StructuredToolMessage) []StructuredToolMessage {
	if len(history) == 0 {
		return nil
	}
	out := make([]StructuredToolMessage, 0, len(history))
	for _, msg := range history {
		out = append(out, normalizeStructuredToolMessage(msg))
	}
	return out
}

func cloneStructuredToolCalls(calls []StructuredToolCall) []StructuredToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]StructuredToolCall, len(calls))
	for i, call := range calls {
		out[i] = StructuredToolCall{
			ID:        strings.TrimSpace(call.ID),
			Name:      strings.TrimSpace(call.Name),
			Arguments: cloneToolSchema(call.Arguments),
		}
	}
	return out
}

func cloneAttachmentRefs(attachments []AttachmentRef) []AttachmentRef {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]AttachmentRef, len(attachments))
	for i, attachment := range attachments {
		out[i] = AttachmentRef{
			ID:             strings.TrimSpace(attachment.ID),
			Kind:           strings.TrimSpace(strings.ToLower(attachment.Kind)),
			Path:           strings.TrimSpace(attachment.Path),
			Name:           strings.TrimSpace(attachment.Name),
			MIMEType:       strings.TrimSpace(attachment.MIMEType),
			MediaType:      strings.TrimSpace(attachment.MediaType),
			SizeBytes:      attachment.SizeBytes,
			Source:         strings.TrimSpace(attachment.Source),
			ExternalID:     strings.TrimSpace(attachment.ExternalID),
			ArtifactID:     strings.TrimSpace(attachment.ArtifactID),
			SourceMetadata: cloneStringStringMap(attachment.SourceMetadata),
		}
		if out[i].MediaType == "" {
			out[i].MediaType = out[i].MIMEType
		}
		if out[i].ID == "" {
			out[i].ID = strings.TrimSpace(firstNonEmptyString(out[i].ExternalID, out[i].ArtifactID, out[i].Path, out[i].Name))
		}
	}
	return out
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func cloneStringStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		out[trimmedKey] = strings.TrimSpace(value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneContentBlocks(blocks []ContentBlock) []ContentBlock {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]ContentBlock, len(blocks))
	for i, block := range blocks {
		out[i] = ContentBlock{
			Type:         strings.TrimSpace(strings.ToLower(block.Type)),
			Text:         strings.TrimSpace(block.Text),
			Name:         strings.TrimSpace(block.Name),
			Path:         strings.TrimSpace(block.Path),
			MIMEType:     strings.TrimSpace(block.MIMEType),
			MediaType:    strings.TrimSpace(block.MediaType),
			AttachmentID: strings.TrimSpace(block.AttachmentID),
			URL:          strings.TrimSpace(block.URL),
			SizeBytes:    block.SizeBytes,
		}
		if out[i].MediaType == "" {
			out[i].MediaType = out[i].MIMEType
		}
	}
	return out
}
