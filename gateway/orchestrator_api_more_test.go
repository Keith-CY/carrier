package gateway

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHandleOrchestratorPlans(t *testing.T) {
	var gotDecomposeBody string
	mux := buildRemoteFeatureMuxWithDaemonHandlers(t, map[string]http.HandlerFunc{
		"POST /api/v1/base-agent/decompose": func(w http.ResponseWriter, r *http.Request) {
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read decompose body: %v", err)
			}
			gotDecomposeBody = strings.TrimSpace(string(raw))
			_, _ = w.Write([]byte(`{"tasks":[{"id":"task-1","input":"collect diagnostics","agentId":"zeroclaw"},{"id":"task-2","input":"summarize diagnostics","agentId":"picoclaw"}]}`))
		},
	})

	rec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/plans", `{
		"goal":"triage live issue",
		"provider":"openrouter",
		"hostIds":["host-a","host-a","host-b"],
		"maxConcurrency":9
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected planning status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(gotDecomposeBody, `"provider":"openrouter"`) {
		t.Fatalf("expected provider to be forwarded, got body=%s", gotDecomposeBody)
	}

	payload := decodeJSONMap(t, rec)
	plan, _ := payload["plan"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(plan["goal"])); got != "triage live issue" {
		t.Fatalf("goal = %q, want triage live issue payload=%+v", got, payload)
	}
	if got := strings.TrimSpace(anyToString(plan["approvalScope"])); got != "infrastructure_only" {
		t.Fatalf("approvalScope = %q, want infrastructure_only", got)
	}
	if got := int(anyToFloat(plan["maxConcurrency"])); got != 2 {
		t.Fatalf("maxConcurrency = %d, want 2", got)
	}
	taskUnits, _ := plan["taskUnits"].([]interface{})
	if len(taskUnits) != 2 {
		t.Fatalf("taskUnits = %d, want 2", len(taskUnits))
	}
}

func TestHandleOrchestratorPlansNegativeCases(t *testing.T) {
	disabledMux := buildRemoteFeatureMuxWithConfigAndDaemonHandlers(t, &GatewayConfig{
		APIToken:                  "test-gateway-token",
		MaxCommandBodyBytes:       64 * 1024,
		RemoteControlPlaneEnabled: false,
		RemoteChatEnabled:         true,
		ProviderBindingEnabled:    true,
	}, nil)
	disabled := runJSONRequest(t, disabledMux, http.MethodPost, "/api/v1/orchestrator/plans", `{"goal":"triage"}`)
	if disabled.Code != http.StatusNotFound {
		t.Fatalf("expected disabled planning status 404, got %d body=%s", disabled.Code, disabled.Body.String())
	}

	mux := buildRemoteFeatureMux(t)
	methodRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/orchestrator/plans", "")
	if methodRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected method not allowed, got %d body=%s", methodRec.Code, methodRec.Body.String())
	}

	badJSON := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/plans", "{")
	if badJSON.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request for invalid json, got %d body=%s", badJSON.Code, badJSON.Body.String())
	}

	emptyGoal := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/plans", `{"goal":" "}`)
	if emptyGoal.Code != http.StatusBadRequest {
		t.Fatalf("expected validation error for empty goal, got %d body=%s", emptyGoal.Code, emptyGoal.Body.String())
	}
}

func TestOrchestratorExecutionEndpointNegativeCases(t *testing.T) {
	mux := buildRemoteFeatureMux(t)
	hostID := createRemoteHostForTests(t, mux)

	methodRec := runJSONRequest(t, mux, http.MethodPut, "/api/v1/orchestrator/executions", "")
	if methodRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected method not allowed, got %d body=%s", methodRec.Code, methodRec.Body.String())
	}

	badJSON := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions", "{")
	if badJSON.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request for invalid json, got %d body=%s", badJSON.Code, badJSON.Body.String())
	}

	invalidReq := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions", `{"goal":"","requiredWorkers":[],"taskUnits":[]}`)
	if invalidReq.Code != http.StatusBadRequest {
		t.Fatalf("expected validation error status 400, got %d body=%s", invalidReq.Code, invalidReq.Body.String())
	}

	createRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions", `{
		"goal":"negative-paths",
		"requiredWorkers":[{"hostId":"`+hostID+`","agentId":"zeroclaw","count":1}],
		"taskUnits":[{"id":"t1","input":"hello"}],
		"approvalScope":"infrastructure_only"
	}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create execution status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	createPayload := decodeJSONMap(t, createRec)
	execMap, _ := createPayload["execution"].(map[string]interface{})
	execID := strings.TrimSpace(anyToString(execMap["id"]))
	if execID == "" {
		t.Fatalf("missing execution id in create response: %+v", createPayload)
	}

	getRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/orchestrator/executions", "")
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected list execution status 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}

	notFound := runJSONRequest(t, mux, http.MethodGet, "/api/v1/orchestrator/executions/missing-id", "")
	if notFound.Code != http.StatusNotFound {
		t.Fatalf("expected missing execution status 404, got %d body=%s", notFound.Code, notFound.Body.String())
	}

	unsupportedAction := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions/"+execID+"/unknown", `{}`)
	if unsupportedAction.Code != http.StatusNotFound {
		t.Fatalf("expected unsupported action status 404, got %d body=%s", unsupportedAction.Code, unsupportedAction.Body.String())
	}

	itemMethod := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions/"+execID, `{}`)
	if itemMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected item method not allowed, got %d body=%s", itemMethod.Code, itemMethod.Body.String())
	}

	authMethod := runJSONRequest(t, mux, http.MethodGet, "/api/v1/orchestrator/executions/"+execID+"/authorize", "")
	if authMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected authorize method not allowed, got %d body=%s", authMethod.Code, authMethod.Body.String())
	}

	cancelMethod := runJSONRequest(t, mux, http.MethodGet, "/api/v1/orchestrator/executions/"+execID+"/cancel", "")
	if cancelMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected cancel method not allowed, got %d body=%s", cancelMethod.Code, cancelMethod.Body.String())
	}

	authBadJSON := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions/"+execID+"/authorize", "{")
	if authBadJSON.Code != http.StatusBadRequest {
		t.Fatalf("expected authorize bad json status 400, got %d body=%s", authBadJSON.Code, authBadJSON.Body.String())
	}

	declineRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions/"+execID+"/authorize", `{"approved":false,"actor":" "}`)
	if declineRec.Code != http.StatusOK {
		t.Fatalf("expected decline status 200, got %d body=%s", declineRec.Code, declineRec.Body.String())
	}
	declinePayload := decodeJSONMap(t, declineRec)
	declinedExec, _ := declinePayload["execution"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(declinedExec["status"])); got != string(OrchestratorExecutionStatusDeclined) {
		t.Fatalf("expected declined status, got %q payload=%+v", got, declinePayload)
	}
	auth, _ := declinedExec["authorization"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(auth["approvedBy"])); got != "operator" {
		t.Fatalf("expected default actor operator, got %q auth=%+v", got, auth)
	}

	terminalApprove := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions/"+execID+"/authorize", `{"approved":true,"actor":"tester"}`)
	if terminalApprove.Code != http.StatusOK {
		t.Fatalf("expected terminal authorize status 200, got %d body=%s", terminalApprove.Code, terminalApprove.Body.String())
	}
}

func TestHandleOrchestratorExecutionsCancel(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))

	cfg := &GatewayConfig{
		RemoteControlPlaneEnabled: true,
	}
	seed, err := upsertOrchestratorExecution(OrchestratorExecution{
		ID:            "exec-cancel-1",
		Goal:          "cancel me",
		ApprovalScope: "infrastructure_only",
		Status:        OrchestratorExecutionStatusRunning,
		Authorization: OrchestratorAuthorization{
			InfrastructureApproved: true,
			ApprovedBy:             "tester",
			ApprovedAt:             nowTimestamp(),
		},
		RequiredWorkers: []OrchestratorRequiredWorker{{HostID: orchestratorLocalHostID, AgentID: "zeroclaw", Count: 1}},
		TaskUnits:       []OrchestratorTaskUnit{{ID: "task-1", Input: "noop"}},
		StartedAt:       nowTimestamp(),
	})
	if err != nil {
		t.Fatalf("seed execution failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/orchestrator/executions/"+seed.ID+"/cancel", strings.NewReader(`{"actor":"operator-ui"}`))
	rec := httptest.NewRecorder()
	handleOrchestratorExecutions(rec, req, "req-cancel-1", cfg)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected cancel accepted status, got %d body=%s", rec.Code, rec.Body.String())
	}

	payload := decodeJSONMap(t, rec)
	execMap, _ := payload["execution"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(execMap["status"])); got != string(OrchestratorExecutionStatusCancelled) {
		t.Fatalf("expected cancelled status, got %q payload=%+v", got, payload)
	}

	updated, found, err := getOrchestratorExecution(seed.ID)
	if err != nil || !found {
		t.Fatalf("get cancelled execution failed found=%v err=%v", found, err)
	}
	if updated.Status != OrchestratorExecutionStatusCancelled {
		t.Fatalf("expected persisted cancelled status, got %+v", updated)
	}
	if updated.CompletedAt == "" {
		t.Fatalf("expected cancelled execution to set completedAt, got %+v", updated)
	}
	if !strings.Contains(updated.Error, "operator-ui") {
		t.Fatalf("expected cancellation reason to include actor, got %+v", updated)
	}
}

