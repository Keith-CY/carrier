package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"carrier/baseagent"
	"carrier/daemon/internal/api"
	"carrier/daemon/internal/commandexec"
	"carrier/daemon/internal/lifecycle"
	"carrier/daemon/internal/manifest"
	"carrier/daemon/internal/ratelimit"
)

type serverNoopRunner struct{}

func (serverNoopRunner) Run(_ context.Context, _ string) (commandexec.Result, error) {
	return commandexec.Result{ExitCode: 0, CombinedOutput: "ok"}, nil
}

type serverNoopChecker struct{}

func (serverNoopChecker) Check(_ manifest.Manifest) error { return nil }

type controlledProcessManager struct {
	mu          sync.Mutex
	running     map[string]bool
	done        map[string]chan struct{}
	stopDelay   time.Duration
	stopErr     error
	closeOnStop bool
}

func newControlledProcessManager(stopDelay time.Duration, stopErr error, closeOnStop bool) *controlledProcessManager {
	return &controlledProcessManager{
		running:     map[string]bool{},
		done:        map[string]chan struct{}{},
		stopDelay:   stopDelay,
		stopErr:     stopErr,
		closeOnStop: closeOnStop,
	}
}

func (pm *controlledProcessManager) Start(agentID, _ string, _ []string) (int, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.running[agentID] = true
	pm.done[agentID] = make(chan struct{})
	return 42, nil
}

func (pm *controlledProcessManager) Stop(agentID string) error {
	if pm.stopDelay > 0 {
		time.Sleep(pm.stopDelay)
	}

	pm.mu.Lock()
	ch := pm.done[agentID]
	pm.mu.Unlock()

	if pm.closeOnStop && ch != nil {
		select {
		case <-ch:
		default:
			close(ch)
		}
	}
	if pm.stopErr != nil {
		return pm.stopErr
	}

	pm.mu.Lock()
	pm.running[agentID] = false
	pm.mu.Unlock()
	return nil
}

func (pm *controlledProcessManager) IsRunning(agentID string) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.running[agentID]
}

func (pm *controlledProcessManager) Wait(agentID string) error {
	pm.mu.Lock()
	ch := pm.done[agentID]
	pm.mu.Unlock()
	if ch == nil {
		return errors.New("process not found")
	}
	<-ch
	return nil
}

func (pm *controlledProcessManager) Cleanup() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for id, ch := range pm.done {
		select {
		case <-ch:
		default:
			close(ch)
		}
		pm.running[id] = false
	}
}

func newRunningServiceWithProcessManager(t *testing.T, pm lifecycle.ProcessController) *lifecycle.Service {
	t.Helper()
	svc := lifecycle.NewService(
		baseagent.NoopTriager{},
		lifecycle.WithRunner(serverNoopRunner{}),
		lifecycle.WithRuntimeChecker(serverNoopChecker{}),
		lifecycle.WithProcessManager(pm),
	)
	m := manifest.Manifest{
		ID:      "runner",
		Name:    "Runner",
		Version: "1.0.0",
		Runtime: manifest.RuntimeSpec{
			Type:    manifest.RuntimeTypeLocalBinary,
			Install: manifest.CommandSpec{Command: "echo install"},
			Start:   manifest.CommandSpec{Command: "echo start"},
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
	return svc
}

func TestBaseAgentChatEndpointBranches(t *testing.T) {
	t.Run("method not allowed", func(t *testing.T) {
		svc := newTestServiceWithAgent(t)
		var ready atomic.Bool
		ready.Store(true)
		mux := buildHTTPMuxWithBaseAgent(svc, nil, &ready, api.NewPairingCodeStore(nil), ratelimit.New())

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/base-agent/chat", nil)
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
	})

	t.Run("runtime unavailable", func(t *testing.T) {
		svc := newTestServiceWithAgent(t)
		var ready atomic.Bool
		ready.Store(true)
		mux := buildHTTPMuxWithBaseAgent(svc, nil, &ready, api.NewPairingCodeStore(nil), ratelimit.New())

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/base-agent/chat", strings.NewReader(`{"message":"list agents"}`))
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("decode and validation errors", func(t *testing.T) {
		svc := newTestServiceWithAgent(t)
		var ready atomic.Bool
		ready.Store(true)
		rt := baseagent.NewRuntime(newLifecycleAgentServiceAdapter(svc), nil)
		mux := buildHTTPMuxWithBaseAgent(svc, rt, &ready, api.NewPairingCodeStore(nil), ratelimit.New())

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/base-agent/chat", strings.NewReader(`{"message"`))
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for decode error, got %d", rec.Code)
		}

		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/api/base-agent/chat", strings.NewReader(`{"message":"   "}`))
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for missing message, got %d", rec.Code)
		}
	})

	t.Run("success", func(t *testing.T) {
		svc := newTestServiceWithAgent(t)
		var ready atomic.Bool
		ready.Store(true)
		rt := baseagent.NewRuntime(newLifecycleAgentServiceAdapter(svc), nil)
		mux := buildHTTPMuxWithBaseAgent(svc, rt, &ready, api.NewPairingCodeStore(nil), ratelimit.New())

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/base-agent/chat", strings.NewReader(`{"message":"list agents"}`))
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Found") {
			t.Fatalf("expected agent list response, got %s", rec.Body.String())
		}
	})
}

func TestPairingVerifyConsumeLimiterAndDecodeBranches(t *testing.T) {
	svc := newTestServiceWithAgent(t)
	var ready atomic.Bool
	ready.Store(true)
	mux := buildHTTPMuxWithBaseAgent(
		svc,
		nil,
		&ready,
		api.NewPairingCodeStore(nil),
		ratelimit.New(ratelimit.WithMax(0), ratelimit.WithWindow(time.Minute)),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/pairing/verify-consume", strings.NewReader(`{"code":"abc"}`))
	req.RemoteAddr = "127.0.0.1:12345"
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Retry-After") != "60" {
		t.Fatalf("expected Retry-After=60, got %q", rec.Header().Get("Retry-After"))
	}

	mux = buildHTTPMuxWithBaseAgent(
		svc,
		nil,
		&ready,
		api.NewPairingCodeStore(nil),
		nil,
	)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/pairing/verify-consume", strings.NewReader(`{"code"`))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for decode error, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDiagnosisHandoffBranches(t *testing.T) {
	svc := newTestServiceWithAgent(t)
	var ready atomic.Bool
	ready.Store(true)
	mux := buildHTTPMuxWithBaseAgent(svc, nil, &ready, api.NewPairingCodeStore(nil), nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/diagnosis/handoffs", strings.NewReader(`{"agentId":"bad/id","consent":true}`))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid agent id, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/diagnosis/handoffs", strings.NewReader(`{"agentId":"missing-agent","consent":true}`))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when service returns agent-not-found, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestShutdownAndStopAllAgents_ErrorBranches(t *testing.T) {
	t.Run("stopAllAgents returns first stop error", func(t *testing.T) {
		pm := newControlledProcessManager(0, errors.New("stop failed"), false)
		svc := newRunningServiceWithProcessManager(t, pm)
		err := stopAllAgents(context.Background(), svc)
		if err == nil || !strings.Contains(err.Error(), "stop failed") {
			t.Fatalf("expected stop failed error, got %v", err)
		}
	})

	t.Run("shutdownAgents returns timeout when stop hangs", func(t *testing.T) {
		pm := newControlledProcessManager(200*time.Millisecond, nil, true)
		svc := newRunningServiceWithProcessManager(t, pm)
		err := shutdownAgents(svc, 20*time.Millisecond)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected context deadline exceeded, got %v", err)
		}
	})
}
