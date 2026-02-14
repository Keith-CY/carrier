package lifecycle

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

func (s *Service) recordAudit(requestID, actor, action, target string, result AuditResult, errorCode, message string) {
	if actor == "" {
		actor = "system"
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.auditLogs = append(s.auditLogs, AuditLog{
		RequestID: requestID,
		Actor:     actor,
		Action:    action,
		Target:    target,
		Result:    result,
		ErrorCode: errorCode,
		Message:   message,
		Timestamp: s.now(),
	})
	if len(s.auditLogs) > s.auditLogLimit {
		s.auditLogs = s.auditLogs[len(s.auditLogs)-s.auditLogLimit:]
	}
}
