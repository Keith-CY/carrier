package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGatewayAuditLogPathPrefersEnv(t *testing.T) {
	customPath := filepath.Join(t.TempDir(), "audit.jsonl")
	t.Setenv("CARRIER_GATEWAY_AUDIT_LOG", customPath)

	path, err := gatewayAuditLogPath()
	if err != nil {
		t.Fatalf("gatewayAuditLogPath error: %v", err)
	}
	if path != customPath {
		t.Fatalf("expected custom audit log path %q, got %q", customPath, path)
	}
}

func TestGatewayAuditLogPathFallbackHome(t *testing.T) {
	t.Setenv("CARRIER_GATEWAY_AUDIT_LOG", "")
	path, err := gatewayAuditLogPath()
	if err != nil {
		t.Fatalf("gatewayAuditLogPath error: %v", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir error: %v", err)
	}
	expected := filepath.Join(home, ".carrier", "gateway-audit.jsonl")
	if path != expected {
		t.Fatalf("expected fallback path %q, got %q", expected, path)
	}
}

func TestSanitizeAuditDetails(t *testing.T) {
	if got := sanitizeAuditDetails(nil); got != nil {
		t.Fatalf("expected nil for nil input, got %#v", got)
	}

	details := map[string]interface{}{
		"api_token": "API_TOKEN=TOKEN_abc123",
		"message":   "hello",
		"count":     3,
	}
	sanitized := sanitizeAuditDetails(details)
	if _, ok := sanitized["count"]; !ok {
		t.Fatalf("expected count preserved in details")
	}
	s := sanitized["api_token"].(string)
	if s == "API_TOKEN=TOKEN_abc123" || !strings.Contains(s, "***REDACTED***") {
		t.Fatalf("expected token redacted, got %q", s)
	}
	if sanitized["message"].(string) != "hello" {
		t.Fatalf("unexpected message value: %#v", sanitized["message"])
	}
}

func TestAppendGatewayAuditEventAndRotate(t *testing.T) {
	auditLog := filepath.Join(t.TempDir(), "gateway-audit.jsonl")

	if err := os.WriteFile(auditLog, strings.Repeat("a", int(maxGatewayAuditLogBytes+1)), 0o600); err != nil {
		t.Fatalf("prepare audit log for rotation error: %v", err)
	}

	event := gatewayAuditEvent{Action: "upload", Target: "host", Result: "ok"}
	appendGatewayAuditEvent(auditLog, event)

	rotated := auditLog + ".1"
	if _, err := os.Stat(rotated); err != nil {
		t.Fatalf("expected rotated log file %s to exist, err=%v", rotated, err)
	}
	current, err := os.ReadFile(auditLog)
	if err != nil {
		t.Fatalf("read current audit log error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(current)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected one line in current log, got %d", len(lines))
	}

	var got gatewayAuditEvent
	if err := json.Unmarshal(current, &got); err != nil {
		if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
			t.Fatalf("decode log entry error: %v", err)
		}
	}
	if got.Action != "upload" || got.Target != "host" || got.Result != "ok" {
		t.Fatalf("unexpected logged event: %+v", got)
	}

	emptyPath := filepath.Join(t.TempDir(), "existing-audit.jsonl")
	before := []byte("seed")
	if err := os.WriteFile(emptyPath, before, 0o600); err != nil {
		t.Fatalf("prepare existing audit file error: %v", err)
	}
	badEvent := gatewayAuditEvent{Action: "bad", Details: map[string]interface{}{"fn": func() {}}}
	appendGatewayAuditEvent(emptyPath, badEvent)
	if data, err := os.ReadFile(emptyPath); err != nil {
		t.Fatalf("read existing audit file after failed marshal error: %v", err)
	} else if len(data) != len(before) {
		t.Fatalf("expected marshal failure to skip write, before=%d after=%d", len(before), len(data))
	}

	appendGatewayAuditEvent("", event)
}

func TestEmitRemoteAuditEventWritesTrimmedAndSanitized(t *testing.T) {
	auditLog := filepath.Join(t.TempDir(), "emit-audit.jsonl")
	t.Setenv("CARRIER_GATEWAY_AUDIT_LOG", auditLog)

	emitRemoteAuditEvent("  req-1 ", " Upload ", " host-1 ", " ok ", map[string]interface{}{
		"note":  "API_TOKEN=token-secret",
		"count": 1,
	})

	data, err := os.ReadFile(auditLog)
	if err != nil {
		t.Fatalf("read audit log error: %v", err)
	}
	line := strings.TrimSpace(string(data))
	var got map[string]interface{}
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("unmarshal audit line error: %v", err)
	}
	if got["requestId"] != "req-1" {
		t.Fatalf("expected trimmed requestId, got %#v", got["requestId"])
	}
	if got["action"] != "Upload" {
		t.Fatalf("expected trimmed action, got %#v", got["action"])
	}
	if got["target"] != "host-1" {
		t.Fatalf("expected trimmed target, got %#v", got["target"])
	}
	if got["result"] != "ok" {
		t.Fatalf("expected trimmed result, got %#v", got["result"])
	}
	if got["timestamp"] == "" {
		t.Fatalf("expected timestamp to be set")
	}
	details := got["details"].(map[string]interface{})
	if sanitized := details["note"].(string); !strings.Contains(sanitized, "***REDACTED***") {
		t.Fatalf("expected redacted note value, got %q", sanitized)
	}
}