func TestOrchestratorWorkersReclaimEndpointNegativeCases(t *testing.T) {
	disabledMux := buildRemoteFeatureMuxWithConfigAndDaemonHandlers(t, &GatewayConfig{
		APIToken:                  "test-gateway-token",
		MaxCommandBodyBytes:       64 * 1024,
		RemoteControlPlaneEnabled: false,
		RemoteChatEnabled:         true,
		ProviderBindingEnabled:    true,
	}, nil)
	disabledRec := runJSONRequest(t, disabledMux, http.MethodPost, "/api/v1/orchestrator/workers/reclaim", `{}`)
	if disabledRec.Code != http.StatusNotFound {
		t.Fatalf("expected disabled reclaim status 404, got %d body=%s", disabledRec.Code, disabledRec.Body.String())
	}

	mux := buildRemoteFeatureMux(t)
	methodRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/orchestrator/workers/reclaim", "")
	if methodRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected reclaim method not allowed, got %d body=%s", methodRec.Code, methodRec.Body.String())
	}

	badJSON := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/workers/reclaim", "{")
	if badJSON.Code != http.StatusBadRequest {
		t.Fatalf("expected reclaim bad json 400, got %d body=%s", badJSON.Code, badJSON.Body.String())
	}

	// Force store read failure to cover error response branch.
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", t.TempDir())
	storeErr := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/workers/reclaim", `{"force":true}`)
	if storeErr.Code != http.StatusBadGateway {
		t.Fatalf("expected reclaim store error status 502, got %d body=%s", storeErr.Code, storeErr.Body.String())
	}
}

func TestRunOrchestratorExecutionFailureAndMarkFailed(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))

	saved, err := upsertOrchestratorExecution(OrchestratorExecution{
		ID:              "exec-fail-1",
		Goal:            "fail on missing host",
		ApprovalScope:   "infrastructure_only",
		Status:          OrchestratorExecutionStatusProvisioning,
		Authorization:   OrchestratorAuthorization{InfrastructureApproved: true},
		RequiredWorkers: []OrchestratorRequiredWorker{{HostID: "missing-host", AgentID: "zeroclaw", Count: 1}},
		TaskUnits:       []OrchestratorTaskUnit{{ID: "task-1", Input: "hello"}},
	})
	if err != nil {
		t.Fatalf("seed execution failed: %v", err)
	}

	runOrchestratorExecution(saved.ID)

	got, found, err := getOrchestratorExecution(saved.ID)
	if err != nil || !found {
		t.Fatalf("getOrchestratorExecution failed found=%v err=%v", found, err)
	}
	if got.Status != OrchestratorExecutionStatusFailed {
		t.Fatalf("expected execution failed status, got %q execution=%+v", got.Status, got)
	}
	if got.CompletedAt == "" || got.Error == "" || !strings.Contains(strings.ToLower(got.Error), "not found") {
		t.Fatalf("expected failure details to be persisted, got %+v", got)
	}

	seed, err := upsertOrchestratorExecution(OrchestratorExecution{
		ID:              "exec-fail-2",
		Goal:            "manual mark",
		ApprovalScope:   "infrastructure_only",
		Status:          OrchestratorExecutionStatusRunning,
		Authorization:   OrchestratorAuthorization{InfrastructureApproved: true},
		RequiredWorkers: []OrchestratorRequiredWorker{{HostID: "h", AgentID: "zeroclaw", Count: 1}},
		TaskUnits:       []OrchestratorTaskUnit{{ID: "task-1", Input: "hello"}},
	})
	if err != nil {
		t.Fatalf("seed second execution failed: %v", err)
	}
	results := []OrchestratorTaskResult{{
		TaskID:   "task-1",
		Status:   OrchestratorTaskStatusFailed,
		Error:    "boom",
		Attempts: 1,
	}}
	markOrchestratorExecutionFailed(seed, errors.New("run failed"), results)

	updated, found, err := getOrchestratorExecution(seed.ID)
	if err != nil || !found {
		t.Fatalf("get marked execution failed found=%v err=%v", found, err)
	}
	if updated.Status != OrchestratorExecutionStatusFailed || updated.Error != "run failed" {
		t.Fatalf("expected marked failed execution, got %+v", updated)
	}
	if len(updated.Results) != 1 || updated.Results[0].TaskID != "task-1" {
		t.Fatalf("expected failure results to be persisted, got %+v", updated.Results)
	}
}

func TestStartOrchestratorExecutionAsyncAndWorkerPoolHelpers(t *testing.T) {
	startOrchestratorExecutionAsync(" ")

	orchestratorExecutionRunState.mu.Lock()
	orchestratorExecutionRunState.running["dup-id"] = true
	orchestratorExecutionRunState.mu.Unlock()
	t.Cleanup(func() {
		orchestratorExecutionRunState.mu.Lock()
		delete(orchestratorExecutionRunState.running, "dup-id")
		orchestratorExecutionRunState.mu.Unlock()
	})

	startOrchestratorExecutionAsync("dup-id")

	orchestratorExecutionRunState.mu.Lock()
	_, stillRunning := orchestratorExecutionRunState.running["dup-id"]
	orchestratorExecutionRunState.mu.Unlock()
	if !stillRunning {
		t.Fatal("expected duplicate start to keep existing running marker")
	}

	startOrchestratorExecutionAsync("missing-exec")
	for i := 0; i < 40; i++ {
		orchestratorExecutionRunState.mu.Lock()
		_, running := orchestratorExecutionRunState.running["missing-exec"]
		orchestratorExecutionRunState.mu.Unlock()
		if !running {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	orchestratorExecutionRunState.mu.Lock()
	_, running := orchestratorExecutionRunState.running["missing-exec"]
	orchestratorExecutionRunState.mu.Unlock()
	if running {
		t.Fatal("expected async runner to clear running marker")
	}

	if _, _, err := acquireWorkerForTask(context.Background(), OrchestratorTaskUnit{
		HostID:  "host-1",
		AgentID: "zeroclaw",
	}, map[string]orchestratorWorkerPool{}, ""); err == nil {
		t.Fatal("expected acquireWorkerForTask to fail when pool is missing")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	key := workerPoolKey("host-1", "zeroclaw")
	pool := orchestratorWorkerPool{
		key: key,
		ch:  make(chan OrchestratorWorkerLease, 1),
	}
	if _, _, err := acquireWorkerForTask(ctx, OrchestratorTaskUnit{
		HostID:  "host-1",
		AgentID: "zeroclaw",
	}, map[string]orchestratorWorkerPool{key: pool}, key); err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled from acquireWorkerForTask, got %v", err)
	}

	fullPool := orchestratorWorkerPool{key: "k", ch: make(chan OrchestratorWorkerLease, 1)}
	fullPool.ch <- OrchestratorWorkerLease{ID: "existing"}
	releaseWorkerToPool(OrchestratorWorkerLease{ID: "ignored", State: OrchestratorWorkerStateReady}, fullPool)
	if got := len(fullPool.ch); got != 1 {
		t.Fatalf("expected full channel size to remain 1, got %d", got)
	}

	emptyPool := orchestratorWorkerPool{key: "k2", ch: make(chan OrchestratorWorkerLease, 1)}
	releaseWorkerToPool(OrchestratorWorkerLease{ID: "reclaimed", State: OrchestratorWorkerStateReclaimed}, emptyPool)
	if got := len(emptyPool.ch); got != 0 {
		t.Fatalf("expected reclaimed lease not to be returned to pool, got channel len %d", got)
	}
}

func TestRunTaskWithRetriesAndRunOrchestratorTasksNegativePaths(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))

	result, err := runTaskWithRetries(context.Background(), OrchestratorExecution{ID: "exec-1"}, OrchestratorTaskUnit{
		ID:          "task-1",
		RetryBudget: -1,
	}, map[string]orchestratorWorkerPool{}, "")
	if err == nil || !strings.Contains(err.Error(), "unknown error") {
		t.Fatalf("expected unknown-error fallback from runTaskWithRetries, err=%v result=%+v", err, result)
	}

	runErrResult, runErr := runOrchestratorTasks(context.Background(), OrchestratorExecution{
		ID:             "exec-1",
		MaxConcurrency: 0,
		TaskUnits:      []OrchestratorTaskUnit{{ID: "task-1", Input: "hello"}},
	}, nil)
	if runErr == nil || !strings.Contains(runErr.Error(), "no available workers") {
		t.Fatalf("expected no available workers error, err=%v results=%+v", runErr, runErrResult)
	}
}

