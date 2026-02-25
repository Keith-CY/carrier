package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"carrier/baseagent"
	"carrier/daemon/internal/api"
	"carrier/daemon/internal/catalog"
	"carrier/daemon/internal/commandexec"
	"carrier/daemon/internal/lifecycle"
	"carrier/daemon/internal/manifest"
	"carrier/daemon/internal/ratelimit"
	"carrier/daemon/internal/runtimecheck"
)

type noopRunner struct{}

func (noopRunner) Run(_ context.Context, _ string) (commandexec.Result, error) {
	return commandexec.Result{ExitCode: 0, CombinedOutput: "ok"}, nil
}

type noopChecker struct{}

func (noopChecker) Check(_ manifest.Manifest) error { return nil }

var _ runtimecheck.Checker = noopChecker{}

type fakeProcessManager struct {
	mu      sync.Mutex
	running map[string]bool
	stopped map[string]chan struct{}
}

func newFakeProcessManager() *fakeProcessManager {
	return &fakeProcessManager{
		running: map[string]bool{},
		stopped: map[string]chan struct{}{},
	}
}

var _ lifecycle.ProcessController = (*fakeProcessManager)(nil)

func buildTestMux(svc *lifecycle.Service, ready bool) *http.ServeMux {
	var readyFlag atomic.Bool
	readyFlag.Store(ready)
	pairStore := api.NewPairingCodeStore(nil)
	return buildHTTPMux(svc, &readyFlag, pairStore, ratelimit.New())
}

func TestNewHTTPServerAppliesTimeouts(t *testing.T) {
	handler := http.NewServeMux()
	server := newHTTPServer("127.0.0.1:9090", handler)

	if server.ReadHeaderTimeout != defaultReadHeaderTimeout {
		t.Fatalf("ReadHeaderTimeout = %v, want %v", server.ReadHeaderTimeout, defaultReadHeaderTimeout)
	}
	if server.ReadTimeout != defaultReadTimeout {
		t.Fatalf("ReadTimeout = %v, want %v", server.ReadTimeout, defaultReadTimeout)
	}
	if server.WriteTimeout != defaultWriteTimeout {
		t.Fatalf("WriteTimeout = %v, want %v", server.WriteTimeout, defaultWriteTimeout)
	}
	if server.IdleTimeout != defaultIdleTimeout {
		t.Fatalf("IdleTimeout = %v, want %v", server.IdleTimeout, defaultIdleTimeout)
	}
	if server.Handler != handler {
		t.Fatal("handler mismatch")
	}
}

func (m *fakeProcessManager) Start(agentID string, _ string, _ []string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running[agentID] {
		return 0, fmt.Errorf("agent %s already running", agentID)
	}
	m.running[agentID] = true
	m.stopped[agentID] = make(chan struct{})
	return 12345, nil
}

func (m *fakeProcessManager) Stop(agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running[agentID] {
		return fmt.Errorf("agent %s is not running", agentID)
	}

	m.running[agentID] = false
	close(m.stopped[agentID])
	return nil
}

func (m *fakeProcessManager) IsRunning(agentID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running[agentID]
}

func (m *fakeProcessManager) Wait(agentID string) error {
	m.mu.Lock()
	ch, exists := m.stopped[agentID]
	running := m.running[agentID]
	m.mu.Unlock()

	if !exists || !running {
		return fmt.Errorf("agent %s is not running", agentID)
	}
	<-ch
	return nil
}

func (m *fakeProcessManager) Cleanup() {
	m.mu.Lock()
	agents := make([]string, 0, len(m.running))
	for agentID := range m.running {
		agents = append(agents, agentID)
	}
	m.mu.Unlock()

	for _, agentID := range agents {
		_ = m.Stop(agentID)
	}
}

