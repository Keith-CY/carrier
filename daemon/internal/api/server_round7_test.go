package api

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleListAgentsDirectGuards(t *testing.T) {
	svc := newServiceForAPITest(t)
	srv := NewServer(svc)

	methodRR := httptest.NewRecorder()
	methodReq := httptest.NewRequest(http.MethodPost, "/api/v1/agents", nil)
	srv.handleListAgents(methodRR, methodReq)
	if methodRR.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method guard status = %d, want 405; body=%s", methodRR.Code, methodRR.Body.String())
	}

	pathRR := httptest.NewRecorder()
	pathReq := httptest.NewRequest(http.MethodGet, "/api/v1/agents/extra", nil)
	srv.handleListAgents(pathRR, pathReq)
	if pathRR.Code != http.StatusNotFound {
		t.Fatalf("path guard status = %d, want 404; body=%s", pathRR.Code, pathRR.Body.String())
	}
}

func TestHandleMergedLogsTailClampAndPathGuard(t *testing.T) {
	svc := newServiceForAPITest(t)
	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}
	srv := NewServer(svc)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?tail=999999", nil)
	srv.handleMergedLogs(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Lines) == 0 {
		t.Fatal("expected merged logs to contain at least one line")
	}

	pathRR := httptest.NewRecorder()
	pathReq := httptest.NewRequest(http.MethodGet, "/api/v1/logs/extra", nil)
	srv.handleMergedLogs(pathRR, pathReq)
	if pathRR.Code != http.StatusNotFound {
		t.Fatalf("path guard status = %d, want 404; body=%s", pathRR.Code, pathRR.Body.String())
	}
}

func TestHandleAuditLogsAdditionalBranches(t *testing.T) {
	svc := newServiceForAPITest(t)
	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}
	srv := NewServer(svc)

	pathRR := httptest.NewRecorder()
	pathReq := httptest.NewRequest(http.MethodGet, "/api/v1/audit/logs/extra", nil)
	srv.handleAuditLogs(pathRR, pathReq)
	if pathRR.Code != http.StatusNotFound {
		t.Fatalf("path guard status = %d, want 404; body=%s", pathRR.Code, pathRR.Body.String())
	}

	for _, path := range []string{
		"/api/v1/audit/logs?result=success&limit=99999",
		"/api/v1/audit/logs?result=neutral",
		"/api/v1/audit/logs?actor=actor-not-found",
		"/api/v1/audit/logs?action=action-not-found",
	} {
		rr := doJSONRequest(t, srv.Handler(), http.MethodGet, path, nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %q status = %d, want 200; body=%s", path, rr.Code, rr.Body.String())
		}
	}
}

func TestHandleAgentActionAdditionalGuardsAndErrors(t *testing.T) {
	svc := newServiceForAPITest(t)
	handler := NewServer(svc).Handler()

	cases := []struct {
		method string
		path   string
		want   int
	}{
		{method: http.MethodGet, path: "/api/v1/agents/openclaw/install", want: http.StatusMethodNotAllowed},
		{method: http.MethodPost, path: "/api/v1/agents/missing/install", want: http.StatusNotFound},
		{method: http.MethodGet, path: "/api/v1/agents/openclaw/start", want: http.StatusMethodNotAllowed},
		{method: http.MethodPost, path: "/api/v1/agents/openclaw/start", want: http.StatusConflict},
		{method: http.MethodGet, path: "/api/v1/agents/openclaw/stop", want: http.StatusMethodNotAllowed},
		{method: http.MethodPost, path: "/api/v1/agents/openclaw/status", want: http.StatusMethodNotAllowed},
		{method: http.MethodGet, path: "/api/v1/agents/missing/status", want: http.StatusNotFound},
		{method: http.MethodPost, path: "/api/v1/agents/openclaw/logs", want: http.StatusMethodNotAllowed},
		{method: http.MethodGet, path: "/api/v1/agents/missing/logs", want: http.StatusNotFound},
		{method: http.MethodGet, path: "/api/v1/agents/openclaw/logs?tail=999999", want: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/agents/openclaw/diagnose", want: http.StatusMethodNotAllowed},
		{method: http.MethodPost, path: "/api/v1/agents/missing/diagnose", want: http.StatusNotFound},
		{method: http.MethodGet, path: "/api/v1/agents/openclaw/upgrade", want: http.StatusMethodNotAllowed},
		{method: http.MethodPost, path: "/api/v1/agents/openclaw/upgrade", want: http.StatusConflict},
		{method: http.MethodGet, path: "/api/v1/agents/openclaw/unknown", want: http.StatusNotFound},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rr := doJSONRequest(t, handler, tc.method, tc.path, nil)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d; body=%s", rr.Code, tc.want, rr.Body.String())
			}
		})
	}
}

type brokenEntropyReader struct{}

func (brokenEntropyReader) Read(_ []byte) (int, error) {
	return 0, errors.New("entropy unavailable")
}

