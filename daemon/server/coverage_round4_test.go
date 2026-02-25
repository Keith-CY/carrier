package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"carrier/baseagent"
	"carrier/daemon/internal/lifecycle"
	"carrier/daemon/internal/manifest"
)

func TestHandleInstallMultiInstancePaths(t *testing.T) {
	svc := newTestServiceWithAgent(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/test-agent/install", strings.NewReader(`{"instance_name":"instance-a","multi_instance":true}`))
	w := httptest.NewRecorder()
	handleInstall(svc, "test-agent", w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for multi-instance install, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"instance_id":"instance-a"`) {
		t.Fatalf("expected explicit instance_id in response, got %s", w.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/agents/test-agent/install", strings.NewReader(`{"instance_name":"bad/name","multi_instance":true}`))
	w = httptest.NewRecorder()
	handleInstall(svc, "test-agent", w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for invalid instance name, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleActionErrorPaths(t *testing.T) {
	svc := newTestServiceWithAgent(t)

	tests := []struct {
		name   string
		method string
		call   func(*httptest.ResponseRecorder, *http.Request)
	}{
		{
			name:   "uninstall_missing_agent",
			method: http.MethodPost,
			call: func(w *httptest.ResponseRecorder, r *http.Request) {
				handleUninstall(svc, "missing-agent", w, r)
			},
		},
		{
			name:   "logs_missing_agent",
			method: http.MethodGet,
			call: func(w *httptest.ResponseRecorder, r *http.Request) {
				handleLogs(svc, "missing-agent", w, r)
			},
		},
		{
			name:   "diagnose_missing_agent",
			method: http.MethodPost,
			call: func(w *httptest.ResponseRecorder, r *http.Request) {
				handleDiagnose(svc, "missing-agent", w, r)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, "/api/v1/agents/missing-agent", nil)
			tc.call(w, req)
			if w.Code != http.StatusNotFound {
				t.Fatalf("expected 404, got %d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestStopAllAgentsStopsRunningAgent(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})
	m := manifest.Manifest{
		ID:      "runner",
		Name:    "Runner",
		Version: "1.0.0",
		Runtime: manifest.RuntimeSpec{
			Type:    manifest.RuntimeTypeLocalBinary,
			Install: manifest.CommandSpec{Command: "echo install"},
			Start:   manifest.CommandSpec{Command: "sleep 30"},
			Stop:    manifest.CommandSpec{Command: "echo stop"},
		},
		Memory: manifest.MemorySpec{
			Supports:  []manifest.MemoryType{manifest.MemoryTypePerAgent},
			MountPath: t.TempDir(),
		},
	}
	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("register manifest: %v", err)
	}
	if err := svc.Install(context.Background(), "runner"); err != nil {
		t.Fatalf("install runner: %v", err)
	}
	if err := svc.Start(context.Background(), "runner"); err != nil {
		t.Fatalf("start runner: %v", err)
	}

	if err := stopAllAgents(context.Background(), svc); err != nil {
		t.Fatalf("stopAllAgents returned error: %v", err)
	}

	st, err := svc.Status("runner")
	if err != nil {
		t.Fatalf("status runner: %v", err)
	}
	if st.Runtime != lifecycle.RuntimeStateStopped {
		t.Fatalf("expected runner to be stopped, got %s", st.Runtime)
	}
}
