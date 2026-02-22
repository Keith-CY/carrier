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

type contextCancelAwareRunner struct{}

func (contextCancelAwareRunner) Run(ctx context.Context, _ string) (commandexec.Result, error) {
	if err := ctx.Err(); err != nil {
		return commandexec.Result{ExitCode: -1}, err
	}
	return commandexec.Result{ExitCode: 0}, nil
}

func newServiceForAPITest(t *testing.T) *lifecycle.Service {
	return newServiceForAPITestWithRunner(t, nil)
}

func newServiceForAPITestWithRunner(t *testing.T, runner commandexec.Runner) *lifecycle.Service {
	t.Helper()
	if runner == nil {
		runner = &fakeRunner{}
	}
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

func TestInstallEndpointIgnoresCanceledRequestContext(t *testing.T) {
	svc := newServiceForAPITestWithRunner(t, contextCancelAwareRunner{})
	handler := NewServer(svc).Handler()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/openclaw/install", nil)
	canceledCtx, cancel := context.WithCancel(req.Context())
	cancel()
	req = req.WithContext(canceledCtx)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("install status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
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
			Start:   manifest.CommandSpec{Command: "tail -f /dev/null"},
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

	return doRawJSONRequest(t, handler, method, path, payload)
}

func doRawJSONRequest(t *testing.T, handler http.Handler, method, path string, payload []byte) *httptest.ResponseRecorder {
	t.Helper()
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
	if agent.InstallState != string(lifecycle.InstallStateNotInstalled) {
		t.Fatalf("expected installState=%q, got %q", lifecycle.InstallStateNotInstalled, agent.InstallState)
	}
	if agent.RuntimeState != string(lifecycle.RuntimeStateStopped) {
		t.Fatalf("unexpected runtime state: %q", agent.RuntimeState)
	}
	if agent.RestartCount != 0 {
		t.Fatalf("expected restartCount=0, got %d", agent.RestartCount)
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

func TestAgentCompatibilityEndpointsInstallStartStatusLogsDiagnose(t *testing.T) {
	svc := newServiceForAPITest(t)
	handler := NewServer(svc).Handler()

	install := doJSONRequest(t, handler, http.MethodPost, "/api/v1/agents/openclaw/install", nil)
	if install.Code != http.StatusOK {
		t.Fatalf("install status = %d, want 200; body=%s", install.Code, install.Body.String())
	}

	start := doJSONRequest(t, handler, http.MethodPost, "/api/v1/agents/openclaw/start", nil)
	if start.Code != http.StatusOK {
		t.Fatalf("start status = %d, want 200; body=%s", start.Code, start.Body.String())
	}

	statusOne := doJSONRequest(t, handler, http.MethodGet, "/api/v1/agents/openclaw/status", nil)
	if statusOne.Code != http.StatusOK {
		t.Fatalf("status(one) = %d, want 200; body=%s", statusOne.Code, statusOne.Body.String())
	}

	statusAll := doJSONRequest(t, handler, http.MethodGet, "/api/v1/agents/status", nil)
	if statusAll.Code != http.StatusOK {
		t.Fatalf("status(all) = %d, want 200; body=%s", statusAll.Code, statusAll.Body.String())
	}

	logs := doJSONRequest(t, handler, http.MethodGet, "/api/v1/agents/openclaw/logs?tail=20", nil)
	if logs.Code != http.StatusOK {
		t.Fatalf("logs status = %d, want 200; body=%s", logs.Code, logs.Body.String())
	}

	diagnose := doJSONRequest(t, handler, http.MethodPost, "/api/v1/agents/openclaw/diagnose", nil)
	if diagnose.Code != http.StatusOK {
		t.Fatalf("diagnose status = %d, want 200; body=%s", diagnose.Code, diagnose.Body.String())
	}
}

func TestParseAgentActionPathRejectsEncodedTraversal(t *testing.T) {
	if _, _, ok := parseAgentActionPath("/api/v1/agents/%2E%2E/start"); ok {
		t.Fatal("expected encoded traversal path to be rejected")
	}
	if _, _, ok := parseAgentActionPath("/api/v1/agents/a%2Fb/start"); ok {
		t.Fatal("expected encoded slash path to be rejected")
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

func TestVerifyConsumePairCodeRejectsTrailingJSONValues(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 2, 14, 17, 0, 0, 0, time.UTC)}
	pairing := NewPairingCodeStore(clock.Now)
	if _, err := pairing.Register("pair-test-code", 30*time.Second); err != nil {
		t.Fatalf("register code: %v", err)
	}

	svc := newServiceForAPITest(t)
	handler := NewServer(svc, WithPairingCodeStore(pairing)).Handler()

	rr := doRawJSONRequest(t, handler, http.MethodPost, "/api/v1/pairing/verify-consume", []byte(`{"code":"pair-test-code"} {"code":"pair-other"}`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}

	var env errorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if env.Error.Code != "E_USAGE" {
		t.Fatalf("unexpected error code: %q", env.Error.Code)
	}
}

func TestIssuePairCodeRejectsTrailingGarbage(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 2, 14, 17, 0, 0, 0, time.UTC)}
	pairing := NewPairingCodeStore(clock.Now)
	svc := newServiceForAPITest(t)
	handler := NewServer(svc, WithPairingCodeStore(pairing)).Handler()

	rr := doRawJSONRequest(t, handler, http.MethodPost, "/api/v1/pairing/codes", []byte(`{"ttlSeconds":60} trailing`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}

	var env errorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if env.Error.Code != "E_USAGE" {
		t.Fatalf("unexpected error code: %q", env.Error.Code)
	}
}

func TestParseAgentActionPathBoundaryCases(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantID     string
		wantAction string
		wantOK     bool
	}{
		{name: "valid simple", path: "/api/v1/agents/openclaw/start", wantID: "openclaw", wantAction: "start", wantOK: true},
		{name: "valid encoded id", path: "/api/v1/agents/openclaw%2Dbeta/status", wantID: "openclaw-beta", wantAction: "status", wantOK: true},
		{name: "reject encoded space", path: "/api/v1/agents/openclaw%20beta/start", wantOK: false},
		{name: "reject dotdot", path: "/api/v1/agents/open..claw/start", wantOK: false},
		{name: "reject missing action", path: "/api/v1/agents/openclaw/", wantOK: false},
		{name: "reject extra segment", path: "/api/v1/agents/openclaw/start/now", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotAction, gotOK := parseAgentActionPath(tt.path)
			if gotOK != tt.wantOK {
				t.Fatalf("ok = %v, want %v (id=%q action=%q)", gotOK, tt.wantOK, gotID, gotAction)
			}
			if !tt.wantOK {
				return
			}
			if gotID != tt.wantID || gotAction != tt.wantAction {
				t.Fatalf("parseAgentActionPath(%q) = (%q, %q), want (%q, %q)", tt.path, gotID, gotAction, tt.wantID, tt.wantAction)
			}
		})
	}
}

func TestVerifyConsumePairCodeRejectsOversizedPayload(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 2, 14, 17, 0, 0, 0, time.UTC)}
	pairing := NewPairingCodeStore(clock.Now)
	if _, err := pairing.Register("pair-test-code", 30*time.Second); err != nil {
		t.Fatalf("register code: %v", err)
	}

	svc := newServiceForAPITest(t)
	handler := NewServer(svc, WithPairingCodeStore(pairing)).Handler()
	oversizedCode := strings.Repeat("x", (1<<20)+32)
	rr := doRawJSONRequest(t, handler, http.MethodPost, "/api/v1/pairing/verify-consume", []byte(`{"code":"`+oversizedCode+`"}`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}

	var env errorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if env.Error.Code != "E_USAGE" {
		t.Fatalf("unexpected error code: %q", env.Error.Code)
	}
	if !strings.Contains(strings.ToLower(env.Error.Message), "too large") {
		t.Fatalf("expected oversized payload error message, got %q", env.Error.Message)
	}
}

func TestAuditLogsEndpointSupportsFilteringAndLimit(t *testing.T) {
	svc := newServiceForAPITest(t)
	handler := NewServer(svc).Handler()

	// Generate two failure audit records with explicit request IDs.
	for _, reqID := range []string{"req-audit-1", "req-audit-2"} {
		rr := doJSONRequest(t, handler, http.MethodPost, "/api/v1/diagnosis/handoffs", map[string]any{
			"agentId":   "openclaw",
			"consent":   true,
			"actor":     "telegram:100",
			"requestId": reqID,
		})
		if rr.Code != http.StatusConflict {
			t.Fatalf("handoff status = %d, want 409; body=%s", rr.Code, rr.Body.String())
		}
	}

	filtered := doJSONRequest(t, handler, http.MethodGet, "/api/v1/audit/logs?action=remote_diagnosis_consent&actor=telegram:100&request_id=req-audit-1&result=failure", nil)
	if filtered.Code != http.StatusOK {
		t.Fatalf("filtered audit status = %d, want 200; body=%s", filtered.Code, filtered.Body.String())
	}

	var filteredResp struct {
		AuditLogs []struct {
			RequestID string `json:"requestId"`
			Actor     string `json:"actor"`
			Action    string `json:"action"`
			Result    string `json:"result"`
			ErrorCode string `json:"errorCode"`
		} `json:"auditLogs"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(filtered.Body.Bytes(), &filteredResp); err != nil {
		t.Fatalf("decode filtered response: %v", err)
	}
	if filteredResp.Total != 1 || len(filteredResp.AuditLogs) != 1 {
		t.Fatalf("expected exactly 1 filtered audit log, got total=%d len=%d", filteredResp.Total, len(filteredResp.AuditLogs))
	}
	log := filteredResp.AuditLogs[0]
	if log.RequestID != "req-audit-1" || log.Action != "remote_diagnosis_consent" || log.Actor != "telegram:100" {
		t.Fatalf("unexpected filtered audit log: %#v", log)
	}
	if log.Result != "failure" || log.ErrorCode != "E_REMOTE_DIAG_NOT_NEEDED" {
		t.Fatalf("unexpected filtered audit result/code: %#v", log)
	}

	limited := doJSONRequest(t, handler, http.MethodGet, "/api/v1/audit/logs?action=remote_diagnosis_consent&limit=1", nil)
	if limited.Code != http.StatusOK {
		t.Fatalf("limited audit status = %d, want 200; body=%s", limited.Code, limited.Body.String())
	}
	var limitedResp struct {
		AuditLogs []struct {
			RequestID string `json:"requestId"`
		} `json:"auditLogs"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(limited.Body.Bytes(), &limitedResp); err != nil {
		t.Fatalf("decode limited response: %v", err)
	}
	if limitedResp.Total != 2 {
		t.Fatalf("expected total=2, got %d", limitedResp.Total)
	}
	if len(limitedResp.AuditLogs) != 1 || limitedResp.AuditLogs[0].RequestID != "req-audit-2" {
		t.Fatalf("expected only latest audit record req-audit-2, got %#v", limitedResp.AuditLogs)
	}
}

func TestAuditLogsEndpointRejectsInvalidResultFilter(t *testing.T) {
	svc := newServiceForAPITest(t)
	handler := NewServer(svc).Handler()

	rr := doJSONRequest(t, handler, http.MethodGet, "/api/v1/audit/logs?result=invalid", nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	var env errorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if env.Error.Code != "E_USAGE" {
		t.Fatalf("unexpected error code: %q", env.Error.Code)
	}
}

func TestDecodeBodyAllowsUnknownFields(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 2, 14, 17, 0, 0, 0, time.UTC)}
	pairing := NewPairingCodeStore(clock.Now)
	svc := newServiceForAPITest(t)
	handler := NewServer(svc, WithPairingCodeStore(pairing)).Handler()

	// Unknown fields should be silently ignored for forward compatibility.
	// The request should proceed to normal validation (code is invalid).
	rr := doRawJSONRequest(t, handler, http.MethodPost, "/api/v1/pairing/verify-consume",
		[]byte(`{"code":"test","extra":"unknown"}`))
	// Should NOT be a JSON parse error — the unknown field is tolerated.
	// It should reach pairing validation and fail there instead.
	body := rr.Body.String()
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (validation failure); body=%s", rr.Code, body)
	}
	if strings.Contains(body, "unknown field") {
		t.Fatalf("decodeBody should allow unknown fields for forward compat, got: %s", body)
	}
}

var _ runtimecheck.Checker = (*fakeChecker)(nil)