func TestStopAllAgents_NoRunning(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}
	if err := stopAllAgents(context.Background(), svc); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestShutdownAgents_Timeout(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	err := shutdownAgents(svc, 5*time.Second)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestShutdownAgents_VeryShortTimeout(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	err := shutdownAgents(svc, 1*time.Millisecond)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestHealthz(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	mux := buildTestMux(svc, true)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestReadyz(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	mux := buildTestMux(svc, true)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestReadyzNotReady(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	mux := buildTestMux(svc, false)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestAPIListAgents(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}
	mux := buildTestMux(svc, true)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/agents", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var agents []interface{}
	if err := json.NewDecoder(rec.Body).Decode(&agents); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(agents) == 0 {
		t.Fatal("expected at least one agent")
	}
}

func TestAPIStatusNotFound(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	mux := buildTestMux(svc, true)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/status/nonexistent", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestAPIListAgentsV1(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}
	mux := buildTestMux(svc, true)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/agents", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestAPIStatusAndLogsV1Aliases(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}
	mux := buildTestMux(svc, true)

	statusRec := httptest.NewRecorder()
	mux.ServeHTTP(statusRec, httptest.NewRequest("GET", "/api/v1/status/openclaw", nil))
	if statusRec.Code != http.StatusOK {
		t.Fatalf("v1 status: expected 200, got %d: %s", statusRec.Code, statusRec.Body.String())
	}

	logsRec := httptest.NewRecorder()
	mux.ServeHTTP(logsRec, httptest.NewRequest("GET", "/api/v1/logs/openclaw", nil))
	if logsRec.Code != http.StatusOK {
		t.Fatalf("v1 logs: expected 200, got %d: %s", logsRec.Code, logsRec.Body.String())
	}
}

func TestAPIStatusAndLogsV1AliasesInvalidAgentID(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}
	mux := buildTestMux(svc, true)

	statusRec := httptest.NewRecorder()
	mux.ServeHTTP(statusRec, httptest.NewRequest("GET", "/api/v1/status/bad/id", nil))
	if statusRec.Code != http.StatusBadRequest {
		t.Fatalf("v1 status invalid id: expected 400, got %d", statusRec.Code)
	}

	logsRec := httptest.NewRecorder()
	mux.ServeHTTP(logsRec, httptest.NewRequest("GET", "/api/v1/logs/bad/id", nil))
	if logsRec.Code != http.StatusBadRequest {
		t.Fatalf("v1 logs invalid id: expected 400, got %d", logsRec.Code)
	}
}

func TestAPIInstallAndStart(t *testing.T) {
	svc := lifecycle.NewService(
		baseagent.NoopTriager{},
		lifecycle.WithRunner(noopRunner{}),
		lifecycle.WithRuntimeChecker(noopChecker{}),
		lifecycle.WithProcessManager(newFakeProcessManager()),
	)
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}
	mux := buildTestMux(svc, true)

	// Set required env
	t.Setenv("OPENAI_API_KEY", "test-key")

	// Install
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/install", strings.NewReader(`{"agentId":"openclaw"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("install: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Start
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/start", strings.NewReader(`{"agentId":"openclaw"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("start: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Stop
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/stop", strings.NewReader(`{"agentId":"openclaw"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("stop: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAPIMethodNotAllowed(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	mux := buildTestMux(svc, true)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/agents", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestWriteServiceErrorMapsWrappedErrors(t *testing.T) {
	rec := httptest.NewRecorder()
	writeServiceError(rec, fmt.Errorf("wrap: %w", lifecycle.ErrNotInstalled))
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

func TestParseLogsTail(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want int
	}{
		{name: "default empty", raw: "", want: defaultLogsTail},
		{name: "invalid", raw: "abc", want: defaultLogsTail},
		{name: "zero", raw: "0", want: defaultLogsTail},
		{name: "negative", raw: "-5", want: defaultLogsTail},
		{name: "valid", raw: "50", want: 50},
		{name: "clamped", raw: "999999", want: maxLogsTail},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLogsTail(tt.raw)
			if got != tt.want {
				t.Fatalf("parseLogsTail(%q)=%d, want %d", tt.raw, got, tt.want)
			}
		})
	}
}

func TestValidateAgentID(t *testing.T) {
	tests := []struct {
		id      string
		wantErr bool
	}{
		{"my-agent", false},
		{"agent_1.0", false},
		{"a", false},
		{"", true},
		{"../etc/passwd", true},
		{"foo/bar", true},
		{"foo\\bar", true},
		{"a..b", true},
		{".hidden", true},
		{"-start", true},
	}
	for _, tt := range tests {
		err := validateAgentID(tt.id)
		if (err != nil) != tt.wantErr {
			t.Errorf("validateAgentID(%q) err=%v, wantErr=%v", tt.id, err, tt.wantErr)
		}
	}
}

func TestPairingCodesEndpoint(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	mux := buildTestMux(svc, true)

	// Test GET /api/pairing/codes
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/pairing/codes", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/pairing/codes: expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := resp["codes"]; !ok {
		t.Fatal("expected 'codes' field in response")
	}

	// Test GET /api/v1/pairing/codes
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/pairing/codes", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/pairing/codes: expected 200, got %d", rec.Code)
	}
}

