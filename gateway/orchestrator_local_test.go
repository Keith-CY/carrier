package gateway

import (
	"encoding/json"
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
	var createMemoryCalls int
	var chatProvider string
	var chatInstanceID string
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
		"POST /api/v2/memory/entries/create": func(w http.ResponseWriter, r *http.Request) {
			createMemoryCalls++
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode create memory body: %v", err)
			}
			_, _ = w.Write([]byte(`{"entry":{"id":"` + strings.TrimSpace(anyToString(body["id"])) + `","type":"per_agent","owner":"` + strings.TrimSpace(anyToString(body["owner"])) + `","state":"created"}}`))
		},
		"POST /api/v1/agents/zeroclaw/chat": func(w http.ResponseWriter, r *http.Request) {
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode chat body: %v", err)
			}
			chatProvider = strings.TrimSpace(anyToString(body["provider"]))
			chatInstanceID = strings.TrimSpace(anyToString(body["instanceId"]))
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
		"requestedProvider":"openrouter",
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
	if createMemoryCalls != 1 {
		t.Fatalf("expected delegated child per-agent memory provisioning, got createMemoryCalls=%d", createMemoryCalls)
	}
	if chatInstanceID == "" {
		t.Fatal("expected local worker chat to include delegated child instanceId")
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
	if chatProvider != "openrouter" {
		t.Fatalf("expected local worker chat provider openrouter, got %q", chatProvider)
	}
}

