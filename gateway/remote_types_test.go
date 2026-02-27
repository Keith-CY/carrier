package gateway

import (
	"strings"
	"testing"
)

func TestValidateAgentIdentifier(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		{"valid simple", "my-agent", false, ""},
		{"valid with dots", "agent.v1.0", false, ""},
		{"valid with underscore", "my_agent_01", false, ""},
		{"empty", "", true, "agent id is required"},
		{"whitespace only", "   ", true, "agent id is required"},
		{"shell injection semicolon", "agent;rm -rf /", true, "unsupported character"},
		{"shell injection backtick", "agent`id`", true, "unsupported character"},
		{"shell injection dollar", "agent$(cmd)", true, "unsupported character"},
		{"path traversal", "../../../etc/passwd", true, "unsupported character"},
		{"null byte", "agent\x00id", true, "unsupported character"},
		{"newline injection", "agent\nid", true, "unsupported character"},
		{"pipe injection", "agent|cat /etc/shadow", true, "unsupported character"},
		{"spaces", "agent id", true, "unsupported character"},
		{"unicode", "agent-日本語", true, "unsupported character"},
		{"too long", strings.Repeat("a", 129), true, "exceeds maximum length"},
		{"exactly 128", strings.Repeat("a", 128), false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAgentIdentifier(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errMsg)
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Fatalf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateRemoteSessionIdentifier(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		{"valid simple", "session-123", false, ""},
		{"valid with dots", "sess.v2.main", false, ""},
		{"empty", "", true, "sessionId is required"},
		{"whitespace only", "  ", true, "sessionId is required"},
		{"shell injection semicolon", "sess;rm -rf /", true, "unsupported character"},
		{"shell injection backtick", "sess`id`", true, "unsupported character"},
		{"shell injection dollar", "sess$(cmd)", true, "unsupported character"},
		{"path traversal", "../../etc/passwd", true, "unsupported character"},
		{"null byte", "sess\x00id", true, "unsupported character"},
		{"newline injection", "sess\nid", true, "unsupported character"},
		{"pipe injection", "sess|cat /etc/shadow", true, "unsupported character"},
		{"too long", strings.Repeat("b", 129), true, "exceeds maximum length"},
		{"exactly 128", strings.Repeat("b", 128), false, ""},
		{"trims whitespace", "  valid-id  ", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validateRemoteSessionIdentifier(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errMsg)
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Fatalf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if strings.TrimSpace(tt.input) != result {
					t.Fatalf("expected trimmed %q, got %q", strings.TrimSpace(tt.input), result)
				}
			}
		})
	}
}
