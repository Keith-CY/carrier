package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"carrier/daemon/internal/baseagent"
	"carrier/daemon/internal/catalog"
	"carrier/daemon/internal/commandexec"
	"carrier/daemon/internal/lifecycle"
	"carrier/daemon/internal/manifest"
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
	return buildHTTPMux(svc, &readyFlag)
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
	if err := stopAllAgents(svc); err != nil {
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
