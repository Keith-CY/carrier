package gateway

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const maxGatewayAuditLogBytes = int64(5 * 1024 * 1024) // 5MB

type gatewayAuditEvent struct {
	Timestamp string                 `json:"timestamp"`
	RequestID string                 `json:"requestId,omitempty"`
	Action    string                 `json:"action"`
	Target    string                 `json:"target,omitempty"`
	Result    string                 `json:"result"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

var gatewayAuditMu sync.Mutex

func gatewayAuditLogPath() (string, error) {
	if custom := strings.TrimSpace(os.Getenv("CARRIER_GATEWAY_AUDIT_LOG")); custom != "" {
		return custom, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir for gateway audit log: %w", err)
	}
	return filepath.Join(home, ".carrier", "gateway-audit.jsonl"), nil
}

func emitRemoteAuditEvent(requestID, action, target, result string, details map[string]interface{}) {
	path, err := gatewayAuditLogPath()
	if err != nil {
		return
	}
	event := gatewayAuditEvent{
		Timestamp: nowTimestamp(),
		RequestID: strings.TrimSpace(requestID),
		Action:    strings.TrimSpace(action),
		Target:    strings.TrimSpace(target),
		Result:    strings.TrimSpace(result),
		Details:   sanitizeAuditDetails(details),
	}
	appendGatewayAuditEvent(path, event)
}

func appendGatewayAuditEvent(path string, event gatewayAuditEvent) {
	gatewayAuditMu.Lock()
	defer gatewayAuditMu.Unlock()

	if strings.TrimSpace(path) == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	if stat, err := os.Stat(path); err == nil && stat.Size() > maxGatewayAuditLogBytes {
		_ = os.Rename(path, path+".1")
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()

	row, err := json.Marshal(event)
	if err != nil {
		return
	}
	_, _ = file.Write(append(row, '\n'))
}

func sanitizeAuditDetails(details map[string]interface{}) map[string]interface{} {
	if len(details) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(details))
	for key, value := range details {
		switch typed := value.(type) {
		case string:
			out[key] = RedactErrorMessage(strings.TrimSpace(typed))
		default:
			out[key] = value
		}
	}
	return out
}
