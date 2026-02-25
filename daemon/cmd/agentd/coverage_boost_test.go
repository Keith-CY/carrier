package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"carrier/baseagent"
	"carrier/daemon/internal/api"
	"carrier/daemon/internal/catalog"
	"carrier/daemon/internal/lifecycle"
	"carrier/daemon/internal/ratelimit"
)

// --- handleUninstall ---

func TestHandleUninstall(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
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

	// Install first
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/install", strings.NewReader(`{"agentId":"openclaw"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("install: %d %s", rec.Code, rec.Body.String())
	}

	// Uninstall via v1 agents path
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/agents/openclaw/uninstall", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("uninstall: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "uninstalled" {
		t.Fatalf("expected status=uninstalled, got %v", resp)
	}
}

func TestHandleUninstallMethodNotAllowed(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}
	mux := buildTestMux(svc, true)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/agents/openclaw/uninstall", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleUninstallNotFound(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	mux := buildTestMux(svc, true)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/agents/nonexistent/uninstall", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// --- handleInstall multi-instance ---

func TestHandleInstallMultiInstance(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	svc := lifecycle.NewService(
		baseagent.NoopTriager{},
		lifecycle.WithRunner(noopRunner{}),
		lifecycle.WithRuntimeChecker(noopChecker{}),
		lifecycle.WithProcessManager(newFakeProcessManager()),
	)
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}

	var readyFlag atomic.Bool
	readyFlag.Store(true)
	pairStore := api.NewPairingCodeStore(nil)
	mux := buildHTTPMux(svc, &readyFlag, pairStore, ratelimit.New())

	// The v1 agents route calls handleInstall with agentID from path.
	// For multi-instance, we POST with instance_name in the body to the v1 route.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/agents/openclaw/install",
		strings.NewReader(`{"instance_name":"myinst1","multi_instance":true}`))
	req.ContentLength = int64(len(`{"instance_name":"myinst1","multi_instance":true}`))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("named instance install: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["instance_id"] != "myinst1" {
		t.Fatalf("expected instance_id=myinst1, got %v", resp["instance_id"])
	}
}

// --- handleInstall multi-instance error paths ---

func TestHandleInstallMultiInstanceNotFoundBase(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	svc := lifecycle.NewService(
		baseagent.NoopTriager{},
		lifecycle.WithRunner(noopRunner{}),
		lifecycle.WithRuntimeChecker(noopChecker{}),
		lifecycle.WithProcessManager(newFakeProcessManager()),
	)
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}

	var readyFlag atomic.Bool
	readyFlag.Store(true)
	pairStore := api.NewPairingCodeStore(nil)
	mux := buildHTTPMux(svc, &readyFlag, pairStore, ratelimit.New())

	// Multi-instance with nonexistent base agent
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/agents/nonexistent/install",
		strings.NewReader(`{"instance_name":"inst1"}`))
	req.ContentLength = int64(len(`{"instance_name":"inst1"}`))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleInstallMultiInstanceDuplicate(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	svc := lifecycle.NewService(
		baseagent.NoopTriager{},
		lifecycle.WithRunner(noopRunner{}),
		lifecycle.WithRuntimeChecker(noopChecker{}),
		lifecycle.WithProcessManager(newFakeProcessManager()),
	)
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}

	var readyFlag atomic.Bool
	readyFlag.Store(true)
	pairStore := api.NewPairingCodeStore(nil)
	mux := buildHTTPMux(svc, &readyFlag, pairStore, ratelimit.New())

	// First instance
	rec := httptest.NewRecorder()
	body := `{"instance_name":"dup1"}`
	req := httptest.NewRequest("POST", "/api/v1/agents/openclaw/install", strings.NewReader(body))
	req.ContentLength = int64(len(body))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Duplicate
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/v1/agents/openclaw/install", strings.NewReader(body))
	req.ContentLength = int64(len(body))
	mux.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("expected error for duplicate instance")
	}
}

// --- handleUpgrade ---