func TestPairingVerifyConsumeEndpoint(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	pairStore := api.NewPairingCodeStore(nil)

	// Issue a pairing code
	record, err := pairStore.Issue(5 * time.Minute)
	if err != nil {
		t.Fatalf("issue pairing code: %v", err)
	}

	var readyFlag atomic.Bool
	readyFlag.Store(true)
	mux := buildHTTPMux(svc, &readyFlag, pairStore, ratelimit.New())

	// Test POST /api/pairing/verify-consume with valid code
	body := fmt.Sprintf(`{"code":"%s"}`, record.Code)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/pairing/verify-consume", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/pairing/verify-consume: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["consumed"] != true {
		t.Fatal("expected 'consumed' to be true")
	}

	// Test with invalid code (already consumed)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/pairing/verify-consume", strings.NewReader(body)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/pairing/verify-consume (consumed): expected 400, got %d", rec.Code)
	}

	// Test POST /api/v1/pairing/verify-consume
	record2, _ := pairStore.Issue(5 * time.Minute)
	body2 := fmt.Sprintf(`{"code":"%s"}`, record2.Code)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/pairing/verify-consume", strings.NewReader(body2)))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/pairing/verify-consume: expected 200, got %d", rec.Code)
	}
}

func TestPairingMethodNotAllowed(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	mux := buildTestMux(svc, true)

	// Test POST on codes endpoint (only GET allowed)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/pairing/codes", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/pairing/codes: expected 405, got %d", rec.Code)
	}

	// Test GET on verify-consume endpoint (only POST allowed)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/pairing/verify-consume", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /api/pairing/verify-consume: expected 405, got %d", rec.Code)
	}
}

func TestDecodeBodyMalformed(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}
	mux := buildTestMux(svc, true)

	// We POST to /api/install which calls decodeBody then validateAgentID.
	// For "valid" JSON the decode succeeds but install may fail downstream,
	// so we only assert that bad bodies get 400 and valid ones do NOT get 400.
	tests := []struct {
		name    string
		body    string
		want400 bool
	}{
		{
			name:    "valid baseline",
			body:    `{"agentId":"openclaw"}`,
			want400: false,
		},
		{
			name:    "unknown field rejected",
			body:    `{"agentId":"openclaw","extra":"bad"}`,
			want400: true,
		},
		{
			name:    "malformed JSON",
			body:    `{agentId: openclaw}`,
			want400: true,
		},
		{
			name:    "trailing JSON object",
			body:    `{"agentId":"openclaw"}{"agentId":"openclaw"}`,
			want400: true,
		},
		{
			name:    "trailing JSON primitive",
			body:    `{"agentId":"openclaw"} 123`,
			want400: true,
		},
		{
			name:    "empty body",
			body:    ``,
			want400: true,
		},
		{
			name:    "whitespace body",
			body:    "   \n\t  ",
			want400: true,
		},
		{
			name:    "oversized body",
			body:    `{"agentId":"` + strings.Repeat("a", 1<<20+100) + `"}`,
			want400: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/api/install", strings.NewReader(tt.body))
			mux.ServeHTTP(rec, req)
			got400 := rec.Code == http.StatusBadRequest
			if got400 != tt.want400 {
				t.Fatalf("status=%d, want400=%v; body=%s", rec.Code, tt.want400, rec.Body.String())
			}
		})
	}
}

