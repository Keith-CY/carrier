package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"carrier/daemon/internal/commandexec"
	"carrier/daemon/internal/lifecycle"
	"carrier/daemon/internal/manifest"
	"carrier/daemon/internal/runtimecheck"
)

type fakeRunner struct {
	results map[string]error
}

func (f *fakeRunner) Run(_ context.Context, command string) (commandexec.Result, error) {
	if f.results != nil {
		if err, ok := f.results[command]; ok {
			return commandexec.Result{ExitCode: 1}, err
		}
	}
	return commandexec.Result{ExitCode: 0}, nil
}

type fakeChecker struct {
	err error
}

func (f *fakeChecker) Check(manifest.Manifest) error {
	return f.err
}

func newServiceForAPITest(t *testing.T) *lifecycle.Service {
	t.Helper()
	runner := &fakeRunner{}
	checker := &fakeChecker{}
	now := func() time.Time {
		return time.Date(2026, 2, 14, 16, 30, 0, 0, time.UTC)
	}
	svc := lifecycle.NewService(nil,
		lifecycle.WithRunner(runner),
		lifecycle.WithRuntimeChecker(checker),
		lifecycle.WithDiagnoseDir(t.TempDir()),
		lifecycle.WithNow(now),
	)
	if err := svc.RegisterManifest(sampleManifest()); err != nil {
		t.Fatalf("RegisterManifest() error: %v", err)
	}
	return svc
}