func TestLocalOrchestratorRunProvisionsDelegatedChild(t *testing.T) {
	t.Setenv("CARRIER_INSTANCE_STORE", t.TempDir()+"/instances.json")
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", t.TempDir()+"/remote-control.json")
	resetRemoteMetricsForTests()

	parent := managedAgentInstance{
		ID:                  "zeroclaw-main",
		AgentID:             "zeroclaw",
		Type:                "zeroclaw",
		AgentLifecycleMode:  managedAgentLifecyclePersistent,
		MemoryBindingMode:   managedMemoryBindingLiveMount,
		MemoryRefreshPolicy: managedMemoryRefreshNextTurn,
		SharedScopes:        []string{"shared:team"},
		CreatedAt:           nowTimestamp(),
		UpdatedAt:           nowTimestamp(),
	}
	if err := upsertManagedInstance(parent); err != nil {
		t.Fatalf("upsertManagedInstance(parent): %v", err)
	}

	order := make([]string, 0, 4)
	var createdOwner string
	var snapshotSourceSubject string
	var snapshotTargetInstanceID string
	var mountedInstanceID string
	var chatInstanceID string
	var chatSessionID string

	daemonSrv := newMockDaemon(map[string]http.HandlerFunc{
		"GET /api/v1/agents": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"agents":[{"id":"zeroclaw","installState":"installed","runtimeState":"running"}]}`))
		},
		"POST /api/v2/memory/entries/create": func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "create")
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode create memory body: %v", err)
			}
			createdOwner = strings.TrimSpace(anyToString(body["owner"]))
			_, _ = w.Write([]byte(`{"entry":{"id":"` + strings.TrimSpace(anyToString(body["id"])) + `","type":"per_agent","owner":"` + createdOwner + `","state":"created"}}`))
		},
		"POST /api/v2/memory/instance/snapshot": func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "snapshot")
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode snapshot body: %v", err)
			}
			snapshotSourceSubject = strings.TrimSpace(anyToString(body["sourceSubject"]))
			snapshotTargetInstanceID = strings.TrimSpace(anyToString(body["targetInstanceId"]))
			_, _ = w.Write([]byte(`{"snapshot":{"id":"snap-local-1","digest":"sha256:local-1","scope":"shared:snapshot-snap-local-1"}}`))
		},
		"POST /api/v2/memory/instance/snapshot/mount": func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "mount")
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode mount body: %v", err)
			}
			mountedInstanceID = strings.TrimSpace(anyToString(body["instanceId"]))
			_, _ = w.Write([]byte(`{"status":"mounted"}`))
		},
		"POST /api/v1/agents/zeroclaw/chat": func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "chat")
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode chat body: %v", err)
			}
			chatInstanceID = strings.TrimSpace(anyToString(body["instanceId"]))
			chatSessionID = strings.TrimSpace(anyToString(body["sessionId"]))
			_, _ = w.Write([]byte(`{"agentId":"zeroclaw","message":"delegated-output"}`))
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
		"goal":"run local delegated task",
		"requestedProvider":"openrouter",
		"requiredMemory":["public","shared:team"],
		"requiredWorkers":[{"hostId":"local","agentId":"zeroclaw","count":1}],
		"taskUnits":[{"id":"t1","input":"hello delegated","hostId":"local","agentId":"zeroclaw"}],
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
	for i := 0; i < 80; i++ {
		rec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/orchestrator/executions/"+execID, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("get execution status=%d body=%s", rec.Code, rec.Body.String())
		}
		payload := decodeJSONMap(t, rec)
		execution, _ := payload["execution"].(map[string]interface{})
		status = strings.TrimSpace(anyToString(execution["status"]))
		if status == string(OrchestratorExecutionStatusCompleted) || status == string(OrchestratorExecutionStatusFailed) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if status != string(OrchestratorExecutionStatusCompleted) {
		t.Fatalf("expected completed status, got %q", status)
	}

	if strings.Join(order, ",") != "create,snapshot,mount,chat" {
		t.Fatalf("provisioning order = %v, want [create snapshot mount chat]", order)
	}
	if snapshotSourceSubject != "zeroclaw" {
		t.Fatalf("snapshot sourceSubject = %q, want zeroclaw", snapshotSourceSubject)
	}
	if createdOwner == "" || createdOwner != snapshotTargetInstanceID {
		t.Fatalf("expected created per-agent memory owner to match child instance, owner=%q targetInstanceId=%q", createdOwner, snapshotTargetInstanceID)
	}
	if mountedInstanceID != snapshotTargetInstanceID {
		t.Fatalf("mounted instanceId = %q, want %q", mountedInstanceID, snapshotTargetInstanceID)
	}
	if chatInstanceID != snapshotTargetInstanceID {
		t.Fatalf("chat instanceId = %q, want %q", chatInstanceID, snapshotTargetInstanceID)
	}
	if chatSessionID == "" || !strings.Contains(chatSessionID, chatInstanceID) {
		t.Fatalf("chat sessionId = %q, want child-specific session containing %q", chatSessionID, chatInstanceID)
	}

	instances, _, err := loadManagedInstances()
	if err != nil {
		t.Fatalf("loadManagedInstances: %v", err)
	}
	childIdx := findManagedInstanceIndex(instances, chatInstanceID)
	if childIdx < 0 {
		t.Fatalf("expected child instance %q in managed instances: %+v", chatInstanceID, instances)
	}
	child := instances[childIdx]
	if child.ParentExecutionID != execID {
		t.Fatalf("ParentExecutionID = %q, want %q", child.ParentExecutionID, execID)
	}
	if child.SnapshotID != "snap-local-1" {
		t.Fatalf("SnapshotID = %q, want snap-local-1", child.SnapshotID)
	}
	if child.SnapshotDigest != "sha256:local-1" {
		t.Fatalf("SnapshotDigest = %q, want sha256:local-1", child.SnapshotDigest)
	}
	if strings.TrimSpace(child.PerAgentMemoryID) == "" {
		t.Fatalf("expected child per-agent memory id, child=%+v", child)
	}

	execution, found, err := getOrchestratorExecution(execID)
	if err != nil {
		t.Fatalf("getOrchestratorExecution: %v", err)
	}
	if !found {
		t.Fatalf("execution %q not found after completion", execID)
	}
	if execution.ChildAgentID != chatInstanceID {
		t.Fatalf("ChildAgentID = %q, want %q", execution.ChildAgentID, chatInstanceID)
	}
	if execution.ChildPerAgentMemoryID != child.PerAgentMemoryID {
		t.Fatalf("ChildPerAgentMemoryID = %q, want %q", execution.ChildPerAgentMemoryID, child.PerAgentMemoryID)
	}
	if execution.SnapshotID != "snap-local-1" {
		t.Fatalf("SnapshotID = %q, want snap-local-1", execution.SnapshotID)
	}
	if execution.SnapshotDigest != "sha256:local-1" {
		t.Fatalf("SnapshotDigest = %q, want sha256:local-1", execution.SnapshotDigest)
	}
}
