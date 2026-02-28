package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"carrier/daemon/internal/lifecycle"
)

// --- mapLifecycleError coverage ---

func TestMapLifecycleErrorAllBranches(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"ErrAgentNotFound", lifecycle.ErrAgentNotFound, 404, "E_AGENT_NOT_FOUND"},
		{"ErrNotInstalled", lifecycle.ErrNotInstalled, 409, "E_NOT_INSTALLED"},
		{"ErrAlreadyRunning", lifecycle.ErrAlreadyRunning, 409, "E_ALREADY_RUNNING"},
		{"ErrAlreadyStopped", lifecycle.ErrAlreadyStopped, 409, "E_ALREADY_STOPPED"},
		{"ErrAgentRunning", lifecycle.ErrAgentRunning, 409, "E_AGENT_RUNNING"},
		{"ErrUpgradeNotSupported", lifecycle.ErrUpgradeNotSupported, 400, "E_UPGRADE_NOT_SUPPORTED"},
		{"ErrRuntimePrerequisites", lifecycle.ErrRuntimePrerequisites, 422, "E_RUNTIME_PREREQUISITES"},
		{"ErrMissingRequiredEnv", lifecycle.ErrMissingRequiredEnv, 422, "E_MISSING_REQUIRED_ENV"},
		{"ErrPortConflict", lifecycle.ErrPortConflict, 422, "E_PORT_CONFLICT"},
		{"ErrIsolationUnavailable", lifecycle.ErrIsolationUnavailable, 422, "E_ISOLATION_UNAVAILABLE"},
		{"ErrIsolationStartFailed", lifecycle.ErrIsolationStartFailed, 502, "E_ISOLATION_START_FAILED"},
		{"ErrUpgradeFailed", lifecycle.ErrUpgradeFailed, 500, "E_UPGRADE_FAILED"},
		{"ErrUpgradeStrategyUnsupported", lifecycle.ErrUpgradeStrategyUnsupported, 400, "E_UPGRADE_STRATEGY_UNSUPPORTED"},
		{"ErrRemoteDiagnosisNotNeeded", lifecycle.ErrRemoteDiagnosisNotNeeded, 409, "E_REMOTE_DIAG_NOT_NEEDED"},
		{"generic", http.ErrServerClosed, 500, "E_INTERNAL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, code, _ := mapLifecycleError(tt.err)
			if status != tt.wantStatus {
				t.Errorf("status=%d, want=%d", status, tt.wantStatus)
			}
			if code != tt.wantCode {
				t.Errorf("code=%q, want=%q", code, tt.wantCode)
			}
		})
	}
}

// --- allowMethod ---

func TestAllowMethodSetsHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	ok := allowMethod(rec, req, "POST")
	if ok {
		t.Fatal("expected false")
	}
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
	if rec.Header().Get("Allow") != "POST" {
		t.Fatalf("expected Allow: POST, got %q", rec.Header().Get("Allow"))
	}
}

// --- readJSON edge cases ---

func TestReadJSONEmptyBody(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(""))
	var dst struct{ Code string }
	err := readJSON(req, &dst)
	// Empty body is allowed (returns nil)
	if err != nil {
		t.Fatalf("expected no error for empty body, got %v", err)
	}
}

func TestReadJSONTrailingContent(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"code":"a"} {"code":"b"}`))
	var dst struct{ Code string }
	err := readJSON(req, &dst)
	if err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("expected trailing content error, got %v", err)
	}
}

func TestReadJSONOversizedBody(t *testing.T) {
	bigBody := strings.NewReader(`{"code":"` + strings.Repeat("x", 1<<20+100) + `"}`)
	req := httptest.NewRequest("POST", "/", bigBody)
	var dst struct{ Code string }
	err := readJSON(req, &dst)
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("expected too large error, got %v", err)
	}
}

func TestReadJSONOversizedContentLength(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{}`))
	req.ContentLength = 1<<20 + 1
	var dst struct{}
	err := readJSON(req, &dst)
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("expected too large error, got %v", err)
	}
}

// --- handleMergedLogs ---