func TestReclaimOrchestratorLeaseSetBranches(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))

	configureSSHRunner(t, func(command string) remoteExecResult {
		if strings.Contains(command, `rm -f "$HOME/.zeroclaw/config.toml"`) {
			return remoteExecResult{ExitCode: 1, Stderr: "uninstall failed"}
		}
		return remoteExecResult{ExitCode: 0}
	})

	host, err := upsertRemoteHost(RemoteHost{
		Name:        "host-1",
		Host:        "127.0.0.1",
		Port:        22,
		User:        "ubuntu",
		AuthMode:    RemoteAuthModePrivateKey,
		KeyPath:     "~/.ssh/id_ed25519",
		RuntimeMode: RemoteRuntimeModeOnDemand,
	})
	if err != nil {
		t.Fatalf("upsertRemoteHost failed: %v", err)
	}

	old := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339Nano)
	recent := time.Now().UTC().Add(-10 * time.Second).Format(time.RFC3339Nano)
	leases := []OrchestratorWorkerLease{
		{
			ID:          "lease-reclaimed",
			ExecutionID: "exec-1",
			HostID:      host.ID,
			AgentID:     "zeroclaw",
			State:       OrchestratorWorkerStateReclaimed,
			Ephemeral:   true,
			HeartbeatAt: old,
		},
		{
			ID:          "lease-non-ephemeral",
			ExecutionID: "exec-1",
			HostID:      host.ID,
			AgentID:     "zeroclaw",
			State:       OrchestratorWorkerStateReady,
			Ephemeral:   false,
			HeartbeatAt: old,
		},
		{
			ID:          "lease-recent",
			ExecutionID: "exec-1",
			HostID:      host.ID,
			AgentID:     "zeroclaw",
			State:       OrchestratorWorkerStateReady,
			Ephemeral:   true,
			HeartbeatAt: recent,
		},
		{
			ID:          "lease-missing-host",
			ExecutionID: "exec-1",
			HostID:      "host-missing",
			AgentID:     "zeroclaw",
			State:       OrchestratorWorkerStateReady,
			Ephemeral:   true,
			HeartbeatAt: old,
		},
		{
			ID:          "lease-uninstall-fail",
			ExecutionID: "exec-1",
			HostID:      host.ID,
			AgentID:     "zeroclaw",
			State:       OrchestratorWorkerStateReady,
			Ephemeral:   true,
			HeartbeatAt: old,
		},
	}
	for _, lease := range leases {
		if _, err := upsertOrchestratorWorkerLease(lease); err != nil {
			t.Fatalf("seed lease %s failed: %v", lease.ID, err)
		}
	}

	summary, reclaimErr := reclaimOrchestratorLeaseSet(context.Background(), leases, false, 30*time.Second)
	if reclaimErr == nil || !strings.Contains(reclaimErr.Error(), "1 worker reclaim operations failed") {
		t.Fatalf("expected reclaim error summary, err=%v summary=%+v", reclaimErr, summary)
	}
	if summary["reclaimed"] != 1 || summary["skipped"] != 3 || summary["failed"] != 1 {
		t.Fatalf("unexpected reclaim summary %+v", summary)
	}

	all, err := listOrchestratorWorkerLeases()
	if err != nil {
		t.Fatalf("listOrchestratorWorkerLeases failed: %v", err)
	}
	stateByID := map[string]OrchestratorWorkerLease{}
	for _, lease := range all {
		stateByID[lease.ID] = lease
	}
	if stateByID["lease-missing-host"].State != OrchestratorWorkerStateReclaimed {
		t.Fatalf("expected missing-host lease reclaimed, got %+v", stateByID["lease-missing-host"])
	}
	if stateByID["lease-uninstall-fail"].State != OrchestratorWorkerStateError || stateByID["lease-uninstall-fail"].LastError == "" {
		t.Fatalf("expected uninstall-fail lease in error state, got %+v", stateByID["lease-uninstall-fail"])
	}
}

func TestParseRFC3339OrNowAndRemoteAgentConfigExists(t *testing.T) {
	now := time.Now().UTC()
	parsed := parseRFC3339OrNow("2026-03-05T01:02:03Z")
	if parsed.Format(time.RFC3339Nano) != "2026-03-05T01:02:03Z" {
		t.Fatalf("unexpected parsed time %s", parsed.Format(time.RFC3339Nano))
	}

	invalid := parseRFC3339OrNow("not-a-time")
	if invalid.Before(now.Add(-2*time.Second)) || invalid.After(time.Now().UTC().Add(2*time.Second)) {
		t.Fatalf("expected invalid timestamp fallback near now, got %s", invalid.Format(time.RFC3339Nano))
	}

	empty := parseRFC3339OrNow(" ")
	if empty.Before(now.Add(-2*time.Second)) || empty.After(time.Now().UTC().Add(2*time.Second)) {
		t.Fatalf("expected empty timestamp fallback near now, got %s", empty.Format(time.RFC3339Nano))
	}

	configureSSHRunner(t, func(command string) remoteExecResult {
		switch {
		case strings.Contains(command, ".zeroclaw/config.toml"):
			return remoteExecResult{ExitCode: 0, Stdout: "1\n"}
		case strings.Contains(command, ".picoclaw/config.toml"):
			return remoteExecResult{ExitCode: 0, Stdout: "0\n"}
		case strings.Contains(command, ".openclaw/openclaw.json"):
			return remoteExecResult{ExitCode: 1, Stderr: "permission denied"}
		default:
			return remoteExecResult{ExitCode: 0, Stdout: "0\n"}
		}
	})

	host := RemoteHost{
		Host:     "127.0.0.1",
		Port:     22,
		User:     "ubuntu",
		AuthMode: RemoteAuthModePrivateKey,
		KeyPath:  "~/.ssh/id_ed25519",
	}
	exists, err := remoteAgentConfigExists(context.Background(), host, "zeroclaw")
	if err != nil || !exists {
		t.Fatalf("expected zeroclaw config to exist, exists=%v err=%v", exists, err)
	}
	exists, err = remoteAgentConfigExists(context.Background(), host, "picoclaw")
	if err != nil || exists {
		t.Fatalf("expected picoclaw config to not exist, exists=%v err=%v", exists, err)
	}
	if _, err := remoteAgentConfigExists(context.Background(), host, "openclaw"); err == nil {
		t.Fatal("expected openclaw config check to fail on non-zero exit")
	}
}