func TestDecodeBodyRejectsOversizedContentLengthHeader(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}
	mux := buildTestMux(svc, true)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/install", strings.NewReader(`{"agentId":"openclaw"}`))
	req.ContentLength = maxBodySize + 1

	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want=400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestPairingVerifyRateLimit(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}
	pairStore := api.NewPairingCodeStore(nil)

	var readyFlag atomic.Bool
	readyFlag.Store(true)
	limiter := ratelimit.New(ratelimit.WithMax(3), ratelimit.WithWindow(1*time.Minute))
	mux := buildHTTPMux(svc, &readyFlag, pairStore, limiter)

	// Make 3 requests (all allowed, even if codes are wrong)
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/pairing/verify-consume", strings.NewReader(`{"code":"badcode1"}`))
		req.RemoteAddr = "1.2.3.4:12345"
		mux.ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d should not be rate-limited", i+1)
		}
	}

	// 4th request should be rate-limited
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/pairing/verify-consume", strings.NewReader(`{"code":"badcode1"}`))
	req.RemoteAddr = "1.2.3.4:12345"
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") != "60" {
		t.Fatal("expected Retry-After header")
	}

	// Different IP should still be allowed
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/pairing/verify-consume", strings.NewReader(`{"code":"badcode1"}`))
	req.RemoteAddr = "5.6.7.8:12345"
	mux.ServeHTTP(rec, req)
	if rec.Code == http.StatusTooManyRequests {
		t.Fatal("different IP should not be rate-limited")
	}
}

func TestBearerAuthMiddlewareBoundaryCases(t *testing.T) {
	token := "secret-token"
	base := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := bearerAuthMiddleware(token, base)

	tests := []struct {
		name string
		path string
		auth string
		want int
	}{
		{name: "api missing auth", path: "/api/v1/agents", auth: "", want: http.StatusUnauthorized},
		{name: "api wrong scheme", path: "/api/v1/agents", auth: "Token secret-token", want: http.StatusUnauthorized},
		{name: "api near miss token", path: "/api/v1/agents", auth: "Bearer secret-token ", want: http.StatusUnauthorized},
		{name: "api length mismatch token", path: "/api/v1/agents", auth: "Bearer secret-token-x", want: http.StatusUnauthorized},
		{name: "api correct token", path: "/api/v1/agents", auth: "Bearer secret-token", want: http.StatusOK},
		{name: "healthz exempt", path: "/healthz", auth: "", want: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.auth != "" {
				req.Header.Set("Authorization", tt.auth)
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != tt.want {
				t.Fatalf("status=%d, want=%d; body=%s", rr.Code, tt.want, rr.Body.String())
			}
		})
	}
}

func TestDecodeBody_EmptyBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(""))
	rr := httptest.NewRecorder()
	var out struct{ Name string }
	ok := decodeBody(rr, req, &out)
	if ok {
		t.Fatal("expected false for empty body")
	}
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestDecodeBody_InvalidJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader("{invalid"))
	rr := httptest.NewRecorder()
	var out struct{ Name string }
	ok := decodeBody(rr, req, &out)
	if ok {
		t.Fatal("expected false for invalid JSON")
	}
}

