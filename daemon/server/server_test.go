package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"carrier/baseagent"
	"carrier/daemon/internal/api"
	"carrier/daemon/internal/lifecycle"
	"carrier/daemon/internal/manifest"
	"carrier/daemon/internal/ratelimit"
)

func newTestService() *lifecycle.Service {
	return lifecycle.NewService(baseagent.NoopTriager{})
}

func newTestServiceWithAgent(t *testing.T) *lifecycle.Service {
	t.Helper()
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	m := manifest.Manifest{
		ID:      "test-agent",
		Name:    "Test Agent",
		Version: "1.0.0",
		Runtime: manifest.RuntimeSpec{
			Type:    manifest.RuntimeTypeLocalBinary,
			Install: manifest.CommandSpec{Command: "echo installed"},
			Start:   manifest.CommandSpec{Command: "echo started"},
			Stop:    manifest.CommandSpec{Command: "echo stopped"},
		},
		Memory: manifest.MemorySpec{
			Supports:  []manifest.MemoryType{manifest.MemoryTypePerAgent},
			MountPath: "/tmp/test-agent-memory",
		},
	}
	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("register manifest: %v", err)
	}
	return svc
}

func newTestMux() *http.ServeMux {
	svc := newTestService()
	ready := &atomic.Bool{}
	ready.Store(true)
	pairStore := api.NewPairingCodeStore(nil)
	pairLimiter := ratelimit.New(ratelimit.WithMax(10), ratelimit.WithWindow(1*time.Minute))
	return buildHTTPMux(svc, ready, pairStore, pairLimiter)
}

func TestHealthz(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected ok, got %q", body["status"])
	}
}