func TestHandleIssuePairCodeDirectPathAndInternalError(t *testing.T) {
	svc := newServiceForAPITest(t)
	srv := NewServer(svc)

	pathRR := httptest.NewRecorder()
	pathReq := httptest.NewRequest(http.MethodPost, "/api/v1/pairing/codes/extra", strings.NewReader(`{"ttlSeconds":1}`))
	pathReq.Header.Set("Content-Type", "application/json")
	srv.handleIssuePairCode(pathRR, pathReq)
	if pathRR.Code != http.StatusNotFound {
		t.Fatalf("path guard status = %d, want 404; body=%s", pathRR.Code, pathRR.Body.String())
	}

	originalReader := rand.Reader
	rand.Reader = brokenEntropyReader{}
	defer func() {
		rand.Reader = originalReader
	}()

	rr := doJSONRequest(t, srv.Handler(), http.MethodPost, "/api/v1/pairing/codes", map[string]any{
		"ttlSeconds": 60,
	})
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", rr.Code, rr.Body.String())
	}

	var env errorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if env.Error.Code != "E_INTERNAL" {
		t.Fatalf("unexpected error code: %q", env.Error.Code)
	}
}

func TestHandleVerifyConsumePairCodeDirectGuards(t *testing.T) {
	svc := newServiceForAPITest(t)
	srv := NewServer(svc)

	methodRR := httptest.NewRecorder()
	methodReq := httptest.NewRequest(http.MethodGet, "/api/v1/pairing/verify-consume", nil)
	srv.handleVerifyConsumePairCode(methodRR, methodReq)
	if methodRR.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method guard status = %d, want 405; body=%s", methodRR.Code, methodRR.Body.String())
	}

	pathRR := httptest.NewRecorder()
	pathReq := httptest.NewRequest(http.MethodPost, "/api/v1/pairing/verify-consume/extra", strings.NewReader(`{"code":"x"}`))
	pathReq.Header.Set("Content-Type", "application/json")
	srv.handleVerifyConsumePairCode(pathRR, pathReq)
	if pathRR.Code != http.StatusNotFound {
		t.Fatalf("path guard status = %d, want 404; body=%s", pathRR.Code, pathRR.Body.String())
	}
}

func TestHandleCreateDiagnosisHandoffSuccessAndGuards(t *testing.T) {
	svc := newServiceForAPITest(t)
	if _, err := svc.HandleFailure(context.Background(), "openclaw", "startup failed"); err != nil {
		t.Fatalf("HandleFailure: %v", err)
	}
	srv := NewServer(svc)

	okResp := doJSONRequest(t, srv.Handler(), http.MethodPost, "/api/v1/diagnosis/handoffs", map[string]any{
		"agentId":   "openclaw",
		"consent":   true,
		"actor":     "telegram:100",
		"requestId": "req-handoff-success",
	})
	if okResp.Code != http.StatusOK {
		t.Fatalf("handoff status = %d, want 200; body=%s", okResp.Code, okResp.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(okResp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if strings.TrimSpace(payload["id"].(string)) == "" {
		t.Fatalf("expected non-empty handoff id, payload=%v", payload)
	}

	pathRR := httptest.NewRecorder()
	pathReq := httptest.NewRequest(http.MethodPost, "/api/v1/diagnosis/handoffs/extra", nil)
	srv.handleCreateDiagnosisHandoff(pathRR, pathReq)
	if pathRR.Code != http.StatusNotFound {
		t.Fatalf("path guard status = %d, want 404; body=%s", pathRR.Code, pathRR.Body.String())
	}

	invalidRR := doRawJSONRequest(t, srv.Handler(), http.MethodPost, "/api/v1/diagnosis/handoffs", []byte(`{bad json`))
	if invalidRR.Code != http.StatusBadRequest {
		t.Fatalf("invalid json status = %d, want 400; body=%s", invalidRR.Code, invalidRR.Body.String())
	}
}

func TestParseAgentActionPathAdditionalRejects(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "missing prefix", path: "/api/v1/agent/openclaw/start"},
		{name: "double slash in tail", path: "/api/v1/agents/openclaw//start"},
		{name: "bad escaped agent id", path: "/api/v1/agents/%zz/start"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if _, _, ok := parseAgentActionPath(tc.path); ok {
				t.Fatalf("expected parseAgentActionPath(%q) to fail", tc.path)
			}
		})
	}
}

type errReadCloser struct{}

func (errReadCloser) Read(_ []byte) (int, error) {
	return 0, errors.New("forced read error")
}

func (errReadCloser) Close() error {
	return nil
}

func TestReadJSONReadFailureAndChunkedOversize(t *testing.T) {
	readErrReq := httptest.NewRequest(http.MethodPost, "/", nil)
	readErrReq.Body = errReadCloser{}
	readErrReq.ContentLength = -1
	var first struct {
		Code string `json:"code"`
	}
	if err := readJSON(readErrReq, &first); err == nil || !strings.Contains(err.Error(), "read request body") {
		t.Fatalf("expected read failure, got %v", err)
	}

	chunkedReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"code":"`+strings.Repeat("x", (1<<20)+64)+`"}`))
	chunkedReq.ContentLength = -1
	var second struct {
		Code string `json:"code"`
	}
	if err := readJSON(chunkedReq, &second); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("expected chunked too-large error, got %v", err)
	}
}