func TestRemoteRunPayloadAndZeroClawFallback(t *testing.T) {
	contract := buildRemoteMemoryRuntimeContract("zeroclaw")

	noJSON := parseRemoteRunPayload("plain output", contract)
	if strings.TrimSpace(anyToString(noJSON["message"])) != "plain output" {
		t.Fatalf("expected plain output message, got %+v", noJSON)
	}
	if _, ok := noJSON["memory"].(map[string]interface{}); !ok {
		t.Fatalf("expected memory block for plain output payload: %+v", noJSON)
	}

	badJSON := parseRemoteRunPayload("{not-json}", contract)
	if strings.TrimSpace(anyToString(badJSON["message"])) == "" {
		t.Fatalf("expected fallback message for bad json payload: %+v", badJSON)
	}

	noMemory := parseRemoteRunPayload(`{"sessionId":"s-1","output_text":"hello"}`, contract)
	mem, ok := noMemory["memory"].(map[string]interface{})
	if !ok || strings.TrimSpace(anyToString(mem["contractDigest"])) == "" {
		t.Fatalf("expected synthesized memory for payload without memory block: %+v", noMemory)
	}

	withMemory := parseRemoteRunPayload(`{"sessionId":"s-2","memory":{"syncState":"ready","contractId":"x"}}`, contract)
	mem, ok = withMemory["memory"].(map[string]interface{})
	if !ok || strings.TrimSpace(anyToString(mem["contractId"])) != "x" {
		t.Fatalf("expected existing memory to be preserved: %+v", withMemory)
	}

	host := RemoteHost{
		Host:     "127.0.0.1",
		Port:     22,
		User:     "ubuntu",
		AuthMode: RemoteAuthModePrivateKey,
		KeyPath:  "~/.ssh/id_ed25519",
	}

	if _, _, err := remoteRunViaZeroClaw(context.Background(), host, "h1", "bad id", "hello", "sess-1"); err == nil {
		t.Fatal("expected invalid agent id to fail")
	}
	if _, _, err := remoteRunViaZeroClaw(context.Background(), host, "h1", "zeroclaw", " ", "sess-1"); err == nil {
		t.Fatal("expected empty message to fail")
	}

	t.Run("fallback-success", func(t *testing.T) {
		configureSSHRunner(t, func(command string) remoteExecResult {
			switch {
			case strings.Contains(command, "zeroclaw task run"),
				strings.Contains(command, "zeroclaw run --message"),
				strings.Contains(command, "zeroclaw agent --message"):
				return remoteExecResult{ExitCode: 1, Stderr: "zeroclaw failed"}
			case strings.Contains(command, "openclaw agent --local"):
				return remoteExecResult{ExitCode: 0, Stdout: `{"sessionId":"sess-open","output_text":"openclaw-output"}`}
			default:
				return remoteExecResult{ExitCode: 0}
			}
		})

		runResult, steps, err := remoteRunViaZeroClaw(context.Background(), host, "host-1", "zeroclaw", "hello", "sess-1")
		if err != nil {
			t.Fatalf("expected fallback success, got error: %v", err)
		}
		if runResult == nil || strings.TrimSpace(runResult.Output) != "openclaw-output" {
			t.Fatalf("expected fallback output openclaw-output, got %+v", runResult)
		}
		if len(steps) < 4 {
			t.Fatalf("expected zeroclaw attempts + fallback step, got %d steps", len(steps))
		}
	})

	t.Run("fallback-failed", func(t *testing.T) {
		configureSSHRunner(t, func(command string) remoteExecResult {
			switch {
			case strings.Contains(command, "zeroclaw task run"),
				strings.Contains(command, "zeroclaw run --message"),
				strings.Contains(command, "zeroclaw agent --message"),
				strings.Contains(command, "openclaw agent --local"):
				return remoteExecResult{ExitCode: 1, Stderr: "all failed"}
			default:
				return remoteExecResult{ExitCode: 0}
			}
		})

		if _, steps, err := remoteRunViaZeroClaw(context.Background(), host, "host-1", "zeroclaw", "hello", "sess-1"); err == nil || !strings.Contains(err.Error(), "fallback failed") {
			t.Fatalf("expected fallback failure error, err=%v steps=%d", err, len(steps))
		}
	})
}

func TestRunOrchestratorTaskAttemptHostAndExecutionErrors(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))

	missingHostResult, missingHostErr := runOrchestratorTaskAttempt(context.Background(), OrchestratorExecution{ID: "exec-1"}, OrchestratorTaskUnit{
		ID:    "task-1",
		Input: "hello",
	}, OrchestratorWorkerLease{
		ID:      "lease-1",
		HostID:  "missing-host",
		AgentID: "zeroclaw",
	}, 1)
	if missingHostErr == nil || !strings.Contains(missingHostErr.Error(), "not found") {
		t.Fatalf("expected missing host error, err=%v result=%+v", missingHostErr, missingHostResult)
	}

	host, err := upsertRemoteHost(RemoteHost{
		Name:        "host-1",
		Host:        "127.0.0.1",
		Port:        22,
		User:        "ubuntu",
		AuthMode:    RemoteAuthModePrivateKey,
		KeyPath:     "~/.ssh/id_ed25519",
		RuntimeMode: RemoteRuntimeModeOnDemand,
	})
	if err != nil {
		t.Fatalf("upsertRemoteHost failed: %v", err)
	}

	t.Run("run-error", func(t *testing.T) {
		configureSSHRunner(t, func(command string) remoteExecResult {
			switch {
			case strings.Contains(command, "zeroclaw task run"),
				strings.Contains(command, "zeroclaw run --message"),
				strings.Contains(command, "zeroclaw agent --message"),
				strings.Contains(command, "openclaw agent --local"):
				return remoteExecResult{ExitCode: 1, Stderr: "run failed"}
			default:
				return remoteExecResult{ExitCode: 0}
			}
		})

		result, runErr := runOrchestratorTaskAttempt(context.Background(), OrchestratorExecution{ID: "exec-1"}, OrchestratorTaskUnit{
			ID:          "task-1",
			Input:       "hello",
			TimeoutMs:   999999,
			RetryBudget: 0,
		}, OrchestratorWorkerLease{
			ID:      "lease-1",
			HostID:  host.ID,
			AgentID: "zeroclaw",
		}, 1)
		if runErr == nil {
			t.Fatalf("expected run error, got result %+v", result)
		}
		if result.Status != OrchestratorTaskStatusFailed || result.StartedAt == "" || result.CompletedAt == "" {
			t.Fatalf("expected failed result with timing fields, got %+v", result)
		}
	})

	t.Run("run-success", func(t *testing.T) {
		configureSSHRunner(t, func(command string) remoteExecResult {
			switch {
			case strings.Contains(command, "zeroclaw task run"):
				return remoteExecResult{ExitCode: 0, Stdout: `{"sessionId":"sess-1","output_text":"ok"}`}
			default:
				return remoteExecResult{ExitCode: 0}
			}
		})

		result, runErr := runOrchestratorTaskAttempt(context.Background(), OrchestratorExecution{ID: "exec-1"}, OrchestratorTaskUnit{
			ID:        "task-1",
			Input:     "hello",
			SessionID: " ",
			TimeoutMs: -1,
		}, OrchestratorWorkerLease{
			ID:      "lease-1",
			HostID:  host.ID,
			AgentID: "zeroclaw",
		}, 2)
		if runErr != nil {
			t.Fatalf("expected run success, got error: %v", runErr)
		}
		if result.Status != OrchestratorTaskStatusCompleted || strings.TrimSpace(result.Output) != "ok" || result.Attempts != 2 {
			t.Fatalf("unexpected success result %+v", result)
		}
	})
}

