package baseagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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
	persisted.PendingContextRequests = cloneDelegationContextRequests(session.PendingContextRequests)
	persisted.ContextRequestAudit = cloneDelegationContextResponses(session.ContextRequestAudit)
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
	session.PendingContextRequests = cloneDelegationContextRequests(session.PendingContextRequests)
	session.ContextRequestAudit = cloneDelegationContextResponses(session.ContextRequestAudit)
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
	msg.GuardrailEvents = NormalizeGuardrailEvents(msg.GuardrailEvents)
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
			OutputRole:     strings.TrimSpace(strings.ToLower(attachment.OutputRole)),
			Path:           strings.TrimSpace(attachment.Path),
			Name:           strings.TrimSpace(attachment.Name),
			MIMEType:       strings.TrimSpace(attachment.MIMEType),
			MediaType:      strings.TrimSpace(attachment.MediaType),
			SizeBytes:      attachment.SizeBytes,
			Source:         strings.TrimSpace(attachment.Source),
			ExternalID:     strings.TrimSpace(attachment.ExternalID),
			ArtifactID:     strings.TrimSpace(attachment.ArtifactID),
			DownloadURL:    strings.TrimSpace(attachment.DownloadURL),
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
			OutputRole:   strings.TrimSpace(strings.ToLower(block.OutputRole)),
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