func TestHandleUpgrade(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
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

	// Install first
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/install", strings.NewReader(`{"agentId":"openclaw"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("install: %d", rec.Code)
	}

	// Upgrade via legacy route — catalog uses "latest" which isn't semver, expect 500
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/upgrade", strings.NewReader(`{"agentId":"openclaw"}`)))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("upgrade: expected 500 (non-semver version), got %d: %s", rec.Code, rec.Body.String())
	}

	// Upgrade via v1 route — same
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/agents/openclaw/upgrade", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("v1 upgrade: expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUpgradeMethodNotAllowed(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	mux := buildTestMux(svc, true)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/agents/openclaw/upgrade", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

// --- writeServiceError covers more error types ---

func TestWriteServiceErrorCoversAllTypes(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
	}{
		{"ErrAlreadyRunning", lifecycle.ErrAlreadyRunning, http.StatusConflict},
		{"ErrAlreadyStopped", lifecycle.ErrAlreadyStopped, http.StatusConflict},
		{"ErrCrashLoop", lifecycle.ErrCrashLoop, http.StatusConflict},
		{"ErrAgentRunning", lifecycle.ErrAgentRunning, http.StatusConflict},
		{"ErrUpgradeNotSupported", lifecycle.ErrUpgradeNotSupported, http.StatusBadRequest},
		{"ErrAgentNotFound", lifecycle.ErrAgentNotFound, http.StatusNotFound},
		{"generic error", lifecycle.ErrNotInstalled, http.StatusConflict},
		{"unknown error", http.ErrServerClosed, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeServiceError(rec, tt.err)
			if rec.Code != tt.status {
				t.Fatalf("expected %d, got %d", tt.status, rec.Code)
			}
		})
	}
}

// --- remoteIP ---

func TestRemoteIPWithXForwardedFor(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 192.168.1.1")
	ip := remoteIP(req)
	if ip != "10.0.0.1" {
		t.Fatalf("expected 10.0.0.1, got %s", ip)
	}
}

func TestRemoteIPFallbackNoPort(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "badaddr" // no port
	ip := remoteIP(req)
	if ip != "badaddr" {
		t.Fatalf("expected badaddr, got %s", ip)
	}
}

// --- isLoopback ---

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
	for _, tt := range tests {
		if got := isLoopback(tt.host); got != tt.want {
			t.Errorf("isLoopback(%q) = %v, want %v", tt.host, got, tt.want)
		}
	}
}

// --- setupForwarding / diagnosis handoffs ---

func TestDiagnosisHandoffsEndpoint(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
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

	// Method not allowed
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/diagnosis/handoffs", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}

	// Empty agentId
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/diagnosis/handoffs",
		strings.NewReader(`{"agentId":"","consent":true,"actor":"test"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	// Invalid agentId
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/diagnosis/handoffs",
		strings.NewReader(`{"agentId":"../bad","consent":true,"actor":"test"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid agentId, got %d", rec.Code)
	}
}

// --- bearerAuthMiddleware readyz exempt ---

func TestBearerAuthMiddlewareReadyzExempt(t *testing.T) {
	base := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := bearerAuthMiddleware("secret", base)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("readyz should be exempt, got %d", rec.Code)
	}
}

// --- stopAllAgents with running agents ---

func TestStopAllAgentsWithRunning(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	svc := lifecycle.NewService(
		baseagent.NoopTriager{},
		lifecycle.WithRunner(noopRunner{}),
		lifecycle.WithRuntimeChecker(noopChecker{}),
		lifecycle.WithProcessManager(newFakeProcessManager()),
	)
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background(), "openclaw"); err != nil {
		t.Fatal(err)
	}

	if err := stopAllAgents(context.Background(), svc); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

// --- handler method not allowed via v1 routes ---

func TestHandlerMethodNotAllowedV1(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}
	mux := buildTestMux(svc, true)

	// stop requires POST
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/agents/openclaw/stop", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("stop GET: expected 405, got %d", rec.Code)
	}

	// status requires GET
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/agents/openclaw/status", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status POST: expected 405, got %d", rec.Code)
	}

	// logs requires GET
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/agents/openclaw/logs", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("logs POST: expected 405, got %d", rec.Code)
	}

	// diagnose requires POST
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/agents/openclaw/diagnose", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("diagnose GET: expected 405, got %d", rec.Code)
	}

	// start requires POST
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/agents/openclaw/start", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("start GET: expected 405, got %d", rec.Code)
	}

	// install requires POST
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/agents/openclaw/install", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("install GET: expected 405, got %d", rec.Code)
	}
}

// --- handleLogs with tail param ---

func TestHandleLogsWithTailParam(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}
	mux := buildTestMux(svc, true)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/logs/openclaw?tail=50", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

// --- handleStatus success ---

func TestHandleStatusSuccess(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}
	mux := buildTestMux(svc, true)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/status/openclaw", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- handleInstall error from service ---