func sampleManifest() manifest.Manifest {
	return manifest.Manifest{
		ID:      "openclaw",
		Name:    "OpenClaw",
		Version: "1.0.0",
		Runtime: manifest.RuntimeSpec{
			Type:    manifest.RuntimeTypeLocalBinary,
			Install: manifest.CommandSpec{Command: "install-openclaw"},
			Upgrade: manifest.CommandSpec{Command: "upgrade-openclaw"},
			Start:   manifest.CommandSpec{Command: "start-openclaw"},
			Stop:    manifest.CommandSpec{Command: "stop-openclaw"},
		},
		Network: manifest.NetworkSpec{
			Ports: []manifest.PortSpec{{Name: "http", Port: 0}},
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

func doJSONRequest(t *testing.T, handler http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var payload []byte
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json marshal: %v", err)
		}
		payload = raw
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func TestListAgentsEndpointReturnsCatalogAndState(t *testing.T) {
	svc := newServiceForAPITest(t)
	handler := NewServer(svc).Handler()

	rr := doJSONRequest(t, handler, http.MethodGet, "/api/v1/agents", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Agents []daemonAgent `json:"agents"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(resp.Agents))
	}
	agent := resp.Agents[0]
	if agent.ID != "openclaw" || agent.Name != "OpenClaw" {
		t.Fatalf("unexpected agent identity: %#v", agent)
	}
	if agent.Installed {
		t.Fatalf("expected not installed, got installed=true")
	}
	if agent.RuntimeState != string(lifecycle.RuntimeStateStopped) {
		t.Fatalf("unexpected runtime state: %q", agent.RuntimeState)
	}
}

func TestStopEndpointSuccessAndAlreadyStopped(t *testing.T) {
	svc := newServiceForAPITest(t)
	handler := NewServer(svc).Handler()
	ctx := context.Background()

	if err := svc.Install(ctx, "openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := svc.Start(ctx, "openclaw"); err != nil {
		t.Fatalf("start: %v", err)
	}

	stopOK := doJSONRequest(t, handler, http.MethodPost, "/api/v1/agents/openclaw/stop", nil)
	if stopOK.Code != http.StatusOK {
		t.Fatalf("stop status = %d, want 200; body=%s", stopOK.Code, stopOK.Body.String())
	}

	stopAgain := doJSONRequest(t, handler, http.MethodPost, "/api/v1/agents/openclaw/stop", nil)
	if stopAgain.Code != http.StatusConflict {
		t.Fatalf("second stop status = %d, want 409; body=%s", stopAgain.Code, stopAgain.Body.String())
	}
	var env errorEnvelope
	if err := json.Unmarshal(stopAgain.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if env.Error.Code != "E_ALREADY_STOPPED" {
		t.Fatalf("unexpected error code: %q", env.Error.Code)
	}
}

func TestUpgradeEndpointReturnsVersionAndRollbackMetadata(t *testing.T) {
	svc := newServiceForAPITest(t)
	handler := NewServer(svc).Handler()
	ctx := context.Background()

	if err := svc.Install(ctx, "openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}

	rr := doJSONRequest(t, handler, http.MethodPost, "/api/v1/agents/openclaw/upgrade", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("upgrade status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		AgentID     string `json:"agentId"`
		FromVersion string `json:"fromVersion"`
		ToVersion   string `json:"toVersion"`
		BackupPath  string `json:"backupPath"`
		Rollback    string `json:"rollbackHint"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.AgentID != "openclaw" || resp.FromVersion != "1.0.0" || resp.ToVersion != "1.0.1" {
		t.Fatalf("unexpected upgrade payload: %#v", resp)
	}
	if strings.TrimSpace(resp.BackupPath) == "" {
		t.Fatal("expected backupPath in upgrade response")
	}
	if !strings.Contains(resp.Rollback, resp.BackupPath) {
		t.Fatalf("expected rollback hint to reference backup path, got %q", resp.Rollback)
	}
}

func TestVerifyConsumePairCodeSuccessAndInvalid(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 2, 14, 17, 0, 0, 0, time.UTC)}
	pairing := NewPairingCodeStore(clock.Now)
	if _, err := pairing.Register("pair-test-code", 30*time.Second); err != nil {
		t.Fatalf("register code: %v", err)
	}

	svc := newServiceForAPITest(t)
	handler := NewServer(svc, WithPairingCodeStore(pairing)).Handler()

	okResp := doJSONRequest(t, handler, http.MethodPost, "/api/v1/pairing/verify-consume", map[string]any{
		"code": "pair-test-code",
	})
	if okResp.Code != http.StatusOK {
		t.Fatalf("verify status = %d, want 200; body=%s", okResp.Code, okResp.Body.String())
	}

	failResp := doJSONRequest(t, handler, http.MethodPost, "/api/v1/pairing/verify-consume", map[string]any{
		"code": "pair-test-code",
	})
	if failResp.Code != http.StatusBadRequest {
		t.Fatalf("second verify status = %d, want 400; body=%s", failResp.Code, failResp.Body.String())
	}
	var env errorEnvelope
	if err := json.Unmarshal(failResp.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if env.Error.Code != "E_PAIR_CODE_INVALID" {
		t.Fatalf("unexpected error code: %q", env.Error.Code)
	}
}

func TestVerifyConsumePairCodeRejectsExpiredCode(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 2, 14, 17, 0, 0, 0, time.UTC)}
	pairing := NewPairingCodeStore(clock.Now)
	if _, err := pairing.Register("pair-expired", 1*time.Second); err != nil {
		t.Fatalf("register code: %v", err)
	}
	clock.Advance(2 * time.Second)

	svc := newServiceForAPITest(t)
	handler := NewServer(svc, WithPairingCodeStore(pairing)).Handler()

	rr := doJSONRequest(t, handler, http.MethodPost, "/api/v1/pairing/verify-consume", map[string]any{
		"code": "pair-expired",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

func TestIssuePairCodeEndpointReturnsCodeAndExpiry(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 2, 14, 17, 0, 0, 0, time.UTC)}
	pairing := NewPairingCodeStore(clock.Now)
	svc := newServiceForAPITest(t)
	handler := NewServer(svc, WithPairingCodeStore(pairing)).Handler()

	rr := doJSONRequest(t, handler, http.MethodPost, "/api/v1/pairing/codes", map[string]any{
		"ttlSeconds": 60,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var resp PairingCodeRecord
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.HasPrefix(resp.Code, "pair-") {
		t.Fatalf("expected generated pair- code, got %q", resp.Code)
	}
	if strings.TrimSpace(resp.ExpiresAt) == "" {
		t.Fatal("expected expiresAt in response")
	}
}

var _ runtimecheck.Checker = (*fakeChecker)(nil)
