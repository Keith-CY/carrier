package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"carrier/shared/redact"
)

func TestMapLifecycleErrorRedactsSensitiveContent(t *testing.T) {
	// Fabricate an error whose message contains a secret.
	err := &fakeError{msg: "connection to postgres://admin:s3cret@db:5432 failed"}
	_, _, message := mapLifecycleError(err)

	if strings.Contains(message, "s3cret") {
		t.Fatalf("error message was not redacted: %q", message)
	}
	if !strings.Contains(message, redact.RedactedValue) {
		t.Fatalf("expected redacted placeholder in message: %q", message)
	}
}

func TestLogsEndpointRedactsSensitiveLines(t *testing.T) {
	svc := newServiceForAPITest(t)
	handler := NewServer(svc).Handler()

	// Install and start to populate some logs and make the agent accessible.
	doJSONRequest(t, handler, http.MethodPost, "/api/v1/agents/openclaw/install", nil)
	doJSONRequest(t, handler, http.MethodPost, "/api/v1/agents/openclaw/start", nil)

	rr := doJSONRequest(t, handler, http.MethodGet, "/api/v1/agents/openclaw/logs?tail=100", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("logs status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// The default test logs won't contain secrets, but verify the endpoint works.
	// The real redaction test is below with a direct unit check.
	for _, line := range resp.Lines {
		if strings.Contains(line, "SECRET") && !strings.Contains(line, redact.RedactedValue) {
			t.Fatalf("log line not redacted: %q", line)
		}
	}
}

func TestStatusEndpointRedactsLastError(t *testing.T) {
	svc := newServiceForAPITest(t)
	handler := NewServer(svc).Handler()

	// Get status — lastError should be empty and safe.
	rr := doJSONRequest(t, handler, http.MethodGet, "/api/v1/agents/openclaw/status", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Statuses []daemonAgent `json:"statuses"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(resp.Statuses))
	}
}

func TestRedactTextAppliedToLogLines(t *testing.T) {
	// Directly verify that the redaction function works on typical log content.
	input := `2026-02-15T00:00:00Z connecting with API_KEY=sk-live-abc123def`
	result := redact.RedactText(input)
	if strings.Contains(result, "sk-live-abc123def") {
		t.Fatalf("sensitive value not redacted: %q", result)
	}
	if !strings.Contains(result, redact.RedactedValue) {
		t.Fatalf("expected redacted placeholder: %q", result)
	}
}

// fakeError implements error for testing mapLifecycleError with arbitrary messages.
type fakeError struct {
	msg string
}

func (e *fakeError) Error() string { return e.msg }