func TestHandleInstallNotFound(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	mux := buildTestMux(svc, true)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/install", strings.NewReader(`{"agentId":"nonexistent"}`)))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// --- extractAgentIDFromBody edge cases ---

func TestExtractAgentIDFromBodyEmptyID(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	mux := buildTestMux(svc, true)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/install", strings.NewReader(`{"agentId":""}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestExtractAgentIDFromBodyInvalidChars(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	mux := buildTestMux(svc, true)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/install", strings.NewReader(`{"agentId":"../bad"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// --- handleStop error path ---

func TestHandleStopNotInstalled(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}
	mux := buildTestMux(svc, true)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/stop", strings.NewReader(`{"agentId":"openclaw"}`)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- handleDiagnose error path ---

func TestHandleDiagnoseNotFound(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	mux := buildTestMux(svc, true)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/diagnose", strings.NewReader(`{"agentId":"nonexistent"}`)))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- trimPathByPrefixes no match ---

func TestTrimPathByPrefixesNoMatch(t *testing.T) {
	result := trimPathByPrefixes("/other/path", "/api/status/", "/api/v1/status/")
	if result != "/other/path" {
		t.Fatalf("expected unchanged path, got %s", result)
	}
}

// --- shutdownAgents with timeout ---

func TestShutdownAgentsWithRunningAgent(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	svc := lifecycle.NewService(
		baseagent.NoopTriager{},
		lifecycle.WithRunner(noopRunner{}),
		lifecycle.WithRuntimeChecker(noopChecker{}),
		lifecycle.WithProcessManager(newFakeProcessManager()),
	)
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background(), "openclaw"); err != nil {
		t.Fatal(err)
	}

	err := shutdownAgents(svc, 10*time.Second)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

// --- v1 agents/status method not allowed ---

func TestAgentsStatusMethodNotAllowed(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}
	mux := buildTestMux(svc, true)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/agents/status", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

// --- pairing verify-consume via legacy route with bad body ---

func TestPairingVerifyConsumeBadBody(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	pairStore := api.NewPairingCodeStore(nil)

	var readyFlag atomic.Bool
	readyFlag.Store(true)
	mux := buildHTTPMux(svc, &readyFlag, pairStore, ratelimit.New())

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/pairing/verify-consume", strings.NewReader(`{bad`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// --- pairing rate limit with X-Forwarded-For ---

func TestPairingRateLimitXForwardedFor(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	pairStore := api.NewPairingCodeStore(nil)

	var readyFlag atomic.Bool
	readyFlag.Store(true)
	limiter := ratelimit.New(ratelimit.WithMax(1), ratelimit.WithWindow(1*time.Minute))
	mux := buildHTTPMux(svc, &readyFlag, pairStore, limiter)

	// First request with XFF
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/pairing/verify-consume", strings.NewReader(`{"code":"bad"}`))
	req.Header.Set("X-Forwarded-For", "10.0.0.99")
	mux.ServeHTTP(rec, req)

	// Second should be rate limited
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/pairing/verify-consume", strings.NewReader(`{"code":"bad"}`))
	req.Header.Set("X-Forwarded-For", "10.0.0.99")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
}

// --- decodeBody edge cases ---

func TestDecodeBodyContentLengthTooLarge(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	var readyFlag atomic.Bool
	readyFlag.Store(true)
	pairStore := api.NewPairingCodeStore(nil)
	limiter := ratelimit.New()
	mux := buildHTTPMux(svc, &readyFlag, pairStore, limiter)

	body := strings.Repeat("x", 2*1024*1024) // 2MB - exceeds maxBodySize
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/pairing/verify-consume", strings.NewReader(body))
	req.ContentLength = int64(len(body))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestDecodeBodyEmptyBody(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	var readyFlag atomic.Bool
	readyFlag.Store(true)
	pairStore := api.NewPairingCodeStore(nil)
	limiter := ratelimit.New()
	mux := buildHTTPMux(svc, &readyFlag, pairStore, limiter)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/pairing/verify-consume", strings.NewReader("   "))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestDecodeBodyTrailingData(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	var readyFlag atomic.Bool
	readyFlag.Store(true)
	pairStore := api.NewPairingCodeStore(nil)
	limiter := ratelimit.New()
	mux := buildHTTPMux(svc, &readyFlag, pairStore, limiter)

	rec := httptest.NewRecorder()
	// Two JSON objects = trailing data
	req := httptest.NewRequest("POST", "/api/pairing/verify-consume", strings.NewReader(`{"code":"x"}{"extra":true}`))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// --- shutdownAgents timeout path ---

func TestShutdownAgentsNoAgents(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	// Shutdown with no agents - exercises the function path
	err := shutdownAgents(svc, 5*time.Second)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestShutdownAgentsTimeoutWithRunning(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	svc := lifecycle.NewService(
		baseagent.NoopTriager{},
		lifecycle.WithRunner(noopRunner{}),
		lifecycle.WithRuntimeChecker(noopChecker{}),
		lifecycle.WithProcessManager(newFakeProcessManager()),
	)
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background(), "openclaw"); err != nil {
		t.Fatal(err)
	}
	// With agents running, shutdown should work
	err := shutdownAgents(svc, 10*time.Second)
	if err != nil {
		t.Logf("shutdown returned: %v", err)
	}
}

func TestStopAllAgentsNoRunning(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	err := stopAllAgents(context.Background(), svc)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestStopAllAgentsCancelledCtx(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	svc := lifecycle.NewService(
		baseagent.NoopTriager{},
		lifecycle.WithRunner(noopRunner{}),
		lifecycle.WithRuntimeChecker(noopChecker{}),
		lifecycle.WithProcessManager(newFakeProcessManager()),
	)
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background(), "openclaw"); err != nil {
		t.Fatal(err)
	}

	// Use already-cancelled context to force Stop error path
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Error is expected since context is cancelled
	_ = stopAllAgents(ctx, svc)
}

// --- handleInstall multi-instance path ---

func TestHandleInstallMultiInstanceWithName(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	svc := lifecycle.NewService(
		baseagent.NoopTriager{},
		lifecycle.WithRunner(noopRunner{}),
		lifecycle.WithRuntimeChecker(noopChecker{}),
		lifecycle.WithProcessManager(newFakeProcessManager()),
	)
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}

	var readyFlag atomic.Bool
	readyFlag.Store(true)
	pairStore := api.NewPairingCodeStore(nil)
	limiter := ratelimit.New()
	mux := buildHTTPMux(svc, &readyFlag, pairStore, limiter)

	// Test instance_name
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/agents/openclaw/install", strings.NewReader(`{"instance_name":"test-inst"}`))
	mux.ServeHTTP(rec, req)
	t.Logf("instance_name response: %d %s", rec.Code, rec.Body.String())
}

// --- handleUpgrade error path ---

func TestHandleUpgradeNotInstalled(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	var readyFlag atomic.Bool
	readyFlag.Store(true)
	pairStore := api.NewPairingCodeStore(nil)
	limiter := ratelimit.New()
	mux := buildHTTPMux(svc, &readyFlag, pairStore, limiter)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/agents/nonexistent/upgrade", nil)
	mux.ServeHTTP(rec, req)
	// Should return error since agent doesn't exist
	if rec.Code == http.StatusOK {
		t.Fatal("expected error for non-existent agent upgrade")
	}
}

// --- bearerAuthMiddleware ---

func TestBearerAuthMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := bearerAuthMiddleware("secret-token", inner)

	// No auth header on /api/ path
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/agents", nil)
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	// Wrong token
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/agents", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	// Correct token
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/agents", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Non-API path should bypass auth
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/healthz", nil)
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

// --- diagnosis handoffs endpoint ---

func TestDiagnosisHandoffsEmptyBody(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	var readyFlag atomic.Bool
	readyFlag.Store(true)
	pairStore := api.NewPairingCodeStore(nil)
	limiter := ratelimit.New()
	mux := buildHTTPMux(svc, &readyFlag, pairStore, limiter)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/diagnosis/handoffs", strings.NewReader("   "))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestDiagnosisHandoffsValid(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	svc := lifecycle.NewService(
		baseagent.NoopTriager{},
		lifecycle.WithRunner(noopRunner{}),
		lifecycle.WithRuntimeChecker(noopChecker{}),
		lifecycle.WithProcessManager(newFakeProcessManager()),
	)
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}
	var readyFlag atomic.Bool
	readyFlag.Store(true)
	pairStore := api.NewPairingCodeStore(nil)
	limiter := ratelimit.New()
	mux := buildHTTPMux(svc, &readyFlag, pairStore, limiter)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/diagnosis/handoffs", strings.NewReader(`{"agentId":"openclaw","consent":true,"actor":"test","requestId":"r1"}`))
	mux.ServeHTTP(rec, req)
	t.Logf("diagnosis handoffs: %d %s", rec.Code, rec.Body.String())
}

func TestDiagnosisHandoffsMethodNotAllowed(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	var readyFlag atomic.Bool
	readyFlag.Store(true)
	pairStore := api.NewPairingCodeStore(nil)
	limiter := ratelimit.New()
	mux := buildHTTPMux(svc, &readyFlag, pairStore, limiter)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/diagnosis/handoffs", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestDiagnosisHandoffsMissingAgentID(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	var readyFlag atomic.Bool
	readyFlag.Store(true)
	pairStore := api.NewPairingCodeStore(nil)
	limiter := ratelimit.New()
	mux := buildHTTPMux(svc, &readyFlag, pairStore, limiter)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/diagnosis/handoffs", strings.NewReader(`{"consent":true}`))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
