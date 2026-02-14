package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
	mux := buildHTTPMux(svc)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestReadyz(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	mux := buildHTTPMux(svc)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestAPIListAgents(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}
	mux := buildHTTPMux(svc)
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
	mux := buildHTTPMux(svc)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/status/nonexistent", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestAPIInstallAndStart(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{}, lifecycle.WithRunner(noopRunner{}), lifecycle.WithRuntimeChecker(noopChecker{}))
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatal(err)
	}
	mux := buildHTTPMux(svc)

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
	mux := buildHTTPMux(svc)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/agents", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
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