func TestDecodeBody_OversizedContentLength(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader("{}"))
	req.ContentLength = maxBodySize + 1
	rr := httptest.NewRecorder()
	var out struct{ Name string }
	ok := decodeBody(rr, req, &out)
	if ok {
		t.Fatal("expected false for oversized content-length")
	}
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestDecodeBody_TrailingData(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{}{}`))
	rr := httptest.NewRecorder()
	var out struct{ Name string }
	ok := decodeBody(rr, req, &out)
	if ok {
		t.Fatal("expected false for trailing data")
	}
}

func TestHandleUpgrade_NotInstalled(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}
	mux := buildTestMux(svc, false)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/openclaw/upgrade", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 for upgrade of uninstalled agent, got %d", rr.Code)
	}
}

type alwaysFailRunner struct{}

func (alwaysFailRunner) Run(_ context.Context, _ string) (commandexec.Result, error) {
	return commandexec.Result{ExitCode: 1, CombinedOutput: "forced failure"}, errors.New("forced runner failure")
}

type errBodyReadCloser struct{}

func (errBodyReadCloser) Read(_ []byte) (int, error) {
	return 0, errors.New("forced read failure")
}

func (errBodyReadCloser) Close() error { return nil }

type failingStopProcessManager struct {
	mu        sync.Mutex
	running   map[string]bool
	waiters   map[string]chan struct{}
	stopCalls int
	cleaned   bool
}

func newFailingStopProcessManager() *failingStopProcessManager {
	return &failingStopProcessManager{
		running: make(map[string]bool),
		waiters: make(map[string]chan struct{}),
	}
}

func (m *failingStopProcessManager) Start(agentID string, _ string, _ []string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running[agentID] = true
	m.waiters[agentID] = make(chan struct{})
	return 1, nil
}

func (m *failingStopProcessManager) Stop(_ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopCalls++
	return errors.New("forced stop failure")
}

func (m *failingStopProcessManager) IsRunning(agentID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running[agentID]
}

func (m *failingStopProcessManager) Wait(agentID string) error {
	m.mu.Lock()
	waitCh := m.waiters[agentID]
	m.mu.Unlock()
	if waitCh != nil {
		<-waitCh
	}
	return nil
}

func (m *failingStopProcessManager) Cleanup() {
	m.mu.Lock()
	for id, waitCh := range m.waiters {
		delete(m.waiters, id)
		delete(m.running, id)
		close(waitCh)
	}
	m.cleaned = true
	m.mu.Unlock()
}

type blockingStopProcessManager struct {
	mu      sync.Mutex
	running map[string]bool
	waiters map[string]chan struct{}
	started chan struct{}
	release chan struct{}
}

func newBlockingStopProcessManager() *blockingStopProcessManager {
	return &blockingStopProcessManager{
		running: make(map[string]bool),
		waiters: make(map[string]chan struct{}),
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (m *blockingStopProcessManager) Start(agentID string, _ string, _ []string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running[agentID] = true
	m.waiters[agentID] = make(chan struct{})
	return 1, nil
}

func (m *blockingStopProcessManager) Stop(agentID string) error {
	select {
	case <-m.started:
	default:
		close(m.started)
	}
	<-m.release
	m.mu.Lock()
	if waitCh := m.waiters[agentID]; waitCh != nil {
		close(waitCh)
		delete(m.waiters, agentID)
	}
	delete(m.running, agentID)
	m.mu.Unlock()
	return nil
}

func (m *blockingStopProcessManager) IsRunning(agentID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running[agentID]
}

func (m *blockingStopProcessManager) Wait(agentID string) error {
	m.mu.Lock()
	waitCh := m.waiters[agentID]
	m.mu.Unlock()
	if waitCh != nil {
		<-waitCh
	}
	return nil
}

func (m *blockingStopProcessManager) Cleanup() {
	m.mu.Lock()
	for id, waitCh := range m.waiters {
		delete(m.waiters, id)
		delete(m.running, id)
		close(waitCh)
	}
	m.mu.Unlock()
}

func newTestManifest(id string) manifest.Manifest {
	return manifest.Manifest{
		ID:      id,
		Name:    strings.ToUpper(id),
		Version: "1.0.0",
		Runtime: manifest.RuntimeSpec{
			Type:    manifest.RuntimeTypeLocalBinary,
			Install: manifest.CommandSpec{Command: "install-" + id},
			Upgrade: manifest.CommandSpec{Command: "upgrade-" + id},
			Start:   manifest.CommandSpec{Command: "tail -f /dev/null"},
			Stop:    manifest.CommandSpec{Command: "stop-" + id},
		},
		Memory: manifest.MemorySpec{
			Supports:  []manifest.MemoryType{manifest.MemoryTypePerAgent},
			MountPath: "./memory",
		},
		Upgrade: manifest.UpgradeSpec{
			Channel:  "stable",
			Strategy: manifest.UpgradeStrategyInPlaceOrReinstall,
		},
	}
}

func TestHandleUpgradeSuccessRound10(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{}, lifecycle.WithRunner(noopRunner{}), lifecycle.WithRuntimeChecker(noopChecker{}))
	m := newTestManifest("openclaw")
	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("register manifest: %v", err)
	}
	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}

	mux := buildTestMux(svc, true)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/upgrade", strings.NewReader(`{"agentId":"openclaw"}`))
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("upgrade status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
}

func TestUpgradeRouteBadBodyRound10(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	mux := buildTestMux(svc, true)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/upgrade", strings.NewReader(`{bad`))
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

func TestDiagnosisHandoffSuccessRound10(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{}, lifecycle.WithRunner(noopRunner{}), lifecycle.WithRuntimeChecker(noopChecker{}), lifecycle.WithDiagnoseDir(t.TempDir()))
	m := newTestManifest("openclaw")
	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("register manifest: %v", err)
	}
	if _, err := svc.HandleFailure(context.Background(), "openclaw", "simulated crash"); err != nil {
		t.Fatalf("HandleFailure: %v", err)
	}

	mux := buildTestMux(svc, true)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/diagnosis/handoffs", strings.NewReader(`{"agentId":"openclaw","consent":true,"actor":"telegram:42","requestId":"req-42"}`))
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	handoffID, _ := payload["id"].(string)
	if strings.TrimSpace(handoffID) == "" {
		t.Fatalf("expected non-empty handoff id, payload=%v", payload)
	}
}

func TestHandleInstallMultiInstanceInstallFailureRound10(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{}, lifecycle.WithRunner(alwaysFailRunner{}), lifecycle.WithRuntimeChecker(noopChecker{}))
	m := newTestManifest("openclaw")
	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("register manifest: %v", err)
	}
	mux := buildTestMux(svc, true)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/openclaw/install", strings.NewReader(`{"multi_instance":true}`))
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", rr.Code, rr.Body.String())
	}
}

func TestDecodeBodyReadAndChunkedOversizeRound10(t *testing.T) {
	t.Run("read failure", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/x", nil)
		req.Body = errBodyReadCloser{}
		req.ContentLength = -1
		var body map[string]any
		if ok := decodeBody(rr, req, &body); ok {
			t.Fatal("decodeBody should fail on read error")
		}
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("chunked oversized body", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(strings.Repeat("x", maxBodySize+1)))
		req.ContentLength = -1
		var body map[string]any
		if ok := decodeBody(rr, req, &body); ok {
			t.Fatal("decodeBody should fail on oversized body")
		}
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
		}
	})
}

func TestStopAllAgentsErrorBranchRound10(t *testing.T) {
	pm := newFailingStopProcessManager()
	svc := lifecycle.NewService(baseagent.NoopTriager{}, lifecycle.WithProcessManager(pm), lifecycle.WithRunner(noopRunner{}), lifecycle.WithRuntimeChecker(noopChecker{}))
	for _, id := range []string{"openclaw", "picoclaw"} {
		m := newTestManifest(id)
		if err := svc.RegisterManifest(m); err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
		if err := svc.Install(context.Background(), id); err != nil {
			t.Fatalf("install %s: %v", id, err)
		}
		if err := svc.Start(context.Background(), id); err != nil {
			t.Fatalf("start %s: %v", id, err)
		}
	}

	err := stopAllAgents(context.Background(), svc)
	if err == nil || !strings.Contains(err.Error(), "forced stop failure") {
		t.Fatalf("expected stop failure, got %v", err)
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if pm.stopCalls == 0 {
		t.Fatal("expected process manager Stop to be called")
	}
	if !pm.cleaned {
		t.Fatal("expected process manager Cleanup to be called")
	}
}

func TestShutdownAgentsTimeoutBranchRound10(t *testing.T) {
	pm := newBlockingStopProcessManager()
	svc := lifecycle.NewService(baseagent.NoopTriager{}, lifecycle.WithProcessManager(pm), lifecycle.WithRunner(noopRunner{}), lifecycle.WithRuntimeChecker(noopChecker{}))
	m := newTestManifest("openclaw")
	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("register manifest: %v", err)
	}
	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("install openclaw: %v", err)
	}
	if err := svc.Start(context.Background(), "openclaw"); err != nil {
		t.Fatalf("start openclaw: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- shutdownAgents(svc, 5*time.Millisecond)
	}()

	select {
	case <-pm.started:
	case <-time.After(2 * time.Second):
		t.Fatal("stop was not started")
	}

	err := <-done
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	close(pm.release)
}
