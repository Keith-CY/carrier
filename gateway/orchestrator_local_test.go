package gateway

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestOrchestratorExecutionLocalLifecycle(t *testing.T) {
	t.Setenv("CARRIER_INSTANCE_STORE", t.TempDir()+"/instances.json")
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", t.TempDir()+"/remote-control.json")
	resetRemoteMetricsForTests()

	var startCalls int
	daemonSrv := newMockDaemon(map[string]http.HandlerFunc{
		"GET /api/v1/agents": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"agents":[{"id":"zeroclaw","installState":"installed","runtimeState":"stopped"}]}`))
		},
		"POST /api/v1/agents/zeroclaw/start": func(w http.ResponseWriter, r *http.Request) {
			startCalls++
			_, _ = w.Write([]byte(`{"started":true}`))
		},
		"GET /api/v1/agents/zeroclaw/status": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"statuses":[{"id":"zeroclaw","installState":"installed","runtimeState":"running"}]}`))
		},
		"POST /api/v1/agents/zeroclaw/chat": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"agentId":"zeroclaw","message":"local-worker-output"}`))
		},
	})
	defer daemonSrv.Close()

	daemon := NewDaemonClient(daemonSrv.URL, "test-token", 5*time.Second)
	sessions := NewSessionStore("", 0, nil)
	t.Cleanup(sessions.Stop)
	downloads := NewDownloadStore("", nil)
	rl := NewGatewayRateLimiter(100, 1000, time.Minute, nil)
	onboard := NewOnboardStore()
	setup := NewSetupStore()
	cfg := &GatewayConfig{
		APIToken:                  "test-gateway-token",
		MaxCommandBodyBytes:       64 * 1024,
		RemoteControlPlaneEnabled: true,
		RemoteChatEnabled:         true,
		ProviderBindingEnabled:    true,
	}
	mux := buildGatewayMux(cfg, daemon, sessions, downloads, rl, onboard, setup)

	origFactory := orchestratorLocalDaemonClientFn
	orchestratorLocalDaemonClientFn = func() *DaemonClient { return daemon }
	t.Cleanup(func() {
		orchestratorLocalDaemonClientFn = origFactory
	})

	createRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions", `{
		"goal":"run local zeroclaw task",
		"requiredWorkers":[{"hostId":"local","agentId":"zeroclaw","count":1}],
		"taskUnits":[{"id":"t1","input":"hello local","hostId":"local","agentId":"zeroclaw"}],
		"approvalScope":"infrastructure_only"
	}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create execution status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	createPayload := decodeJSONMap(t, createRec)
	execMap, _ := createPayload["execution"].(map[string]interface{})
	execID := strings.TrimSpace(anyToString(execMap["id"]))
	if execID == "" {
		t.Fatalf("missing execution id payload=%+v", createPayload)
	}

	authRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions/"+execID+"/authorize", `{"approved":true,"actor":"tester"}`)
	if authRec.Code != http.StatusAccepted {
		t.Fatalf("authorize status=%d body=%s", authRec.Code, authRec.Body.String())
	}

	var status string
	var finalPayload map[string]interface{}
	for i := 0; i < 80; i++ {
		rec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/orchestrator/executions/"+execID, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("get execution status=%d body=%s", rec.Code, rec.Body.String())
		}
		payload := decodeJSONMap(t, rec)
		execution, _ := payload["execution"].(map[string]interface{})
		status = strings.TrimSpace(anyToString(execution["status"]))
		if status == string(OrchestratorExecutionStatusCompleted) || status == string(OrchestratorExecutionStatusFailed) {
			finalPayload = payload
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if status != string(OrchestratorExecutionStatusCompleted) {
		t.Fatalf("expected completed status, got %q payload=%+v", status, finalPayload)
	}

	execution, _ := finalPayload["execution"].(map[string]interface{})
	results, _ := execution["results"].([]interface{})
	if len(results) != 1 {
		t.Fatalf("expected one task result, got %d payload=%+v", len(results), finalPayload)
	}
	resultMap, _ := results[0].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(resultMap["status"])); got != string(OrchestratorTaskStatusCompleted) {
		t.Fatalf("expected task completed, got %q result=%+v", got, resultMap)
	}
	if got := strings.TrimSpace(anyToString(resultMap["output"])); got != "local-worker-output" {
		t.Fatalf("expected local output, got %q result=%+v", got, resultMap)
	}

	if startCalls == 0 {
		t.Fatal("expected local agent start to be called when runtimeState is stopped")
	}

	workers, _ := finalPayload["workers"].([]interface{})
	if len(workers) != 1 {
		t.Fatalf("expected one worker lease, got %d payload=%+v", len(workers), finalPayload)
	}
	worker, _ := workers[0].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(worker["hostId"])); got != orchestratorLocalHostID {
		t.Fatalf("expected worker hostId=local, got %q worker=%+v", got, worker)
	}
	if got := strings.TrimSpace(anyToString(worker["state"])); got != string(OrchestratorWorkerStateReclaimed) {
		t.Fatalf("expected worker reclaimed, got %q worker=%+v", got, worker)
	}
}
