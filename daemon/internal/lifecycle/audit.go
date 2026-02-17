package lifecycle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// AuditStatus returns the current audit buffer size and configured limit.
// This is primarily queried by operational inspection/debug endpoints.
type AuditStatus struct {
	BufferSize int `json:"buffer_size"`
	Limit      int `json:"limit"`
}

// AuditBufferStatus returns a snapshot of the audit buffer metrics.
func (s *Service) AuditBufferStatus() AuditStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return AuditStatus{
		BufferSize: len(s.auditLogs),
		Limit:      s.auditLogLimit,
	}
}

func (s *Service) AuditLogs() []AuditLog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]AuditLog(nil), s.auditLogs...)
}

// auditLogJSONL is the JSON-lines representation of an audit log entry.
type auditLogJSONL struct {
	RequestID string `json:"request_id"`
	Actor     string `json:"actor"`
	Action    string `json:"action"`
	Target    string `json:"target"`
	Result    string `json:"result"`
	ErrorCode string `json:"error_code,omitempty"`
	Message   string `json:"message,omitempty"`
	Timestamp string `json:"timestamp"`
}

// persistAuditEntry appends a single audit log entry to the JSONL audit file.
// Errors are logged but do not block the in-memory append.
func (s *Service) persistAuditEntry(entry AuditLog) {
	if s.auditLogDir == "" {
		return
	}
	if err := os.MkdirAll(s.auditLogDir, 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "[audit] failed to create audit log dir: %v\n", err)
		return
	}
	filePath := filepath.Join(s.auditLogDir, "audit.jsonl")
	// Rotate if file exceeds 10 MB to prevent unbounded growth.
	s.rotateAuditLogIfNeeded(filePath)
	record := auditLogJSONL{
		RequestID: entry.RequestID,
		Actor:     entry.Actor,
		Action:    entry.Action,
		Target:    entry.Target,
		Result:    string(entry.Result),
		ErrorCode: entry.ErrorCode,
		Message:   entry.Message,
		Timestamp: entry.Timestamp.UTC().Format(time.RFC3339Nano),
	}
	data, err := json.Marshal(record)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[audit] failed to marshal audit entry: %v\n", err)
		return
	}
	data = append(data, '\n')

	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[audit] failed to open audit file: %v\n", err)
		return
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		fmt.Fprintf(os.Stderr, "[audit] failed to write audit entry: %v\n", err)
	}
}

const maxAuditLogBytes = 10 * 1024 * 1024 // 10 MB

// rotateAuditLogIfNeeded renames the current audit log when it exceeds the
// size cap, keeping at most one rotated backup (.1). This bounds disk usage
// to ~20 MB worst-case.
func (s *Service) rotateAuditLogIfNeeded(filePath string) {
	info, err := os.Stat(filePath)
	if err != nil || info.Size() < maxAuditLogBytes {
		return
	}
	rotated := filePath + ".1"
	// Remove previous backup (best-effort).
	_ = os.Remove(rotated)
	if err := os.Rename(filePath, rotated); err != nil {
		fmt.Fprintf(os.Stderr, "[audit] failed to rotate audit log: %v\n", err)
	}
}

func (s *Service) recordAudit(requestID, actor, action, target string, result AuditResult, errorCode, message string) {
	if actor == "" {
		actor = "system"
	}

	entry := AuditLog{
		RequestID: requestID,
		Actor:     actor,
		Action:    action,
		Target:    target,
		Result:    result,
		ErrorCode: errorCode,
		Message:   message,
		Timestamp: s.now(),
	}

	s.mu.Lock()
	s.auditLogs = append(s.auditLogs, entry)
	if len(s.auditLogs) > s.auditLogLimit {
		s.auditLogs = s.auditLogs[len(s.auditLogs)-s.auditLogLimit:]
	}
	s.mu.Unlock()

	// Persist to disk (best-effort, outside the lock)
	s.persistAuditEntry(entry)
}