func TestReadyz_Ready(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestReadyz_NotReady(t *testing.T) {
	svc := newTestService()
	ready := &atomic.Bool{}
	ready.Store(false)
	pairStore := api.NewPairingCodeStore(nil)
	mux := buildHTTPMux(svc, ready, pairStore, nil)

	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestListAgents(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest("GET", "/api/agents", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestListAgents_V1(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest("GET", "/api/v1/agents/status", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestListAgents_MethodNotAllowed(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest("POST", "/api/agents", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestInstall_MissingAgentID(t *testing.T) {
	mux := newTestMux()
	body := `{"agentId":""}`
	req := httptest.NewRequest("POST", "/api/install", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestInstall_MethodNotAllowed(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest("GET", "/api/install", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestStart_MethodNotAllowed(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest("GET", "/api/start", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestStop_MethodNotAllowed(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest("GET", "/api/stop", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestPairingCodes_List(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest("GET", "/api/pairing/codes", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestPairingVerifyConsume_MethodNotAllowed(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest("GET", "/api/pairing/verify-consume", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestPairingVerifyConsume_InvalidCode(t *testing.T) {
	mux := newTestMux()
	body := `{"code":"invalid-code"}`
	req := httptest.NewRequest("POST", "/api/pairing/verify-consume", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestPairingVerifyConsume_Success(t *testing.T) {
	svc := newTestService()
	ready := &atomic.Bool{}
	ready.Store(true)
	pairStore := api.NewPairingCodeStore(nil)
	record, _ := pairStore.Issue(5 * time.Minute)
	pairLimiter := ratelimit.New(ratelimit.WithMax(10), ratelimit.WithWindow(1*time.Minute))
	mux := buildHTTPMux(svc, ready, pairStore, pairLimiter)

	body := `{"code":"` + record.Code + `"}`
	req := httptest.NewRequest("POST", "/api/pairing/verify-consume", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestDiagnosisHandoffs_MethodNotAllowed(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest("GET", "/api/v1/diagnosis/handoffs", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestDiagnosisHandoffs_MissingAgentID(t *testing.T) {
	mux := newTestMux()
	body := `{"agentId":"","consent":true}`
	req := httptest.NewRequest("POST", "/api/v1/diagnosis/handoffs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestV1AgentAction_NotFound(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest("GET", "/api/v1/agents/myagent/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestV1AgentAction_InvalidPath(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest("GET", "/api/v1/agents/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// Test helper functions

func TestValidateAgentID(t *testing.T) {
	tests := []struct {
		id      string
		wantErr bool
	}{
		{"valid-agent", false},
		{"agent.v1", false},
		{"agent_test", false},
		{"", true},
		{"../bad", true},
		{"a/b", true},
		{"a\\b", true},
		{".invalid", true},
		{"-invalid", true},
	}
	for _, tc := range tests {
		err := validateAgentID(tc.id)
		if tc.wantErr && err == nil {
			t.Errorf("validateAgentID(%q): expected error", tc.id)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("validateAgentID(%q): unexpected error: %v", tc.id, err)
		}
	}
}

func TestParseAgentActionPath(t *testing.T) {
	tests := []struct {
		path   string
		agent  string
		action string
		wantOK bool
	}{
		{"/api/v1/agents/myagent/start", "myagent", "start", true},
		{"/api/v1/agents/agent.v1/stop", "agent.v1", "stop", true},
		{"/api/v1/agents/", "", "", false},
		{"/api/v1/agents/a", "", "", false},        // no action
		{"/api/v1/agents/a/b/c", "", "", false},    // too many parts
		{"/api/v1/agents//start", "", "", false},   // double slash
		{"/api/v1/agents/../start", "", "", false}, // path traversal
		{"/other/path", "", "", false},
	}
	for _, tc := range tests {
		agent, action, ok := parseAgentActionPath(tc.path)
		if ok != tc.wantOK {
			t.Errorf("parseAgentActionPath(%q): ok=%v, want %v", tc.path, ok, tc.wantOK)
			continue
		}
		if ok {
			if agent != tc.agent {
				t.Errorf("parseAgentActionPath(%q): agent=%q, want %q", tc.path, agent, tc.agent)
			}
			if action != tc.action {
				t.Errorf("parseAgentActionPath(%q): action=%q, want %q", tc.path, action, tc.action)
			}
		}
	}
}

func TestParseLogsTail(t *testing.T) {
	tests := []struct {
		raw  string
		want int
	}{
		{"", defaultLogsTail},
		{"100", 100},
		{"abc", defaultLogsTail},
		{"0", defaultLogsTail},
		{"-1", defaultLogsTail},
		{"99999", maxLogsTail},
	}
	for _, tc := range tests {
		got := parseLogsTail(tc.raw)
		if got != tc.want {
			t.Errorf("parseLogsTail(%q) = %d, want %d", tc.raw, got, tc.want)
		}
	}
}

func TestParsePathAgentID(t *testing.T) {
	tests := []struct {
		raw     string
		wantErr bool
	}{
		{"", true},
		{"valid-agent", false},
		{"../bad", true},
		{"a%2fb", true}, // contains /
	}
	for _, tc := range tests {
		_, err := parsePathAgentID(tc.raw)
		if tc.wantErr && err == nil {
			t.Errorf("parsePathAgentID(%q): expected error", tc.raw)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("parsePathAgentID(%q): unexpected error: %v", tc.raw, err)
		}
	}
}

func TestTrimPathByPrefixes(t *testing.T) {
	got := trimPathByPrefixes("/api/logs/myagent", "/api/logs/", "/api/v1/logs/")
	if got != "myagent" {
		t.Errorf("expected myagent, got %q", got)
	}
	got = trimPathByPrefixes("/other/path", "/api/logs/")
	if got != "/other/path" {
		t.Errorf("expected unchanged, got %q", got)
	}
}

func TestBearerAuthMiddleware(t *testing.T) {
	inner := http.NewServeMux()
	inner.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	inner.HandleFunc("/api/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	inner.HandleFunc("/other", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := bearerAuthMiddleware("secret", inner)

	// /api/ without token → 401
	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("/api/test no token: expected 401, got %d", w.Code)
	}

	// /api/ with correct token → 200
	req = httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("/api/test with token: expected 200, got %d", w.Code)
	}

	// /api/ with X-Gateway-Token → 200
	req = httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Gateway-Token", "secret")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("/api/test with X-Gateway-Token: expected 200, got %d", w.Code)
	}

	// /other doesn't need auth
	req = httptest.NewRequest("GET", "/other", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("/other: expected 200, got %d", w.Code)
	}

	// /healthz requires auth when token is set
	req = httptest.NewRequest("GET", "/healthz", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("/healthz no token: expected 401, got %d", w.Code)
	}
}

func TestDecodeBody(t *testing.T) {
	// Valid body
	body := `{"agentId":"myagent"}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	var result agentIDBody
	ok := decodeBody(w, req, &result)
	if !ok {
		t.Fatal("expected ok")
	}
	if result.AgentID != "myagent" {
		t.Errorf("expected myagent, got %q", result.AgentID)
	}

	// Empty body
	req = httptest.NewRequest("POST", "/", strings.NewReader(""))
	w = httptest.NewRecorder()
	ok = decodeBody(w, req, &result)
	if ok {
		t.Error("expected fail for empty body")
	}

	// Invalid JSON
	req = httptest.NewRequest("POST", "/", strings.NewReader("{invalid"))
	w = httptest.NewRecorder()
	ok = decodeBody(w, req, &result)
	if ok {
		t.Error("expected fail for invalid JSON")
	}

	// Too large content length
	req = httptest.NewRequest("POST", "/", strings.NewReader("x"))
	req.ContentLength = maxBodySize + 1
	w = httptest.NewRecorder()
	ok = decodeBody(w, req, &result)
	if ok {
		t.Error("expected fail for too large body")
	}
}

func TestDecodeBody_TrailingData(t *testing.T) {
	body := `{"agentId":"a1"}{"extra":"data"}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	w := httptest.NewRecorder()
	var result agentIDBody
	ok := decodeBody(w, req, &result)
	if ok {
		t.Error("expected fail for trailing data")
	}
}

func TestDecodeBody_TooLargeActual(t *testing.T) {
	big := `{"agentId":"` + strings.Repeat("a", maxBodySize+10) + `"}`
	req := httptest.NewRequest("POST", "/", bytes.NewReader([]byte(big)))
	w := httptest.NewRecorder()
	var result agentIDBody
	ok := decodeBody(w, req, &result)
	if ok {
		t.Error("expected fail for actually too large body")
	}
}

func TestRemoteIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	if got := remoteIP(req); got != "192.168.1.1" {
		t.Errorf("expected 192.168.1.1, got %q", got)
	}

	req.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2")
	if got := remoteIP(req); got != "10.0.0.1" {
		t.Errorf("expected 10.0.0.1, got %q", got)
	}
}

func TestIsLoopback(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"", true},
		{"localhost", true},
		{"127.0.0.1", true},
		{"::1", true},
		{"0.0.0.0", false},
		{"192.168.1.1", false},
	}
	for _, tc := range tests {
		got := isLoopback(tc.host)
		if got != tc.want {
			t.Errorf("isLoopback(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

func TestWriteJSON_Server(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusCreated, map[string]string{"k": "v"})
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
}

func TestWriteJSONError(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSONError(w, http.StatusBadRequest, "bad request")
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "bad request" {
		t.Errorf("expected 'bad request', got %q", body["error"])
	}
}

func TestNewHTTPServer(t *testing.T) {
	srv := newHTTPServer(":0", http.NewServeMux())
	if srv.ReadHeaderTimeout != defaultReadHeaderTimeout {
		t.Errorf("unexpected ReadHeaderTimeout: %v", srv.ReadHeaderTimeout)
	}
	if srv.WriteTimeout != defaultWriteTimeout {
		t.Errorf("unexpected WriteTimeout: %v", srv.WriteTimeout)
	}
	if srv.IdleTimeout != defaultIdleTimeout {
		t.Errorf("unexpected IdleTimeout: %v", srv.IdleTimeout)
	}
}

func newTestMuxWithAgent(t *testing.T) *http.ServeMux {
	t.Helper()
	svc := newTestServiceWithAgent(t)
	ready := &atomic.Bool{}
	ready.Store(true)
	pairStore := api.NewPairingCodeStore(nil)
	pairLimiter := ratelimit.New(ratelimit.WithMax(10), ratelimit.WithWindow(1*time.Minute))
	return buildHTTPMux(svc, ready, pairStore, pairLimiter)
}

func TestV1AgentInstall(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	body := `{"agentId":"test-agent"}`
	req := httptest.NewRequest("POST", "/api/v1/agents/test-agent/install", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestV1AgentStatus(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	req := httptest.NewRequest("GET", "/api/v1/agents/test-agent/status", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestV1AgentStart_NotInstalled(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	body := `{"agentId":"test-agent"}`
	req := httptest.NewRequest("POST", "/api/v1/agents/test-agent/start", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Agent not installed yet → conflict
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestV1AgentStop_NotRunning(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	body := `{"agentId":"test-agent"}`
	req := httptest.NewRequest("POST", "/api/v1/agents/test-agent/stop", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Not running → conflict
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestV1AgentLogs(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	req := httptest.NewRequest("GET", "/api/v1/agents/test-agent/logs", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Should succeed or return empty logs
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestV1AgentUpgrade_NotSupported(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	body := `{"agentId":"test-agent"}`
	req := httptest.NewRequest("POST", "/api/v1/agents/test-agent/upgrade", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Agent not installed → conflict
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestV1AgentDiagnose(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	body := `{"agentId":"test-agent"}`
	req := httptest.NewRequest("POST", "/api/v1/agents/test-agent/diagnose", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestV1AgentUninstall(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	body := `{"agentId":"test-agent"}`
	req := httptest.NewRequest("POST", "/api/v1/agents/test-agent/uninstall", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestV1AgentInstall_MethodNotAllowed(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	req := httptest.NewRequest("GET", "/api/v1/agents/test-agent/install", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestApiStatus_WithAgentID(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	req := httptest.NewRequest("GET", "/api/status/test-agent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestApiLogs_WithAgentID(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	req := httptest.NewRequest("GET", "/api/logs/test-agent?tail=50", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestApiInstall_WithBody(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	body := `{"agentId":"test-agent"}`
	req := httptest.NewRequest("POST", "/api/install", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestApiStart_NotInstalled(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	body := `{"agentId":"test-agent"}`
	req := httptest.NewRequest("POST", "/api/start", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestApiStop_NotRunning(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	body := `{"agentId":"test-agent"}`
	req := httptest.NewRequest("POST", "/api/stop", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Not running → conflict
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestApiUpgrade_WithBody(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	body := `{"agentId":"test-agent"}`
	req := httptest.NewRequest("POST", "/api/upgrade", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Agent not installed → conflict
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestApiDiagnose_WithBody(t *testing.T) {
	mux := newTestMuxWithAgent(t)
	body := `{"agentId":"test-agent"}`
	req := httptest.NewRequest("POST", "/api/diagnose", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestV1AgentStatus_NotFound(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest("GET", "/api/v1/agents/nonexistent/status", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestWriteServiceError(t *testing.T) {
	tests := []struct {
		err    error
		status int
	}{
		{lifecycle.ErrAgentNotFound, http.StatusNotFound},
		{lifecycle.ErrNotInstalled, http.StatusConflict},
		{lifecycle.ErrAlreadyRunning, http.StatusConflict},
		{lifecycle.ErrAlreadyStopped, http.StatusConflict},
		{lifecycle.ErrCrashLoop, http.StatusConflict},
		{lifecycle.ErrAgentRunning, http.StatusConflict},
		{lifecycle.ErrUpgradeNotSupported, http.StatusBadRequest},
		{lifecycle.ErrIsolationUnavailable, http.StatusUnprocessableEntity},
		{lifecycle.ErrIsolationStartFailed, http.StatusBadGateway},
	}
	for _, tc := range tests {
		w := httptest.NewRecorder()
		writeServiceError(w, tc.err)
		if w.Code != tc.status {
			t.Errorf("writeServiceError(%v): expected %d, got %d", tc.err, tc.status, w.Code)
		}
	}
}