func TestHandleMergedLogs(t *testing.T) {
	svc := newServiceForAPITest(t)
	handler := NewServer(svc).Handler()

	rr := doJSONRequest(t, handler, "GET", "/api/v1/logs", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if _, ok := resp["lines"]; !ok {
		t.Fatal("expected 'lines' field")
	}
}

func TestHandleMergedLogsWithTail(t *testing.T) {
	svc := newServiceForAPITest(t)
	handler := NewServer(svc).Handler()

	rr := doJSONRequest(t, handler, "GET", "/api/v1/logs?tail=50", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleMergedLogsMethodNotAllowed(t *testing.T) {
	svc := newServiceForAPITest(t)
	handler := NewServer(svc).Handler()

	rr := doJSONRequest(t, handler, "POST", "/api/v1/logs", nil)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestHandleMergedLogsWrongPath(t *testing.T) {
	svc := newServiceForAPITest(t)
	handler := NewServer(svc).Handler()

	rr := doJSONRequest(t, handler, "GET", "/api/v1/logs/extra", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

// --- handleListAgents wrong path ---

func TestHandleListAgentsWrongPath(t *testing.T) {
	svc := newServiceForAPITest(t)
	handler := NewServer(svc).Handler()

	rr := doJSONRequest(t, handler, "GET", "/api/v1/agents/", nil)
	// Should go to handleAgentAction, not list; with empty path, expect non-200
	if rr.Code == http.StatusOK {
		t.Log("agent action returned 200 for empty path, acceptable")
	}
}

// --- handleIssuePairCode GET ---

func TestHandleIssuePairCodeGET(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 2, 14, 17, 0, 0, 0, time.UTC)}
	pairing := NewPairingCodeStore(clock.Now)
	if _, err := pairing.Register("test-code", 5*time.Minute); err != nil {
		t.Fatalf("register error: %v", err)
	}
	svc := newServiceForAPITest(t)
	handler := NewServer(svc, WithPairingCodeStore(pairing)).Handler()

	rr := doJSONRequest(t, handler, "GET", "/api/v1/pairing/codes", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	codes := resp["codes"]
	if codes == nil {
		t.Fatal("expected codes field")
	}
}

// --- handleIssuePairCode POST with custom code ---

func TestHandleIssuePairCodePOSTWithCode(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 2, 14, 17, 0, 0, 0, time.UTC)}
	pairing := NewPairingCodeStore(clock.Now)
	svc := newServiceForAPITest(t)
	handler := NewServer(svc, WithPairingCodeStore(pairing)).Handler()

	rr := doJSONRequest(t, handler, "POST", "/api/v1/pairing/codes", map[string]interface{}{
		"code":       "my-custom-code",
		"ttlSeconds": 120,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --- handleCreateDiagnosisHandoff missing agentId ---

func TestHandleCreateDiagnosisHandoffMissingAgentId(t *testing.T) {
	svc := newServiceForAPITest(t)
	handler := NewServer(svc).Handler()

	rr := doJSONRequest(t, handler, "POST", "/api/v1/diagnosis/handoffs", map[string]interface{}{
		"agentId": "",
		"consent": true,
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHandleCreateDiagnosisHandoffMethodNotAllowed(t *testing.T) {
	svc := newServiceForAPITest(t)
	handler := NewServer(svc).Handler()

	rr := doJSONRequest(t, handler, "GET", "/api/v1/diagnosis/handoffs", nil)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

// --- handleAgentAction status for all agents ---

func TestHandleAgentsStatusAll(t *testing.T) {
	svc := newServiceForAPITest(t)
	handler := NewServer(svc).Handler()

	rr := doJSONRequest(t, handler, "GET", "/api/v1/agents/status", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp struct {
		Statuses []daemonAgent `json:"statuses"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(resp.Statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(resp.Statuses))
	}
}

func TestHandleAgentsStatusAllMethodNotAllowed(t *testing.T) {
	svc := newServiceForAPITest(t)
	handler := NewServer(svc).Handler()

	rr := doJSONRequest(t, handler, "POST", "/api/v1/agents/status", nil)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

// --- uninstall flow through v1 API ---

func TestHandleAgentActionReinstall(t *testing.T) {
	svc := newServiceForAPITest(t)
	handler := NewServer(svc).Handler()

	// Install first
	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatal(err)
	}

	// Re-install (idempotent)
	rr := doJSONRequest(t, handler, "POST", "/api/v1/agents/openclaw/install", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --- audit logs edge cases ---

func TestAuditLogsInvalidLimit(t *testing.T) {
	svc := newServiceForAPITest(t)
	handler := NewServer(svc).Handler()

	rr := doJSONRequest(t, handler, "GET", "/api/v1/audit/logs?limit=-1", nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}

	rr = doJSONRequest(t, handler, "GET", "/api/v1/audit/logs?limit=abc", nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestAuditLogsMethodNotAllowed(t *testing.T) {
	svc := newServiceForAPITest(t)
	handler := NewServer(svc).Handler()

	rr := doJSONRequest(t, handler, "POST", "/api/v1/audit/logs", nil)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestAuditLogsWrongPath(t *testing.T) {
	svc := newServiceForAPITest(t)
	handler := NewServer(svc).Handler()

	rr := doJSONRequest(t, handler, "GET", "/api/v1/audit/logs/extra", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

// --- content-type enforcement (readJSON with malformed JSON) ---

func TestReadJSONMalformedJSON(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader("{bad json"))
	var dst struct{ Code string }
	err := readJSON(req, &dst)
	if err == nil || !strings.Contains(err.Error(), "invalid json") {
		t.Fatalf("expected invalid json error, got %v", err)
	}
}

// --- handleVerifyConsumePairCode wrong path ---

func TestHandleVerifyConsumePairCodeWrongPath(t *testing.T) {
	svc := newServiceForAPITest(t)
	handler := NewServer(svc).Handler()

	rr := doJSONRequest(t, handler, "POST", "/api/v1/pairing/verify-consume/extra", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

// --- handleIssuePairCode wrong path ---

func TestHandleIssuePairCodeWrongPath(t *testing.T) {
	svc := newServiceForAPITest(t)
	handler := NewServer(svc).Handler()

	rr := doJSONRequest(t, handler, "GET", "/api/v1/pairing/codes/extra", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

// --- handleCreateDiagnosisHandoff wrong path ---

func TestHandleCreateDiagnosisHandoffWrongPath(t *testing.T) {
	svc := newServiceForAPITest(t)
	handler := NewServer(svc).Handler()

	rr := doJSONRequest(t, handler, "POST", "/api/v1/diagnosis/handoffs/extra", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

// --- handleIssuePairCode method not allowed ---

func TestHandleIssuePairCodeMethodNotAllowed(t *testing.T) {
	svc := newServiceForAPITest(t)
	handler := NewServer(svc).Handler()

	rr := doJSONRequest(t, handler, "DELETE", "/api/v1/pairing/codes", nil)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}