func TestHandleOrchestratorExecutionsErrorBranches(t *testing.T) {
	cfg := &GatewayConfig{
		RemoteControlPlaneEnabled: true,
	}

	t.Run("feature-disabled", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/orchestrator/executions", nil)
		rec := httptest.NewRecorder()
		handleOrchestratorExecutions(rec, req, "req-1", &GatewayConfig{RemoteControlPlaneEnabled: false})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 when feature disabled, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("list-store-error", func(t *testing.T) {
		t.Setenv("CARRIER_REMOTE_CONTROL_STORE", t.TempDir())
		req := httptest.NewRequest(http.MethodGet, "/api/v1/orchestrator/executions", nil)
		rec := httptest.NewRecorder()
		handleOrchestratorExecutions(rec, req, "req-2", cfg)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected list store error status 500, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("idempotency-lookup-error", func(t *testing.T) {
		t.Setenv("CARRIER_REMOTE_CONTROL_STORE", t.TempDir())
		req := httptest.NewRequest(http.MethodPost, "/api/v1/orchestrator/executions", strings.NewReader(`{
			"goal":"g",
			"idempotencyKey":"idem-1",
			"requiredWorkers":[{"hostId":"h1","agentId":"zeroclaw","count":1}],
			"taskUnits":[{"id":"t1","input":"hello"}],
			"approvalScope":"infrastructure_only"
		}`))
		rec := httptest.NewRecorder()
		handleOrchestratorExecutions(rec, req, "req-3", cfg)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected idempotency lookup error status 500, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("create-save-error", func(t *testing.T) {
		storePath := filepath.Join(t.TempDir(), "remote-control.json")
		t.Setenv("CARRIER_REMOTE_CONTROL_STORE", storePath)
		if _, err := upsertOrchestratorExecution(OrchestratorExecution{
			ID:              "seed-create-save-error",
			Goal:            "seed",
			ApprovalScope:   "infrastructure_only",
			RequiredWorkers: []OrchestratorRequiredWorker{{HostID: "h1", AgentID: "zeroclaw", Count: 1}},
			TaskUnits:       []OrchestratorTaskUnit{{ID: "t1", Input: "hello"}},
		}); err != nil {
			t.Fatalf("seed execution failed: %v", err)
		}
		mustMakeReadOnly(t, storePath)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/orchestrator/executions", strings.NewReader(`{
			"goal":"g",
			"requiredWorkers":[{"hostId":"h1","agentId":"zeroclaw","count":1}],
			"taskUnits":[{"id":"t1","input":"hello"}],
			"approvalScope":"infrastructure_only"
		}`))
		rec := httptest.NewRecorder()
		handleOrchestratorExecutions(rec, req, "req-4", cfg)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected create save error status 500, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("empty-execution-id", func(t *testing.T) {
		t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))
		req := httptest.NewRequest(http.MethodGet, "/api/v1/orchestrator/executions/dummy", nil)
		req.URL.Path = "/api/v1/orchestrator/executions/ /authorize"
		rec := httptest.NewRecorder()
		handleOrchestratorExecutions(rec, req, "req-5", cfg)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected empty execution id status 400, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("load-execution-error", func(t *testing.T) {
		t.Setenv("CARRIER_REMOTE_CONTROL_STORE", t.TempDir())
		req := httptest.NewRequest(http.MethodGet, "/api/v1/orchestrator/executions/e1", nil)
		rec := httptest.NewRecorder()
		handleOrchestratorExecutions(rec, req, "req-6", cfg)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected load execution error status 500, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("authorize-decline-save-error", func(t *testing.T) {
		storePath := filepath.Join(t.TempDir(), "remote-control.json")
		t.Setenv("CARRIER_REMOTE_CONTROL_STORE", storePath)
		seed, err := upsertOrchestratorExecution(OrchestratorExecution{
			ID:              "exec-save-err-1",
			Goal:            "g",
			ApprovalScope:   "infrastructure_only",
			Status:          OrchestratorExecutionStatusPendingAuthorization,
			RequiredWorkers: []OrchestratorRequiredWorker{{HostID: "h1", AgentID: "zeroclaw", Count: 1}},
			TaskUnits:       []OrchestratorTaskUnit{{ID: "t1", Input: "hello"}},
		})
		if err != nil {
			t.Fatalf("seed execution failed: %v", err)
		}
		mustMakeReadOnly(t, storePath)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/orchestrator/executions/"+seed.ID+"/authorize", strings.NewReader(`{"approved":false}`))
		rec := httptest.NewRecorder()
		handleOrchestratorExecutions(rec, req, "req-7", cfg)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected decline save error status 500, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("authorize-approve-save-error-and-concurrency-defaults", func(t *testing.T) {
		storePath := filepath.Join(t.TempDir(), "remote-control.json")
		t.Setenv("CARRIER_REMOTE_CONTROL_STORE", storePath)
		seed, err := upsertOrchestratorExecution(OrchestratorExecution{
			ID:              "exec-save-err-2",
			Goal:            "g",
			ApprovalScope:   "infrastructure_only",
			Status:          OrchestratorExecutionStatusPendingAuthorization,
			MaxConcurrency:  -1,
			RequiredWorkers: []OrchestratorRequiredWorker{{HostID: "h1", AgentID: "zeroclaw", Count: 1}},
			TaskUnits:       []OrchestratorTaskUnit{{ID: "t1", Input: "hello"}},
		})
		if err != nil {
			t.Fatalf("seed execution failed: %v", err)
		}
		mustMakeReadOnly(t, storePath)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/orchestrator/executions/"+seed.ID+"/authorize", strings.NewReader(`{"approved":true,"maxConcurrency":999}`))
		rec := httptest.NewRecorder()
		handleOrchestratorExecutions(rec, req, "req-8", cfg)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected approve save error status 500, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("list-worker-leases-error", func(t *testing.T) {
		t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))
		seed, err := upsertOrchestratorExecution(OrchestratorExecution{
			ID:              "exec-worker-list-err",
			Goal:            "g",
			ApprovalScope:   "infrastructure_only",
			Status:          OrchestratorExecutionStatusPendingAuthorization,
			RequiredWorkers: []OrchestratorRequiredWorker{{HostID: "h1", AgentID: "zeroclaw", Count: 1}},
			TaskUnits:       []OrchestratorTaskUnit{{ID: "t1", Input: "hello"}},
		})
		if err != nil {
			t.Fatalf("seed execution failed: %v", err)
		}

		orig := orchestratorListLeasesByExecution
		orchestratorListLeasesByExecution = func(executionID string) ([]OrchestratorWorkerLease, error) {
			_ = executionID
			return nil, errors.New("lease load failed")
		}
		t.Cleanup(func() {
			orchestratorListLeasesByExecution = orig
		})

		req := httptest.NewRequest(http.MethodGet, "/api/v1/orchestrator/executions/"+seed.ID, nil)
		rec := httptest.NewRecorder()
		handleOrchestratorExecutions(rec, req, "req-9", cfg)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected worker lease load error status 500, got %d body=%s", rec.Code, rec.Body.String())
		}
	})
}

func TestHandleOrchestratorExecutionsApproveDefaultConcurrencyBranch(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))

	cfg := &GatewayConfig{
		RemoteControlPlaneEnabled: true,
	}
	seed, err := upsertOrchestratorExecution(OrchestratorExecution{
		ID:              "exec-default-concurrency",
		Goal:            "g",
		ApprovalScope:   "infrastructure_only",
		Status:          OrchestratorExecutionStatusPendingAuthorization,
		MaxConcurrency:  0,
		RequiredWorkers: []OrchestratorRequiredWorker{{HostID: "h1", AgentID: "zeroclaw", Count: 1}},
		TaskUnits:       []OrchestratorTaskUnit{{ID: "t1", Input: "hello"}},
	})
	if err != nil {
		t.Fatalf("seed execution failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/orchestrator/executions/"+seed.ID+"/authorize", strings.NewReader(`{"approved":true}`))
	rec := httptest.NewRecorder()
	handleOrchestratorExecutions(rec, req, "req-10", cfg)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected authorize accepted status, got %d body=%s", rec.Code, rec.Body.String())
	}
	payload := decodeJSONMap(t, rec)
	execMap, _ := payload["execution"].(map[string]interface{})
	if int(anyToFloat(execMap["maxConcurrency"])) != 8 {
		t.Fatalf("expected default maxConcurrency 8, got %+v", execMap)
	}
}

func TestRunOrchestratorExecutionAdditionalBranches(t *testing.T) {
	t.Run("missing-or-store-error", func(t *testing.T) {
		runOrchestratorExecution("missing-exec")
		t.Setenv("CARRIER_REMOTE_CONTROL_STORE", t.TempDir())
		runOrchestratorExecution("exec-x")
	})

	t.Run("not-approved-noop", func(t *testing.T) {
		t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))
		seed, err := upsertOrchestratorExecution(OrchestratorExecution{
			ID:              "exec-not-approved",
			Goal:            "g",
			ApprovalScope:   "infrastructure_only",
			Status:          OrchestratorExecutionStatusPendingAuthorization,
			Authorization:   OrchestratorAuthorization{InfrastructureApproved: false},
			RequiredWorkers: []OrchestratorRequiredWorker{{HostID: "h1", AgentID: "zeroclaw", Count: 1}},
			TaskUnits:       []OrchestratorTaskUnit{{ID: "t1", Input: "hello"}},
		})
		if err != nil {
			t.Fatalf("seed execution failed: %v", err)
		}
		runOrchestratorExecution(seed.ID)
		got, found, err := getOrchestratorExecution(seed.ID)
		if err != nil || !found {
			t.Fatalf("get execution failed found=%v err=%v", found, err)
		}
		if got.Status != OrchestratorExecutionStatusPendingAuthorization {
			t.Fatalf("expected status unchanged for not approved execution, got %q", got.Status)
		}
	})

	t.Run("save-running-state-error", func(t *testing.T) {
		storePath := filepath.Join(t.TempDir(), "remote-control.json")
		t.Setenv("CARRIER_REMOTE_CONTROL_STORE", storePath)
		seed, err := upsertOrchestratorExecution(OrchestratorExecution{
			ID:              "exec-save-running-err",
			Goal:            "g",
			ApprovalScope:   "infrastructure_only",
			Status:          OrchestratorExecutionStatusProvisioning,
			Authorization:   OrchestratorAuthorization{InfrastructureApproved: true},
			RequiredWorkers: []OrchestratorRequiredWorker{},
			TaskUnits:       []OrchestratorTaskUnit{{ID: "t1", Input: "hello"}},
		})
		if err != nil {
			t.Fatalf("seed execution failed: %v", err)
		}
		mustMakeReadOnly(t, storePath)
		runOrchestratorExecution(seed.ID)
	})

	t.Run("task-run-error-path", func(t *testing.T) {
		t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))
		configureSSHRunner(t, func(command string) remoteExecResult {
			switch {
			case strings.Contains(command, `if [ -f "$HOME/.zeroclaw/config.toml" ]`):
				return remoteExecResult{ExitCode: 0, Stdout: "1\n"}
			case strings.Contains(command, "zeroclaw task run"),
				strings.Contains(command, "zeroclaw run --message"),
				strings.Contains(command, "zeroclaw agent --message"),
				strings.Contains(command, "openclaw agent --local"):
				return remoteExecResult{ExitCode: 1, Stderr: "failed"}
			case strings.Contains(command, `rm -f "$HOME/.zeroclaw/config.toml"`):
				return remoteExecResult{ExitCode: 0}
			default:
				return remoteExecResult{ExitCode: 0}
			}
		})
		host, err := upsertRemoteHost(RemoteHost{
			Name:        "host-1",
			Host:        "127.0.0.1",
			Port:        22,
			User:        "ubuntu",
			AuthMode:    RemoteAuthModePrivateKey,
			KeyPath:     "~/.ssh/id_ed25519",
			RuntimeMode: RemoteRuntimeModeOnDemand,
		})
		if err != nil {
			t.Fatalf("upsertRemoteHost failed: %v", err)
		}
		seed, err := upsertOrchestratorExecution(OrchestratorExecution{
			ID:            "exec-run-err",
			Goal:          "g",
			ApprovalScope: "infrastructure_only",
			Status:        OrchestratorExecutionStatusProvisioning,
			Authorization: OrchestratorAuthorization{InfrastructureApproved: true},
			RequiredWorkers: []OrchestratorRequiredWorker{
				{HostID: host.ID, AgentID: "zeroclaw", Count: 1},
			},
			TaskUnits: []OrchestratorTaskUnit{{ID: "t1", Input: "hello", RetryBudget: 0}},
		})
		if err != nil {
			t.Fatalf("seed execution failed: %v", err)
		}
		runOrchestratorExecution(seed.ID)
		got, found, err := getOrchestratorExecution(seed.ID)
		if err != nil || !found {
			t.Fatalf("get execution failed found=%v err=%v", found, err)
		}
		if got.Status != OrchestratorExecutionStatusFailed {
			t.Fatalf("expected failed status after task run error, got %+v", got)
		}
		if len(got.Results) != 1 {
			t.Fatalf("expected failed task results to be persisted, got %+v", got.Results)
		}
	})
}

func TestAcquireWorkerForTaskMatchesAgentWithoutPinnedHost(t *testing.T) {
	picoclawPool := orchestratorWorkerPool{
		key: workerPoolKey(orchestratorLocalHostID, "picoclaw"),
		ch:  make(chan OrchestratorWorkerLease, 1),
	}
	zeroclawPool := orchestratorWorkerPool{
		key: workerPoolKey("host-z", "zeroclaw"),
		ch:  make(chan OrchestratorWorkerLease, 1),
	}
	picoclawPool.ch <- OrchestratorWorkerLease{ID: "lease-p", HostID: orchestratorLocalHostID, AgentID: "picoclaw"}
	zeroclawPool.ch <- OrchestratorWorkerLease{ID: "lease-z", HostID: "host-z", AgentID: "zeroclaw"}

	pools := map[string]orchestratorWorkerPool{
		picoclawPool.key: picoclawPool,
		zeroclawPool.key: zeroclawPool,
	}

	lease, key, err := acquireWorkerForTask(context.Background(), OrchestratorTaskUnit{
		ID:      "task-1",
		Input:   "collect logs",
		AgentID: "zeroclaw",
	}, pools, picoclawPool.key)
	if err != nil {
		t.Fatalf("acquireWorkerForTask returned error: %v", err)
	}
	if key != zeroclawPool.key {
		t.Fatalf("expected zeroclaw pool key, got %q", key)
	}
	if lease.ID != "lease-z" || lease.AgentID != "zeroclaw" {
		t.Fatalf("expected zeroclaw lease, got %+v", lease)
	}
}

func TestProvisionOrchestratorWorkersErrorBranches(t *testing.T) {
	t.Run("get-host-error", func(t *testing.T) {
		t.Setenv("CARRIER_REMOTE_CONTROL_STORE", t.TempDir())
		_, err := provisionOrchestratorWorkers(context.Background(), OrchestratorExecution{
			ID: "exec-1",
			RequiredWorkers: []OrchestratorRequiredWorker{
				{HostID: "h1", AgentID: "zeroclaw", Count: 1},
			},
		})
		if err == nil {
			t.Fatal("expected get host error")
		}
	})

	t.Run("first-lease-save-error", func(t *testing.T) {
		storePath := filepath.Join(t.TempDir(), "remote-control.json")
		t.Setenv("CARRIER_REMOTE_CONTROL_STORE", storePath)
		host, err := upsertRemoteHost(RemoteHost{
			Name:        "host-1",
			Host:        "127.0.0.1",
			Port:        22,
			User:        "ubuntu",
			AuthMode:    RemoteAuthModePrivateKey,
			KeyPath:     "~/.ssh/id_ed25519",
			RuntimeMode: RemoteRuntimeModeOnDemand,
		})
		if err != nil {
			t.Fatalf("upsertRemoteHost failed: %v", err)
		}
		mustMakeReadOnly(t, storePath)
		_, err = provisionOrchestratorWorkers(context.Background(), OrchestratorExecution{
			ID: "exec-1",
			RequiredWorkers: []OrchestratorRequiredWorker{
				{HostID: host.ID, AgentID: "zeroclaw", Count: 1},
			},
		})
		if err == nil {
			t.Fatal("expected first lease save error")
		}
	})

	t.Run("config-check-error", func(t *testing.T) {
		t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))
		configureSSHRunner(t, func(command string) remoteExecResult {
			if strings.Contains(command, `if [ -f "$HOME/.zeroclaw/config.toml" ]`) {
				return remoteExecResult{ExitCode: 1, Stderr: "check failed"}
			}
			return remoteExecResult{ExitCode: 0}
		})
		host, err := upsertRemoteHost(RemoteHost{
			Name:        "host-1",
			Host:        "127.0.0.1",
			Port:        22,
			User:        "ubuntu",
			AuthMode:    RemoteAuthModePrivateKey,
			KeyPath:     "~/.ssh/id_ed25519",
			RuntimeMode: RemoteRuntimeModeOnDemand,
		})
		if err != nil {
			t.Fatalf("upsertRemoteHost failed: %v", err)
		}
		_, err = provisionOrchestratorWorkers(context.Background(), OrchestratorExecution{
			ID: "exec-1",
			RequiredWorkers: []OrchestratorRequiredWorker{
				{HostID: host.ID, AgentID: "zeroclaw", Count: 1},
			},
		})
		if err == nil {
			t.Fatal("expected config-check error")
		}
	})

	t.Run("install-error", func(t *testing.T) {
		t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))
		configureSSHRunner(t, func(command string) remoteExecResult {
			switch {
			case strings.Contains(command, `if [ -f "$HOME/.zeroclaw/config.toml" ]`):
				return remoteExecResult{ExitCode: 0, Stdout: "0\n"}
			case strings.Contains(command, "zeroclaw-") && strings.Contains(command, "curl -fsSL"):
				return remoteExecResult{ExitCode: 1, Stderr: "install failed"}
			default:
				return remoteExecResult{ExitCode: 0}
			}
		})
		host, err := upsertRemoteHost(RemoteHost{
			Name:        "host-1",
			Host:        "127.0.0.1",
			Port:        22,
			User:        "ubuntu",
			AuthMode:    RemoteAuthModePrivateKey,
			KeyPath:     "~/.ssh/id_ed25519",
			RuntimeMode: RemoteRuntimeModeOnDemand,
		})
		if err != nil {
			t.Fatalf("upsertRemoteHost failed: %v", err)
		}
		_, err = provisionOrchestratorWorkers(context.Background(), OrchestratorExecution{
			ID: "exec-1",
			RequiredWorkers: []OrchestratorRequiredWorker{
				{HostID: host.ID, AgentID: "zeroclaw", Count: 1},
			},
		})
		if err == nil {
			t.Fatal("expected install error")
		}
	})

	t.Run("second-lease-save-error-after-config-check", func(t *testing.T) {
		storePath := filepath.Join(t.TempDir(), "remote-control.json")
		t.Setenv("CARRIER_REMOTE_CONTROL_STORE", storePath)
		host, err := upsertRemoteHost(RemoteHost{
			Name:        "host-1",
			Host:        "127.0.0.1",
			Port:        22,
			User:        "ubuntu",
			AuthMode:    RemoteAuthModePrivateKey,
			KeyPath:     "~/.ssh/id_ed25519",
			RuntimeMode: RemoteRuntimeModeOnDemand,
		})
		if err != nil {
			t.Fatalf("upsertRemoteHost failed: %v", err)
		}

		chmodDone := false
		configureSSHRunner(t, func(command string) remoteExecResult {
			if strings.Contains(command, `if [ -f "$HOME/.zeroclaw/config.toml" ]`) {
				if !chmodDone {
					chmodDone = true
					mustMakeReadOnly(t, storePath)
				}
				return remoteExecResult{ExitCode: 0, Stdout: "1\n"}
			}
			return remoteExecResult{ExitCode: 0}
		})

		_, err = provisionOrchestratorWorkers(context.Background(), OrchestratorExecution{
			ID: "exec-1",
			RequiredWorkers: []OrchestratorRequiredWorker{
				{HostID: host.ID, AgentID: "zeroclaw", Count: 1},
			},
		})
		if err == nil {
			t.Fatal("expected second lease save error")
		}
	})

	t.Run("install-result-incomplete", func(t *testing.T) {
		t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))
		configureSSHRunner(t, func(command string) remoteExecResult {
			if strings.Contains(command, `if [ -f "$HOME/.zeroclaw/config.toml" ]`) {
				return remoteExecResult{ExitCode: 0, Stdout: "0\n"}
			}
			return remoteExecResult{ExitCode: 0}
		})
		host, err := upsertRemoteHost(RemoteHost{
			Name:        "host-1",
			Host:        "127.0.0.1",
			Port:        22,
			User:        "ubuntu",
			AuthMode:    RemoteAuthModePrivateKey,
			KeyPath:     "~/.ssh/id_ed25519",
			RuntimeMode: RemoteRuntimeModeOnDemand,
		})
		if err != nil {
			t.Fatalf("upsertRemoteHost failed: %v", err)
		}

		orig := orchestratorInstallAgent
		orchestratorInstallAgent = func(ctx context.Context, host RemoteHost, hostID, agentID string, isolation bool) (*remoteInstallResult, error) {
			_ = ctx
			_ = host
			_ = hostID
			_ = agentID
			_ = isolation
			return &remoteInstallResult{Installed: false}, nil
		}
		t.Cleanup(func() {
			orchestratorInstallAgent = orig
		})

		_, err = provisionOrchestratorWorkers(context.Background(), OrchestratorExecution{
			ID: "exec-1",
			RequiredWorkers: []OrchestratorRequiredWorker{
				{HostID: host.ID, AgentID: "zeroclaw", Count: 1},
			},
		})
		if err == nil || !strings.Contains(err.Error(), "worker install did not complete") {
			t.Fatalf("expected install-result-incomplete error, got %v", err)
		}
	})
}

func TestRunTasksAndRetriesAdditionalBranches(t *testing.T) {
	t.Run("run-tasks-default-concurrency-and-error-return", func(t *testing.T) {
		t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))
		configureSSHRunner(t, func(command string) remoteExecResult {
			switch {
			case strings.Contains(command, "zeroclaw task run"),
				strings.Contains(command, "zeroclaw run --message"),
				strings.Contains(command, "zeroclaw agent --message"),
				strings.Contains(command, "openclaw agent --local"):
				return remoteExecResult{ExitCode: 1, Stderr: "run failed"}
			default:
				return remoteExecResult{ExitCode: 0}
			}
		})
		host, err := upsertRemoteHost(RemoteHost{
			Name:        "host-1",
			Host:        "127.0.0.1",
			Port:        22,
			User:        "ubuntu",
			AuthMode:    RemoteAuthModePrivateKey,
			KeyPath:     "~/.ssh/id_ed25519",
			RuntimeMode: RemoteRuntimeModeOnDemand,
		})
		if err != nil {
			t.Fatalf("upsertRemoteHost failed: %v", err)
		}
		lease, err := upsertOrchestratorWorkerLease(OrchestratorWorkerLease{
			ID:          "lease-1",
			ExecutionID: "exec-1",
			HostID:      host.ID,
			AgentID:     "zeroclaw",
			State:       OrchestratorWorkerStateReady,
		})
		if err != nil {
			t.Fatalf("upsert lease failed: %v", err)
		}
		results, runErr := runOrchestratorTasks(context.Background(), OrchestratorExecution{
			ID:             "exec-1",
			MaxConcurrency: 0,
			TaskUnits: []OrchestratorTaskUnit{
				{ID: "t1", Input: "hello", RetryBudget: 0},
			},
		}, []OrchestratorWorkerLease{lease})
		if runErr == nil || len(results) != 1 {
			t.Fatalf("expected run error with one result, err=%v results=%+v", runErr, results)
		}
	})

	t.Run("run-tasks-empty-task-list-branch", func(t *testing.T) {
		results, err := runOrchestratorTasks(context.Background(), OrchestratorExecution{
			ID:             "exec-1",
			MaxConcurrency: 0,
			TaskUnits:      []OrchestratorTaskUnit{},
		}, []OrchestratorWorkerLease{{
			ID:      "lease-1",
			HostID:  "host-1",
			AgentID: "zeroclaw",
			State:   OrchestratorWorkerStateReady,
		}})
		if err != nil {
			t.Fatalf("expected nil error for empty task list, got %v", err)
		}
		if len(results) != 0 {
			t.Fatalf("expected empty results, got %+v", results)
		}
	})

	t.Run("run-task-with-retries-acquire-error-and-retry-success", func(t *testing.T) {
		t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))
		noPoolResult, noPoolErr := runTaskWithRetries(context.Background(), OrchestratorExecution{ID: "exec-1"}, OrchestratorTaskUnit{
			ID:          "task-1",
			RetryBudget: 0,
		}, map[string]orchestratorWorkerPool{}, "")
		if noPoolErr == nil || !strings.Contains(noPoolErr.Error(), "no worker pool available") {
			t.Fatalf("expected acquire error branch, err=%v result=%+v", noPoolErr, noPoolResult)
		}

		host, err := upsertRemoteHost(RemoteHost{
			Name:        "host-1",
			Host:        "127.0.0.1",
			Port:        22,
			User:        "ubuntu",
			AuthMode:    RemoteAuthModePrivateKey,
			KeyPath:     "~/.ssh/id_ed25519",
			RuntimeMode: RemoteRuntimeModeOnDemand,
		})
		if err != nil {
			t.Fatalf("upsertRemoteHost failed: %v", err)
		}
		lease, err := upsertOrchestratorWorkerLease(OrchestratorWorkerLease{
			ID:          "lease-retry",
			ExecutionID: "exec-1",
			HostID:      host.ID,
			AgentID:     "zeroclaw",
			State:       OrchestratorWorkerStateReady,
		})
		if err != nil {
			t.Fatalf("upsert lease failed: %v", err)
		}
		key := workerPoolKey(host.ID, "zeroclaw")
		pool := orchestratorWorkerPool{key: key, ch: make(chan OrchestratorWorkerLease, 1)}
		pool.ch <- lease
		callCount := 0
		configureSSHRunner(t, func(command string) remoteExecResult {
			switch {
			case strings.Contains(command, "zeroclaw task run"):
				callCount++
				if callCount == 1 {
					return remoteExecResult{ExitCode: 1, Stderr: "first fail"}
				}
				return remoteExecResult{ExitCode: 0, Stdout: `{"output_text":"ok"}`}
			case strings.Contains(command, "zeroclaw run --message"),
				strings.Contains(command, "zeroclaw agent --message"),
				strings.Contains(command, "openclaw agent --local"):
				return remoteExecResult{ExitCode: 1, Stderr: "retry path fail"}
			default:
				return remoteExecResult{ExitCode: 0}
			}
		})
		result, err := runTaskWithRetries(context.Background(), OrchestratorExecution{ID: "exec-1"}, OrchestratorTaskUnit{
			ID:          "task-1",
			Input:       "hello",
			HostID:      host.ID,
			AgentID:     "zeroclaw",
			RetryBudget: 1,
		}, map[string]orchestratorWorkerPool{key: pool}, key)
		if err != nil {
			t.Fatalf("expected retry success, got error: %v", err)
		}
		if result.Attempts != 2 || strings.TrimSpace(result.Output) != "ok" {
			t.Fatalf("expected second-attempt success result, got %+v", result)
		}
	})

	t.Run("run-task-with-retries-final-failure", func(t *testing.T) {
		t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))
		host, err := upsertRemoteHost(RemoteHost{
			Name:        "host-1",
			Host:        "127.0.0.1",
			Port:        22,
			User:        "ubuntu",
			AuthMode:    RemoteAuthModePrivateKey,
			KeyPath:     "~/.ssh/id_ed25519",
			RuntimeMode: RemoteRuntimeModeOnDemand,
		})
		if err != nil {
			t.Fatalf("upsertRemoteHost failed: %v", err)
		}
		lease, err := upsertOrchestratorWorkerLease(OrchestratorWorkerLease{
			ID:          "lease-fail",
			ExecutionID: "exec-1",
			HostID:      host.ID,
			AgentID:     "zeroclaw",
			State:       OrchestratorWorkerStateReady,
		})
		if err != nil {
			t.Fatalf("upsert lease failed: %v", err)
		}
		key := workerPoolKey(host.ID, "zeroclaw")
		pool := orchestratorWorkerPool{key: key, ch: make(chan OrchestratorWorkerLease, 1)}
		pool.ch <- lease
		configureSSHRunner(t, func(command string) remoteExecResult {
			if strings.Contains(command, "zeroclaw task run") ||
				strings.Contains(command, "zeroclaw run --message") ||
				strings.Contains(command, "zeroclaw agent --message") ||
				strings.Contains(command, "openclaw agent --local") {
				return remoteExecResult{ExitCode: 1, Stderr: "always fail"}
			}
			return remoteExecResult{ExitCode: 0}
		})
		_, runErr := runTaskWithRetries(context.Background(), OrchestratorExecution{ID: "exec-1"}, OrchestratorTaskUnit{
			ID:          "task-1",
			Input:       "hello",
			HostID:      host.ID,
			AgentID:     "zeroclaw",
			RetryBudget: 0,
		}, map[string]orchestratorWorkerPool{key: pool}, key)
		if runErr == nil {
			t.Fatal("expected final failure when retries exhausted")
		}
	})
}

func TestAcquireAndReclaimAdditionalBranches(t *testing.T) {
	t.Run("acquire-default-agent-id", func(t *testing.T) {
		poolKey := workerPoolKey("host-1", "zeroclaw")
		pool := orchestratorWorkerPool{
			key: poolKey,
			ch:  make(chan OrchestratorWorkerLease, 1),
		}
		pool.ch <- OrchestratorWorkerLease{ID: "lease-1"}
		lease, key, err := acquireWorkerForTask(context.Background(), OrchestratorTaskUnit{
			HostID: "host-1",
		}, map[string]orchestratorWorkerPool{poolKey: pool}, poolKey)
		if err != nil {
			t.Fatalf("acquireWorkerForTask default-agent failed: %v", err)
		}
		if key != poolKey || lease.ID != "lease-1" {
			t.Fatalf("unexpected acquired lease=%+v key=%q", lease, key)
		}
	})

	t.Run("reclaim-execution-leases-list-error", func(t *testing.T) {
		t.Setenv("CARRIER_REMOTE_CONTROL_STORE", t.TempDir())
		if _, err := reclaimExecutionLeases(context.Background(), "exec-1", true); err == nil {
			t.Fatal("expected list error from reclaimExecutionLeases")
		}
	})

	t.Run("reclaim-lease-set-host-load-error", func(t *testing.T) {
		t.Setenv("CARRIER_REMOTE_CONTROL_STORE", t.TempDir())
		summary, err := reclaimOrchestratorLeaseSet(context.Background(), []OrchestratorWorkerLease{{
			ID:          "lease-1",
			ExecutionID: "exec-1",
			HostID:      "host-1",
			AgentID:     "zeroclaw",
			State:       OrchestratorWorkerStateReady,
			Ephemeral:   true,
		}}, true, 0)
		failedCount, _ := summary["failed"].(int)
		if err == nil || failedCount < 1 {
			t.Fatalf("expected host load error branch, err=%v summary=%+v", err, summary)
		}
		failures, _ := summary["failures"].([]string)
		hasHostLoadFailure := false
		for _, failure := range failures {
			if strings.Contains(failure, "lease-1: read remote control store") {
				hasHostLoadFailure = true
				break
			}
		}
		if !hasHostLoadFailure {
			t.Fatalf("expected host load failure detail, summary=%+v", summary)
		}
	})
}

func TestRemoteRunAgentRoutingAndParsingBranches(t *testing.T) {
	t.Run("remoteRunTaskViaAgent-default-openclaw", func(t *testing.T) {
		host := RemoteHost{
			Host:     "127.0.0.1",
			Port:     22,
			User:     "ubuntu",
			AuthMode: RemoteAuthModePrivateKey,
			KeyPath:  "~/.ssh/id_ed25519",
		}
		configureSSHRunner(t, func(command string) remoteExecResult {
			if strings.Contains(command, "openclaw agent --local") {
				return remoteExecResult{ExitCode: 0, Stdout: `{"sessionId":"s1","output_text":"openclaw-ok"}`}
			}
			return remoteExecResult{ExitCode: 0}
		})
		result, _, err := remoteRunTaskViaAgent(context.Background(), host, "host-1", "openclaw", "hello", "sess-1")
		if err != nil {
			t.Fatalf("expected openclaw routing success, got %v", err)
		}
		if strings.TrimSpace(result.Output) != "openclaw-ok" {
			t.Fatalf("unexpected routing result %+v", result)
		}
	})

	t.Run("remoteRunViaZeroClaw-runRemoteCommand-error", func(t *testing.T) {
		host := RemoteHost{
			Host:     "127.0.0.1",
			Port:     22,
			User:     "ubuntu",
			AuthMode: RemoteAuthModePrivateKey,
			KeyPath:  "~/.ssh/id_ed25519",
		}
		orig := sshExecRunner
		sshExecRunner = func(_ context.Context, args []string) (remoteExecResult, error) {
			_ = args
			return remoteExecResult{ExitCode: 1, Stderr: "transport error"}, errors.New("transport error")
		}
		t.Cleanup(func() {
			sshExecRunner = orig
		})
		if _, _, err := remoteRunViaZeroClaw(context.Background(), host, "host-1", "zeroclaw", "hello", "sess-1"); err == nil {
			t.Fatal("expected runRemoteCommand error branch in remoteRunViaZeroClaw")
		}
	})

	t.Run("parseRemoteRunPayload-unmarshal-error", func(t *testing.T) {
		contract := buildRemoteMemoryRuntimeContract("zeroclaw")
		payload := parseRemoteRunPayload(`[1,2,3]`, contract)
		if strings.TrimSpace(anyToString(payload["message"])) == "" {
			t.Fatalf("expected message fallback on unmarshal error, got %+v", payload)
		}
		if _, ok := payload["memory"].(map[string]interface{}); !ok {
			t.Fatalf("expected memory block on unmarshal error, got %+v", payload)
		}
	})
}

func TestRunOrchestratorTaskAttemptGetHostErrorBranch(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", t.TempDir())
	_, err := runOrchestratorTaskAttempt(context.Background(), OrchestratorExecution{ID: "exec-1"}, OrchestratorTaskUnit{
		ID:    "task-1",
		Input: "hello",
	}, OrchestratorWorkerLease{
		ID:      "lease-1",
		HostID:  "host-1",
		AgentID: "zeroclaw",
	}, 1)
	if err == nil {
		t.Fatal("expected getRemoteHost error branch")
	}
}

func mustMakeReadOnly(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat path %q failed: %v", path, err)
	}
	if info.IsDir() {
		if err := os.Chmod(path, 0o500); err != nil {
			t.Fatalf("chmod dir read-only failed: %v", err)
		}
		return
	}
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatalf("chmod file read-only failed: %v", err)
	}
}
