package gateway

import (
	"context"
	"encoding/json"
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

func TestEnsureOrchestratorLocalAgentReadyUsesIsolationStartOptions(t *testing.T) {
	var startIsolation any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/agents":
			_, _ = w.Write([]byte(`{"agents":[{"id":"zeroclaw","installState":"installed","runtimeState":"stopped","isolated":true}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/agents/zeroclaw/start":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
				startIsolation = payload["isolation"]
			}
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/agents/zeroclaw/status":
			_, _ = w.Write([]byte(`{"statuses":[{"id":"zeroclaw","runtimeState":"running"}]}`))
		default:
			t.Fatalf("unexpected daemon request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewDaemonClient(server.URL, "", time.Second)
	if err := ensureOrchestratorLocalAgentReady(context.Background(), client, "exec-1", "zeroclaw"); err != nil {
		t.Fatalf("ensureOrchestratorLocalAgentReady returned error: %v", err)
	}
	if startIsolation != true {
		t.Fatalf("expected isolated local worker start to forward isolation=true, got %#v", startIsolation)
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

func TestHandleOrchestratorPlansAcceptsHostLabels(t *testing.T) {
	mux := buildRemoteFeatureMuxWithDaemonHandlers(t, map[string]http.HandlerFunc{
		"POST /api/v1/base-agent/decompose": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"tasks":[{"id":"task-1","input":"collect gpu diagnostics","agentId":"zeroclaw"},{"id":"task-2","input":"summarize gpu diagnostics","agentId":"picoclaw"}]}`))
		},
	})

	rec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/plans", `{
		"goal":"triage gpu incident",
		"hostLabels":["gpu","prod","gpu"],
		"maxConcurrency":9
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected planning status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	payload := decodeJSONMap(t, rec)
	plan, _ := payload["plan"].(map[string]interface{})
	hostLabels, _ := plan["hostLabels"].([]interface{})
	if len(hostLabels) != 2 || anyToString(hostLabels[0]) != "gpu" || anyToString(hostLabels[1]) != "prod" {
		t.Fatalf("hostLabels = %+v, want [gpu prod]", hostLabels)
	}
	requiredWorkers, _ := plan["requiredWorkers"].([]interface{})
	if len(requiredWorkers) != 2 {
		t.Fatalf("requiredWorkers = %d, want 2", len(requiredWorkers))
	}
	firstWorker, _ := requiredWorkers[0].(map[string]interface{})
	workerLabels, _ := firstWorker["hostLabels"].([]interface{})
	if len(workerLabels) != 2 || anyToString(workerLabels[0]) != "gpu" || anyToString(workerLabels[1]) != "prod" {
		t.Fatalf("requiredWorkers[0].hostLabels = %+v, want [gpu prod]", workerLabels)
	}
	taskUnits, _ := plan["taskUnits"].([]interface{})
	if len(taskUnits) != 2 {
		t.Fatalf("taskUnits = %d, want 2", len(taskUnits))
	}
	firstTask, _ := taskUnits[0].(map[string]interface{})
	taskLabels, _ := firstTask["hostLabels"].([]interface{})
	if len(taskLabels) != 2 || anyToString(taskLabels[0]) != "gpu" || anyToString(taskLabels[1]) != "prod" {
		t.Fatalf("taskUnits[0].hostLabels = %+v, want [gpu prod]", taskLabels)
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

func TestHandleOrchestratorExecutionsRetryRequiresFailedTasks(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))

	mux := buildRemoteFeatureMux(t)
	seed, err := upsertOrchestratorExecution(OrchestratorExecution{
		ID:              "exec-retry-none",
		Goal:            "nothing to retry",
		ApprovalScope:   "infrastructure_only",
		Status:          OrchestratorExecutionStatusCompleted,
		RequiredWorkers: []OrchestratorRequiredWorker{{HostID: orchestratorLocalHostID, AgentID: "zeroclaw", Count: 1}},
		TaskUnits: []OrchestratorTaskUnit{
			{ID: "task-1", Input: "collect logs", HostID: orchestratorLocalHostID, AgentID: "zeroclaw"},
		},
		Results: []OrchestratorTaskResult{
			{TaskID: "task-1", Status: OrchestratorTaskStatusCompleted, Summary: "done"},
		},
	})
	if err != nil {
		t.Fatalf("seed execution failed: %v", err)
	}

	rec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions/"+seed.ID+"/retry", `{}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected retry conflict status, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "E_ORCHESTRATOR_RETRY_NOTHING") {
		t.Fatalf("expected retry empty error code, got body=%s", rec.Body.String())
	}
}

func TestHandleOrchestratorExecutionsRetryCreatesDerivedExecution(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))

	mux := buildRemoteFeatureMux(t)
	seed, err := upsertOrchestratorExecution(OrchestratorExecution{
		ID:              "exec-retry-source",
		Goal:            "retry failing step",
		ApprovalScope:   "infrastructure_only",
		Status:          OrchestratorExecutionStatusRetryableFailed,
		RequiredWorkers: []OrchestratorRequiredWorker{{HostID: orchestratorLocalHostID, AgentID: "zeroclaw", Count: 1}},
		TaskUnits: []OrchestratorTaskUnit{
			{ID: "task-1", Input: "collect logs", HostID: orchestratorLocalHostID, AgentID: "zeroclaw"},
			{ID: "task-2", Input: "summarize logs", HostID: orchestratorLocalHostID, AgentID: "zeroclaw"},
		},
		Results: []OrchestratorTaskResult{
			{TaskID: "task-1", Status: OrchestratorTaskStatusCompleted, Summary: "done"},
			{TaskID: "task-2", Status: OrchestratorTaskStatusFailed, Summary: "provider timeout", FailureReason: "timeout", FailureCategory: "provider_failed"},
		},
	})
	if err != nil {
		t.Fatalf("seed execution failed: %v", err)
	}

	rec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions/"+seed.ID+"/retry", `{}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected retry created status, got %d body=%s", rec.Code, rec.Body.String())
	}
	payload := decodeJSONMap(t, rec)
	execMap, _ := payload["execution"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(execMap["status"])); got != string(OrchestratorExecutionStatusPendingAuthorization) {
		t.Fatalf("retry status=%q, want pending_authorization payload=%+v", got, payload)
	}
	if got := strings.TrimSpace(anyToString(execMap["parentExecutionId"])); got != seed.ID {
		t.Fatalf("parentExecutionId=%q, want %s", got, seed.ID)
	}
	if got := strings.TrimSpace(anyToString(execMap["sourceExecutionId"])); got != seed.ID {
		t.Fatalf("sourceExecutionId=%q, want %s", got, seed.ID)
	}
	if got := strings.TrimSpace(anyToString(execMap["launchReason"])); got != "retry_failed_tasks" {
		t.Fatalf("launchReason=%q, want retry_failed_tasks", got)
	}
	taskUnits, _ := execMap["taskUnits"].([]interface{})
	if len(taskUnits) != 1 {
		t.Fatalf("retry taskUnits=%d, want 1 payload=%+v", len(taskUnits), payload)
	}
	firstTask, _ := taskUnits[0].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(firstTask["id"])); got != "task-2" {
		t.Fatalf("retry task id=%q, want task-2", got)
	}
	results, _ := execMap["results"].([]interface{})
	if len(results) != 0 {
		t.Fatalf("retry results len=%d, want 0", len(results))
	}
}

func TestHandleOrchestratorExecutionsRerunAndCloneCreateDerivedExecutions(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))

	mux := buildRemoteFeatureMux(t)
	seed, err := upsertOrchestratorExecution(OrchestratorExecution{
		ID:                "exec-derived-source",
		Goal:              "rerun and clone me",
		RequestedProvider: "openrouter",
		ApprovalScope:     "infrastructure_only",
		Status:            OrchestratorExecutionStatusCompleted,
		Authorization: OrchestratorAuthorization{
			InfrastructureApproved: true,
			ApprovedBy:             "operator",
			ApprovedAt:             nowTimestamp(),
		},
		Policy: OrchestratorExecutionPolicySnapshot{
			Decision:        orchestratorPolicyDecisionAsk,
			Reason:          "manual review required",
			MatchedRuleID:   "rule-1",
			MatchedRuleName: "review production",
			ApprovedBy:      "reviewer",
			ApprovedAt:      nowTimestamp(),
		},
		RequiredWorkers: []OrchestratorRequiredWorker{{HostID: orchestratorLocalHostID, AgentID: "picoclaw", Count: 1}},
		TaskUnits: []OrchestratorTaskUnit{
			{ID: "task-1", Input: "collect logs", HostID: orchestratorLocalHostID, AgentID: "picoclaw"},
		},
		Results: []OrchestratorTaskResult{
			{TaskID: "task-1", Status: OrchestratorTaskStatusCompleted, Summary: "done"},
		},
	})
	if err != nil {
		t.Fatalf("seed execution failed: %v", err)
	}

	rerunRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions/"+seed.ID+"/rerun", `{}`)
	if rerunRec.Code != http.StatusCreated {
		t.Fatalf("expected rerun created status, got %d body=%s", rerunRec.Code, rerunRec.Body.String())
	}
	rerunPayload := decodeJSONMap(t, rerunRec)
	rerunExec, _ := rerunPayload["execution"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(rerunExec["launchReason"])); got != "rerun_execution" {
		t.Fatalf("rerun launchReason=%q, want rerun_execution", got)
	}
	if got := strings.TrimSpace(anyToString(rerunExec["status"])); got != string(OrchestratorExecutionStatusPendingAuthorization) {
		t.Fatalf("rerun status=%q, want pending_authorization", got)
	}
	if auth, _ := rerunExec["authorization"].(map[string]interface{}); anyToString(auth["approvedBy"]) != "" {
		t.Fatalf("rerun authorization should be reset: %+v", auth)
	}
	if policy, _ := rerunExec["policy"].(map[string]interface{}); strings.TrimSpace(anyToString(policy["matchedRuleName"])) != "review production" || strings.TrimSpace(anyToString(policy["approvedBy"])) != "" {
		t.Fatalf("rerun policy mismatch/reset failure: %+v", policy)
	}

	cloneRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions/"+seed.ID+"/clone", `{}`)
	if cloneRec.Code != http.StatusCreated {
		t.Fatalf("expected clone created status, got %d body=%s", cloneRec.Code, cloneRec.Body.String())
	}
	clonePayload := decodeJSONMap(t, cloneRec)
	cloneExec, _ := clonePayload["execution"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(cloneExec["launchReason"])); got != "clone_execution" {
		t.Fatalf("clone launchReason=%q, want clone_execution", got)
	}
	results, _ := cloneExec["results"].([]interface{})
	if len(results) != 0 {
		t.Fatalf("clone results len=%d, want 0", len(results))
	}
	taskUnits, _ := cloneExec["taskUnits"].([]interface{})
	if len(taskUnits) != 1 {
		t.Fatalf("clone taskUnits len=%d, want 1", len(taskUnits))
	}
}

func TestHandleOrchestratorExecutionArtifactsListAndDownload(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))
	artifactRoot, err := os.MkdirTemp(".", "artifact-root-*")
	if err != nil {
		t.Fatalf("mkdir artifact root: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(artifactRoot) })
	artifactPath := filepath.Join(artifactRoot, "release-notes.txt")
	if err = os.WriteFile(artifactPath, []byte("draft release notes"), 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	mux := buildRemoteFeatureMuxWithConfigAndDaemonHandlers(t, &GatewayConfig{
		APIToken:                  "test-gateway-token",
		MaxCommandBodyBytes:       64 * 1024,
		RemoteControlPlaneEnabled: true,
		RemoteChatEnabled:         true,
		ProviderBindingEnabled:    true,
		ArtifactRoot:              artifactRoot,
	}, nil)

	seed, err := upsertOrchestratorExecution(OrchestratorExecution{
		ID:              "exec-artifacts-1",
		Goal:            "collect artifacts",
		ApprovalScope:   "infrastructure_only",
		Status:          OrchestratorExecutionStatusCompleted,
		RequiredWorkers: []OrchestratorRequiredWorker{{HostID: orchestratorLocalHostID, AgentID: "zeroclaw", Count: 1}},
		TaskUnits:       []OrchestratorTaskUnit{{ID: "task-1", Input: "build notes", HostID: orchestratorLocalHostID, AgentID: "zeroclaw"}},
		Outcome: OrchestratorExecutionOutcome{
			Summary: "artifact produced",
			Artifacts: []OrchestratorArtifact{
				{
					ID:          "artifact-1",
					TaskID:      "task-1",
					Name:        "release-notes.txt",
					Kind:        "text",
					ContentType: "text/plain; charset=utf-8",
					SizeBytes:   int64(len("draft release notes")),
					Path:        artifactPath,
					CreatedAt:   nowTimestamp(),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("seed execution failed: %v", err)
	}

	listRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/orchestrator/executions/"+seed.ID+"/artifacts", "")
	if listRec.Code != http.StatusOK {
		t.Fatalf("artifact list status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	listPayload := decodeJSONMap(t, listRec)
	artifacts, _ := listPayload["artifacts"].([]interface{})
	if len(artifacts) != 1 {
		t.Fatalf("artifacts len=%d, want 1 payload=%+v", len(artifacts), listPayload)
	}
	artifactMap, _ := artifacts[0].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(artifactMap["id"])); got != "artifact-1" {
		t.Fatalf("artifact id=%q, want artifact-1", got)
	}

	downloadReq := httptest.NewRequest(http.MethodGet, "/api/v1/orchestrator/executions/"+seed.ID+"/artifacts/artifact-1", nil)
	downloadReq.Header.Set("Authorization", "Bearer test-gateway-token")
	downloadRec := httptest.NewRecorder()
	mux.ServeHTTP(downloadRec, downloadReq)
	if downloadRec.Code != http.StatusOK {
		t.Fatalf("artifact download status=%d body=%s", downloadRec.Code, downloadRec.Body.String())
	}
	if got := downloadRec.Header().Get("Content-Type"); !strings.Contains(got, "text/plain") {
		t.Fatalf("content-type=%q, want text/plain", got)
	}
	if got := strings.TrimSpace(downloadRec.Body.String()); got != "draft release notes" {
		t.Fatalf("download body=%q, want draft release notes", got)
	}
}

func TestHandleOrchestratorExecutionArtifactsMissingCases(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))
	artifactRoot, err := os.MkdirTemp(".", "artifact-root-*")
	if err != nil {
		t.Fatalf("mkdir artifact root: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(artifactRoot) })
	outsideDir := t.TempDir()
	outsidePath := filepath.Join(outsideDir, "outside.txt")
	if err = os.WriteFile(outsidePath, []byte("outside"), 0o600); err != nil {
		t.Fatalf("write outside artifact: %v", err)
	}

	mux := buildRemoteFeatureMuxWithConfigAndDaemonHandlers(t, &GatewayConfig{
		APIToken:                  "test-gateway-token",
		MaxCommandBodyBytes:       64 * 1024,
		RemoteControlPlaneEnabled: true,
		RemoteChatEnabled:         true,
		ProviderBindingEnabled:    true,
		ArtifactRoot:              artifactRoot,
	}, nil)

	seed, err := upsertOrchestratorExecution(OrchestratorExecution{
		ID:              "exec-artifacts-missing",
		Goal:            "bad artifact path",
		ApprovalScope:   "infrastructure_only",
		Status:          OrchestratorExecutionStatusCompleted,
		RequiredWorkers: []OrchestratorRequiredWorker{{HostID: orchestratorLocalHostID, AgentID: "zeroclaw", Count: 1}},
		TaskUnits:       []OrchestratorTaskUnit{{ID: "task-1", Input: "noop", HostID: orchestratorLocalHostID, AgentID: "zeroclaw"}},
		Outcome: OrchestratorExecutionOutcome{
			Artifacts: []OrchestratorArtifact{
				{ID: "artifact-unsafe", Name: "outside.txt", Path: outsidePath},
			},
		},
	})
	if err != nil {
		t.Fatalf("seed execution failed: %v", err)
	}

	methodRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions/"+seed.ID+"/artifacts", `{}`)
	if methodRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("artifact list method status=%d body=%s", methodRec.Code, methodRec.Body.String())
	}

	missingRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/orchestrator/executions/"+seed.ID+"/artifacts/missing", "")
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("missing artifact status=%d body=%s", missingRec.Code, missingRec.Body.String())
	}

	unsafeReq := httptest.NewRequest(http.MethodGet, "/api/v1/orchestrator/executions/"+seed.ID+"/artifacts/artifact-unsafe", nil)
	unsafeReq.Header.Set("Authorization", "Bearer test-gateway-token")
	unsafeRec := httptest.NewRecorder()
	mux.ServeHTTP(unsafeRec, unsafeReq)
	if unsafeRec.Code != http.StatusNotFound {
		t.Fatalf("unsafe artifact status=%d body=%s", unsafeRec.Code, unsafeRec.Body.String())
	}
}

func TestHandleOrchestratorExecutionsCreatePersistsPolicySnapshot(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))

	mux := buildRemoteFeatureMux(t)
	rec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions", `{
		"goal":"triage production incident",
		"approvalScope":"infrastructure_only",
		"toolPolicy":{"mode":"restricted","allowedTools":["shell","grep","shell"]},
		"maxConcurrency":9,
		"requiredWorkers":[
			{"hostId":"local","agentId":"zeroclaw","count":1},
			{"hostId":"host-b","agentId":"picoclaw","count":2}
		],
		"taskUnits":[
			{"id":"t1","input":"collect traces","hostId":"local","agentId":"zeroclaw","timeoutMs":45000,"retryBudget":1},
			{"id":"t2","input":"summarize traces","hostId":"host-b","agentId":"picoclaw","timeoutMs":120000,"retryBudget":3}
		]
	}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create execution status=%d body=%s", rec.Code, rec.Body.String())
	}

	payload := decodeJSONMap(t, rec)
	execMap, _ := payload["execution"].(map[string]interface{})
	policy, _ := execMap["policy"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(policy["decision"])); got != "allow" {
		t.Fatalf("policy.decision = %q, want allow payload=%+v", got, payload)
	}
	if got := int(anyToFloat(policy["configuredMaxConcurrency"])); got != 9 {
		t.Fatalf("policy.configuredMaxConcurrency = %d, want 9", got)
	}
	if got := int(anyToFloat(policy["effectiveMaxConcurrency"])); got != 2 {
		t.Fatalf("policy.effectiveMaxConcurrency = %d, want 2", got)
	}
	if got := int(anyToFloat(policy["maxTaskTimeoutMs"])); got != 120000 {
		t.Fatalf("policy.maxTaskTimeoutMs = %d, want 120000", got)
	}
	if got := int(anyToFloat(policy["maxRetryBudget"])); got != 3 {
		t.Fatalf("policy.maxRetryBudget = %d, want 3", got)
	}
	if got := strings.TrimSpace(anyToString(policy["summary"])); got == "" {
		t.Fatalf("expected non-empty policy summary payload=%+v", payload)
	}

	toolPolicy, _ := policy["toolPolicy"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(toolPolicy["mode"])); got != "restricted" {
		t.Fatalf("tool policy mode = %q, want restricted", got)
	}
	allowedTools, _ := toolPolicy["allowedTools"].([]interface{})
	if len(allowedTools) != 2 {
		t.Fatalf("allowedTools len = %d, want 2 payload=%+v", len(allowedTools), policy)
	}

	targets, _ := policy["targets"].([]interface{})
	if len(targets) != 2 {
		t.Fatalf("policy targets len = %d, want 2 payload=%+v", len(targets), policy)
	}
}

func TestHandleOrchestratorPoliciesCRUDAndExecutionEnforcement(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))

	mux := buildRemoteFeatureMux(t)

	denyRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/policies", `{
		"name":"deny anthropic on picoclaw",
		"action":"deny",
		"reason":"anthropic is not approved for picoclaw workloads",
		"requestedProviders":["anthropic"],
		"agentIds":["picoclaw"]
	}`)
	if denyRec.Code != http.StatusOK {
		t.Fatalf("create deny policy status=%d body=%s", denyRec.Code, denyRec.Body.String())
	}
	denyPayload := decodeJSONMap(t, denyRec)
	denyRule, _ := denyPayload["policy"].(map[string]interface{})
	denyRuleID := strings.TrimSpace(anyToString(denyRule["id"]))
	if denyRuleID == "" {
		t.Fatalf("missing deny rule id payload=%+v", denyPayload)
	}

	askRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/policies", `{
		"name":"review picoclaw production runs",
		"action":"ask",
		"reason":"picoclaw on host-b needs explicit operator acknowledgement",
		"hostIds":["host-b"],
		"agentIds":["picoclaw"],
		"priority":50
	}`)
	if askRec.Code != http.StatusOK {
		t.Fatalf("create ask policy status=%d body=%s", askRec.Code, askRec.Body.String())
	}

	listRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/orchestrator/policies", "")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list policies status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	listPayload := decodeJSONMap(t, listRec)
	policies, _ := listPayload["policies"].([]interface{})
	if len(policies) != 2 {
		t.Fatalf("policies len=%d want 2 payload=%+v", len(policies), listPayload)
	}

	deniedCreate := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions", `{
		"goal":"triage anthropic failure",
		"requestedProvider":"anthropic",
		"approvalScope":"infrastructure_only",
		"requiredWorkers":[{"hostId":"host-b","agentId":"picoclaw","count":1}],
		"taskUnits":[{"id":"t1","input":"triage failure","hostId":"host-b","agentId":"picoclaw"}]
	}`)
	if deniedCreate.Code != http.StatusForbidden {
		t.Fatalf("expected deny create status 403, got %d body=%s", deniedCreate.Code, deniedCreate.Body.String())
	}
	deniedPayload := decodeJSONMap(t, deniedCreate)
	policyMap, _ := deniedPayload["policy"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(policyMap["decision"])); got != "deny" {
		t.Fatalf("policy decision=%q want deny payload=%+v", got, deniedPayload)
	}

	askedCreate := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions", `{
		"goal":"summarize traces",
		"approvalScope":"infrastructure_only",
		"requiredWorkers":[{"hostId":"host-b","agentId":"picoclaw","count":1}],
		"taskUnits":[{"id":"t1","input":"summarize traces","hostId":"host-b","agentId":"picoclaw"}]
	}`)
	if askedCreate.Code != http.StatusCreated {
		t.Fatalf("expected ask create status 201, got %d body=%s", askedCreate.Code, askedCreate.Body.String())
	}
	askedPayload := decodeJSONMap(t, askedCreate)
	execMap, _ := askedPayload["execution"].(map[string]interface{})
	execID := strings.TrimSpace(anyToString(execMap["id"]))
	if execID == "" {
		t.Fatalf("expected execution id payload=%+v", askedPayload)
	}
	execPolicy, _ := execMap["policy"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(execPolicy["decision"])); got != "ask" {
		t.Fatalf("policy decision=%q want ask payload=%+v", got, askedPayload)
	}

	blockedAuthorize := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions/"+execID+"/authorize", `{
		"approved":true,
		"actor":"carrier-cli"
	}`)
	if blockedAuthorize.Code != http.StatusConflict {
		t.Fatalf("expected authorize blocked status 409, got %d body=%s", blockedAuthorize.Code, blockedAuthorize.Body.String())
	}

	approvedAuthorize := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions/"+execID+"/authorize", `{
		"approved":true,
		"actor":"carrier-cli",
		"policyApproved":true
	}`)
	if approvedAuthorize.Code != http.StatusAccepted {
		t.Fatalf("expected authorize accepted status 202, got %d body=%s", approvedAuthorize.Code, approvedAuthorize.Body.String())
	}
	approvedPayload := decodeJSONMap(t, approvedAuthorize)
	approvedExec, _ := approvedPayload["execution"].(map[string]interface{})
	approvedPolicy, _ := approvedExec["policy"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(approvedPolicy["approvedBy"])); got != "carrier-cli" {
		t.Fatalf("policy approvedBy=%q want carrier-cli payload=%+v", got, approvedPayload)
	}

	deleteRec := runJSONRequest(t, mux, http.MethodDelete, "/api/v1/orchestrator/policies/"+denyRuleID, "")
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete policy status=%d body=%s", deleteRec.Code, deleteRec.Body.String())
	}
}

func TestHandleOrchestratorPoliciesApplyConstraintsAndHostLabels(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))

	mux := buildRemoteFeatureMux(t)

	host, err := upsertRemoteHost(RemoteHost{
		Name:        "prod-gpu-1",
		Host:        "10.0.0.10",
		Port:        22,
		User:        "ubuntu",
		AuthMode:    RemoteAuthModePrivateKey,
		KeyPath:     filepath.Join(t.TempDir(), "id.key"),
		Labels:      []string{"prod", "gpu", "prod"},
		RuntimeMode: RemoteRuntimeModeOnDemand,
	})
	if err != nil {
		t.Fatalf("upsertRemoteHost failed: %v", err)
	}

	policyRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/policies", `{
		"name":"constrain prod gpu picoclaw runs",
		"action":"allow",
		"reason":"prod gpu runs must stay within bounded tools and retries",
		"hostLabels":["prod","gpu"],
		"agentIds":["picoclaw"],
		"allowedTools":["shell","grep"],
		"maxTaskTimeoutMs":45000,
		"maxRetryBudget":1
	}`)
	if policyRec.Code != http.StatusOK {
		t.Fatalf("create constraint policy status=%d body=%s", policyRec.Code, policyRec.Body.String())
	}

	createRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions", `{
		"goal":"diagnose prod gpu drift",
		"approvalScope":"infrastructure_only",
		"toolPolicy":{"mode":"workspace_write","allowedTools":["grep","write_file","shell"]},
		"requiredWorkers":[{"hostId":"`+host.ID+`","agentId":"picoclaw","count":1}],
		"taskUnits":[
			{"id":"t1","input":"diagnose","hostId":"`+host.ID+`","agentId":"picoclaw","timeoutMs":120000,"retryBudget":3}
		]
	}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create constrained execution status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	createPayload := decodeJSONMap(t, createRec)
	execMap, _ := createPayload["execution"].(map[string]interface{})
	policyMap, _ := execMap["policy"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(policyMap["matchedRuleName"])); got != "constrain prod gpu picoclaw runs" {
		t.Fatalf("matchedRuleName=%q payload=%+v", got, createPayload)
	}
	if got := int(anyToFloat(policyMap["maxTaskTimeoutMs"])); got != 45000 {
		t.Fatalf("policy.maxTaskTimeoutMs=%d want 45000 payload=%+v", got, createPayload)
	}
	if got := int(anyToFloat(policyMap["maxRetryBudget"])); got != 1 {
		t.Fatalf("policy.maxRetryBudget=%d want 1 payload=%+v", got, createPayload)
	}
	toolPolicy, _ := policyMap["toolPolicy"].(map[string]interface{})
	allowedTools, _ := toolPolicy["allowedTools"].([]interface{})
	if len(allowedTools) != 2 {
		t.Fatalf("policy allowedTools len=%d want 2 payload=%+v", len(allowedTools), createPayload)
	}
	execToolPolicy, _ := execMap["toolPolicy"].(map[string]interface{})
	execAllowedTools, _ := execToolPolicy["allowedTools"].([]interface{})
	if len(execAllowedTools) != 2 {
		t.Fatalf("execution allowedTools len=%d want 2 payload=%+v", len(execAllowedTools), createPayload)
	}
	taskUnits, _ := execMap["taskUnits"].([]interface{})
	if len(taskUnits) != 1 {
		t.Fatalf("taskUnits len=%d want 1 payload=%+v", len(taskUnits), createPayload)
	}
	taskMap, _ := taskUnits[0].(map[string]interface{})
	if got := int(anyToFloat(taskMap["timeoutMs"])); got != 45000 {
		t.Fatalf("task timeoutMs=%d want 45000 payload=%+v", got, createPayload)
	}
	if got := int(anyToFloat(taskMap["retryBudget"])); got != 1 {
		t.Fatalf("task retryBudget=%d want 1 payload=%+v", got, createPayload)
	}

	unmatchedHost, err := upsertRemoteHost(RemoteHost{
		Name:        "staging-1",
		Host:        "10.0.0.11",
		Port:        22,
		User:        "ubuntu",
		AuthMode:    RemoteAuthModePrivateKey,
		KeyPath:     filepath.Join(t.TempDir(), "id-2.key"),
		Labels:      []string{"staging"},
		RuntimeMode: RemoteRuntimeModeOnDemand,
	})
	if err != nil {
		t.Fatalf("upsert unmatched host failed: %v", err)
	}
	unmatchedRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions", `{
		"goal":"diagnose staging drift",
		"approvalScope":"infrastructure_only",
		"toolPolicy":{"mode":"workspace_write","allowedTools":["grep","write_file","shell"]},
		"requiredWorkers":[{"hostId":"`+unmatchedHost.ID+`","agentId":"picoclaw","count":1}],
		"taskUnits":[
			{"id":"t1","input":"diagnose","hostId":"`+unmatchedHost.ID+`","agentId":"picoclaw","timeoutMs":120000,"retryBudget":3}
		]
	}`)
	if unmatchedRec.Code != http.StatusCreated {
		t.Fatalf("create unmatched execution status=%d body=%s", unmatchedRec.Code, unmatchedRec.Body.String())
	}
	unmatchedPayload := decodeJSONMap(t, unmatchedRec)
	unmatchedExec, _ := unmatchedPayload["execution"].(map[string]interface{})
	unmatchedPolicy, _ := unmatchedExec["policy"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(unmatchedPolicy["matchedRuleName"])); got != "" {
		t.Fatalf("expected empty matchedRuleName for unmatched host labels, got %q payload=%+v", got, unmatchedPayload)
	}
	unmatchedTasks, _ := unmatchedExec["taskUnits"].([]interface{})
	unmatchedTask, _ := unmatchedTasks[0].(map[string]interface{})
	if got := int(anyToFloat(unmatchedTask["timeoutMs"])); got != 120000 {
		t.Fatalf("unmatched task timeoutMs=%d want 120000 payload=%+v", got, unmatchedPayload)
	}
	if got := int(anyToFloat(unmatchedTask["retryBudget"])); got != 3 {
		t.Fatalf("unmatched task retryBudget=%d want 3 payload=%+v", got, unmatchedPayload)
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

func TestHandleOrchestratorWorkersInventory(t *testing.T) {
	mux := buildRemoteFeatureMuxWithDaemonHandlers(t, map[string]http.HandlerFunc{
		"GET /api/v1/agents": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"agents":[
				{"id":"zeroclaw","installState":"installed","runtimeState":"running","health":"healthy","updatedAt":"2026-03-08T12:00:00Z"},
				{"id":"picoclaw","installState":"installed","runtimeState":"stopped","health":"healthy","updatedAt":"2026-03-08T11:58:00Z"},
				{"id":"openclaw","installState":"not_installed","runtimeState":"stopped","health":"unknown","updatedAt":"2026-03-08T11:50:00Z"}
			]}`))
		},
	})
	hostID := createRemoteHostForTests(t, mux)

	if _, err := patchRemoteHost(hostID, RemoteHost{
		Name:        "prod-host-1",
		RuntimeMode: RemoteRuntimeModeManagedGateway,
		LastHealth:  RemoteHealthHealthy,
	}); err != nil {
		t.Fatalf("patchRemoteHost failed: %v", err)
	}
	if _, err := upsertRemoteInstanceSyncStatus(RemoteInstanceSyncStatus{
		HostID:         hostID,
		AgentID:        "picoclaw",
		SyncMode:       providerBindingSyncModeAlwaysPush,
		DriftState:     "in_sync",
		LastSyncStatus: "success",
		UpdatedAt:      "2026-03-08T12:01:00Z",
	}); err != nil {
		t.Fatalf("upsertRemoteInstanceSyncStatus failed: %v", err)
	}
	if _, err := upsertOrchestratorWorkerLease(OrchestratorWorkerLease{
		ID:             "lease-remote-1",
		ExecutionID:    "exec-1",
		HostID:         hostID,
		AgentID:        "zeroclaw",
		State:          OrchestratorWorkerStateBusy,
		Ephemeral:      true,
		InstalledByRun: true,
		TaskCount:      2,
		HeartbeatAt:    "2026-03-08T12:02:00Z",
		LeaseExpireAt:  "2026-03-08T12:10:00Z",
		CreatedAt:      "2026-03-08T12:02:00Z",
		UpdatedAt:      "2026-03-08T12:02:00Z",
	}); err != nil {
		t.Fatalf("upsertOrchestratorWorkerLease failed: %v", err)
	}

	rec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/orchestrator/workers", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected workers status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	payload := decodeJSONMap(t, rec)
	workers, _ := payload["workers"].([]interface{})
	if len(workers) != 4 {
		t.Fatalf("workers = %d, want 4 payload=%+v", len(workers), payload)
	}
	summary, _ := payload["summary"].(map[string]interface{})
	if got := int(anyToFloat(summary["total"])); got != 4 {
		t.Fatalf("summary.total=%d want 4 summary=%+v", got, summary)
	}
	if got := int(anyToFloat(summary["busy"])); got != 1 {
		t.Fatalf("summary.busy=%d want 1 summary=%+v", got, summary)
	}
	if got := int(anyToFloat(summary["local"])); got != 2 {
		t.Fatalf("summary.local=%d want 2 summary=%+v", got, summary)
	}
	if got := int(anyToFloat(summary["remote"])); got != 2 {
		t.Fatalf("summary.remote=%d want 2 summary=%+v", got, summary)
	}

	var foundLocalRunning bool
	var foundRemoteManaged bool
	var foundBusyLease bool
	for _, raw := range workers {
		item, _ := raw.(map[string]interface{})
		source := strings.TrimSpace(anyToString(item["source"]))
		host := strings.TrimSpace(anyToString(item["hostId"]))
		agent := strings.TrimSpace(anyToString(item["agentId"]))
		state := strings.TrimSpace(anyToString(item["state"]))
		if source == "local" && host == orchestratorLocalHostID && agent == "zeroclaw" && state == "available" {
			foundLocalRunning = true
		}
		if source == "remote_sync" && host == hostID && agent == "picoclaw" && state == "managed" {
			foundRemoteManaged = true
		}
		if source == "lease" && host == hostID && agent == "zeroclaw" && state == string(OrchestratorWorkerStateBusy) {
			foundBusyLease = true
		}
	}
	if !foundLocalRunning {
		t.Fatalf("expected local running worker in payload=%+v", payload)
	}
	if !foundRemoteManaged {
		t.Fatalf("expected remote managed worker in payload=%+v", payload)
	}
	if !foundBusyLease {
		t.Fatalf("expected busy leased worker in payload=%+v", payload)
	}
}

func TestHandleOrchestratorWorkersInventoryNegativeCases(t *testing.T) {
	disabledMux := buildRemoteFeatureMuxWithConfigAndDaemonHandlers(t, &GatewayConfig{
		APIToken:                  "test-gateway-token",
		MaxCommandBodyBytes:       64 * 1024,
		RemoteControlPlaneEnabled: false,
		RemoteChatEnabled:         true,
		ProviderBindingEnabled:    true,
	}, nil)
	disabledRec := runJSONRequest(t, disabledMux, http.MethodGet, "/api/v1/orchestrator/workers", "")
	if disabledRec.Code != http.StatusNotFound {
		t.Fatalf("expected disabled workers status 404, got %d body=%s", disabledRec.Code, disabledRec.Body.String())
	}

	mux := buildRemoteFeatureMux(t)
	methodRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/workers", `{}`)
	if methodRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected workers method not allowed, got %d body=%s", methodRec.Code, methodRec.Body.String())
	}
}

func TestReclaimStaleWorkersEndpointAndQueueSummary(t *testing.T) {
	mux := buildRemoteFeatureMuxWithConfigAndDaemonHandlers(t, &GatewayConfig{
		APIToken:                  "test-gateway-token",
		MaxCommandBodyBytes:       64 * 1024,
		RemoteControlPlaneEnabled: true,
		RemoteChatEnabled:         true,
		ProviderBindingEnabled:    true,
		WorkerLeaseStaleAfter:     5 * time.Minute,
		WorkerHeartbeatTimeout:    2 * time.Minute,
	}, nil)
	hostID := createRemoteHostForTests(t, mux)

	if _, err := upsertOrchestratorExecution(OrchestratorExecution{
		ID:        "exec-running",
		Goal:      "investigate latency",
		Status:    OrchestratorExecutionStatusRunning,
		TaskUnits: []OrchestratorTaskUnit{{ID: "task-1", Input: "collect logs"}, {ID: "task-2", Input: "summarize"}, {ID: "task-3", Input: "report findings"}},
		Results:   []OrchestratorTaskResult{{TaskID: "task-1", Status: OrchestratorTaskStatusCompleted}},
		CreatedAt: nowTimestamp(),
		UpdatedAt: nowTimestamp(),
		StartedAt: nowTimestamp(),
		Authorization: OrchestratorAuthorization{
			InfrastructureApproved: true,
		},
	}); err != nil {
		t.Fatalf("upsertOrchestratorExecution running failed: %v", err)
	}
	if _, err := upsertOrchestratorExecution(OrchestratorExecution{
		ID:          "exec-done",
		Goal:        "finished task",
		Status:      OrchestratorExecutionStatusCompleted,
		TaskUnits:   []OrchestratorTaskUnit{{ID: "task-3", Input: "done"}},
		Results:     []OrchestratorTaskResult{{TaskID: "task-3", Status: OrchestratorTaskStatusCompleted}},
		CreatedAt:   nowTimestamp(),
		UpdatedAt:   nowTimestamp(),
		CompletedAt: nowTimestamp(),
	}); err != nil {
		t.Fatalf("upsertOrchestratorExecution completed failed: %v", err)
	}

	if _, err := upsertOrchestratorWorkerLease(OrchestratorWorkerLease{
		ID:            "lease-stale-terminal",
		ExecutionID:   "exec-done",
		HostID:        hostID,
		AgentID:       "picoclaw",
		State:         OrchestratorWorkerStateBusy,
		Ephemeral:     false,
		HeartbeatAt:   time.Now().UTC().Add(-30 * time.Second).Format(time.RFC3339),
		LeaseExpireAt: time.Now().UTC().Add(5 * time.Minute).Format(time.RFC3339),
		CreatedAt:     nowTimestamp(),
		UpdatedAt:     nowTimestamp(),
	}); err != nil {
		t.Fatalf("upsert stale terminal lease failed: %v", err)
	}
	if _, err := upsertOrchestratorWorkerLease(OrchestratorWorkerLease{
		ID:            "lease-busy-healthy",
		ExecutionID:   "exec-running",
		HostID:        hostID,
		AgentID:       "zeroclaw",
		State:         OrchestratorWorkerStateBusy,
		Ephemeral:     false,
		HeartbeatAt:   time.Now().UTC().Add(-30 * time.Second).Format(time.RFC3339),
		LeaseExpireAt: time.Now().UTC().Add(5 * time.Minute).Format(time.RFC3339),
		CreatedAt:     nowTimestamp(),
		UpdatedAt:     nowTimestamp(),
	}); err != nil {
		t.Fatalf("upsert healthy lease failed: %v", err)
	}

	queueRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/orchestrator/workers/queue", "")
	if queueRec.Code != http.StatusOK {
		t.Fatalf("expected queue status 200, got %d body=%s", queueRec.Code, queueRec.Body.String())
	}
	queuePayload := decodeJSONMap(t, queueRec)
	summary, _ := queuePayload["summary"].(map[string]interface{})
	if got := int(anyToFloat(summary["activeExecutions"])); got != 1 {
		t.Fatalf("activeExecutions=%d want 1 summary=%+v", got, summary)
	}
	if got := int(anyToFloat(summary["queuedTasks"])); got != 1 {
		t.Fatalf("queuedTasks=%d want 1 summary=%+v", got, summary)
	}
	if got := int(anyToFloat(summary["staleLeases"])); got != 1 {
		t.Fatalf("staleLeases=%d want 1 summary=%+v", got, summary)
	}
	if got := int(anyToFloat(summary["reclaimableWorkers"])); got != 1 {
		t.Fatalf("reclaimableWorkers=%d want 1 summary=%+v", got, summary)
	}

	reclaimRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/workers/reclaim-stale", `{}`)
	if reclaimRec.Code != http.StatusOK {
		t.Fatalf("expected reclaim-stale status 200, got %d body=%s", reclaimRec.Code, reclaimRec.Body.String())
	}
	reclaimPayload := decodeJSONMap(t, reclaimRec)
	reclaimMap, _ := reclaimPayload["reclaim"].(map[string]interface{})
	if got := int(anyToFloat(reclaimMap["reclaimed"])); got != 1 {
		t.Fatalf("reclaim-stale reclaimed=%d want 1 payload=%+v", got, reclaimPayload)
	}
	if got := int(anyToFloat(reclaimMap["skipped"])); got != 0 {
		t.Fatalf("reclaim-stale skipped=%d want 0 payload=%+v", got, reclaimPayload)
	}

	leases, err := listOrchestratorWorkerLeases()
	if err != nil {
		t.Fatalf("listOrchestratorWorkerLeases failed: %v", err)
	}
	stateByID := map[string]OrchestratorWorkerLease{}
	for _, lease := range leases {
		stateByID[lease.ID] = lease
	}
	if stateByID["lease-stale-terminal"].State != OrchestratorWorkerStateReclaimed {
		t.Fatalf("expected stale terminal lease reclaimed, got %+v", stateByID["lease-stale-terminal"])
	}
	if stateByID["lease-busy-healthy"].State != OrchestratorWorkerStateBusy {
		t.Fatalf("expected healthy busy lease preserved, got %+v", stateByID["lease-busy-healthy"])
	}
}

func TestOrchestratorExecutionCapturesGovernanceAndAudit(t *testing.T) {
	auditPath := filepath.Join(t.TempDir(), "gateway-audit.jsonl")
	t.Setenv("CARRIER_GATEWAY_AUDIT_LOG", auditPath)

	mux := buildRemoteFeatureMuxWithDaemonHandlers(t, map[string]http.HandlerFunc{
		"POST /api/v1/base-agent/authorize": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"approved":true}`))
		},
		"POST /api/v1/agents/status": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"statuses":[{"id":"zeroclaw","installState":"installed","runtimeState":"running"}]}`))
		},
	})
	hostID := createRemoteHostForTests(t, mux)

	if _, err := upsertProviderProfile(ProviderProfile{
		ID:       "profile-instance",
		Name:     "openrouter-instance",
		Provider: "openrouter",
		Model:    "openai/gpt-4o-mini",
		AuthRef:  "env:OPENROUTER_API_KEY",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("upsertProviderProfile failed: %v", err)
	}
	if _, err := upsertProviderBinding(ProviderBinding{
		ID:         "binding-instance",
		ProfileID:  "profile-instance",
		TargetType: "instance",
		TargetID:   hostID + ":zeroclaw",
		SyncMode:   providerBindingSyncModeManual,
	}); err != nil {
		t.Fatalf("upsertProviderBinding failed: %v", err)
	}

	createRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions", `{
		"goal":"governance audit",
		"idempotencyKey":"gov-audit-1",
		"approvalScope":"infrastructure_only",
		"requestedProvider":"openrouter",
		"requiredWorkers":[{"hostId":"`+hostID+`","agentId":"zeroclaw","count":1}],
		"taskUnits":[{"id":"t1","input":"hello","hostId":"`+hostID+`","agentId":"zeroclaw"}]
	}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create execution status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	createPayload := decodeJSONMap(t, createRec)
	execMap, _ := createPayload["execution"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(execMap["requestedProvider"])); got != "openrouter" {
		t.Fatalf("expected requestedProvider=openrouter, got %q payload=%+v", got, createPayload)
	}
	governance, _ := execMap["governance"].(map[string]interface{})
	resolutions, _ := governance["providerResolutions"].([]interface{})
	if len(resolutions) != 1 {
		t.Fatalf("expected 1 provider resolution, got %d governance=%+v", len(resolutions), governance)
	}
	resolution, _ := resolutions[0].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(resolution["source"])); got != "instance" {
		t.Fatalf("expected governance source instance, got %q resolution=%+v", got, resolution)
	}
	if got := strings.TrimSpace(anyToString(resolution["profileId"])); got != "profile-instance" {
		t.Fatalf("expected governance profile-instance, got %q resolution=%+v", got, resolution)
	}

	executionID := strings.TrimSpace(anyToString(execMap["id"]))
	if executionID == "" {
		t.Fatalf("missing execution id payload=%+v", createPayload)
	}

	authRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions/"+executionID+"/authorize", `{"approved":true,"actor":"auditor"}`)
	if authRec.Code != http.StatusAccepted {
		t.Fatalf("authorize status=%d body=%s", authRec.Code, authRec.Body.String())
	}

	cancelRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions/"+executionID+"/cancel", `{"actor":"auditor"}`)
	if cancelRec.Code != http.StatusAccepted {
		t.Fatalf("cancel status=%d body=%s", cancelRec.Code, cancelRec.Body.String())
	}

	rawAudit, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read audit log failed: %v", err)
	}
	text := string(rawAudit)
	if !strings.Contains(text, `"action":"orchestrator_execution_create"`) {
		t.Fatalf("expected orchestrator_execution_create audit entry, audit=%s", text)
	}
	if !strings.Contains(text, `"action":"orchestrator_execution_authorize"`) {
		t.Fatalf("expected orchestrator_execution_authorize audit entry, audit=%s", text)
	}
	if !strings.Contains(text, `"action":"orchestrator_execution_cancel"`) {
		t.Fatalf("expected orchestrator_execution_cancel audit entry, audit=%s", text)
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

	t.Run("resolve-required-worker-by-host-labels", func(t *testing.T) {
		t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))
		configureSSHRunner(t, func(command string) remoteExecResult {
			if strings.Contains(command, `if [ -f "$HOME/.picoclaw/config.toml" ]`) {
				return remoteExecResult{ExitCode: 0, Stdout: "1\n"}
			}
			return remoteExecResult{ExitCode: 0}
		})
		if _, err := upsertRemoteHost(RemoteHost{
			Name:        "prod-a",
			Host:        "127.0.0.1",
			Port:        22,
			User:        "ubuntu",
			AuthMode:    RemoteAuthModePrivateKey,
			KeyPath:     "~/.ssh/id_ed25519",
			Labels:      []string{"prod"},
			RuntimeMode: RemoteRuntimeModeOnDemand,
		}); err != nil {
			t.Fatalf("upsertRemoteHost hostA failed: %v", err)
		}
		hostB, err := upsertRemoteHost(RemoteHost{
			Name:        "gpu-b",
			Host:        "127.0.0.2",
			Port:        22,
			User:        "ubuntu",
			AuthMode:    RemoteAuthModePrivateKey,
			KeyPath:     "~/.ssh/id_ed25519",
			Labels:      []string{"prod", "gpu"},
			RuntimeMode: RemoteRuntimeModeOnDemand,
		})
		if err != nil {
			t.Fatalf("upsertRemoteHost hostB failed: %v", err)
		}
		leases, err := provisionOrchestratorWorkers(context.Background(), OrchestratorExecution{
			ID: "exec-labels",
			RequiredWorkers: []OrchestratorRequiredWorker{
				{HostLabels: []string{"gpu"}, AgentID: "picoclaw", Count: 1},
			},
		})
		if err != nil {
			t.Fatalf("provisionOrchestratorWorkers host-labels failed: %v", err)
		}
		if len(leases) != 1 || leases[0].HostID != hostB.ID {
			t.Fatalf("unexpected label-based lease selection: %+v want host %s", leases, hostB.ID)
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

	t.Run("acquire-by-host-labels", func(t *testing.T) {
		t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))
		hostA, err := upsertRemoteHost(RemoteHost{
			Name:        "prod-a",
			Host:        "127.0.0.1",
			Port:        22,
			User:        "ubuntu",
			AuthMode:    RemoteAuthModePrivateKey,
			KeyPath:     "~/.ssh/id_ed25519",
			Labels:      []string{"prod"},
			RuntimeMode: RemoteRuntimeModeOnDemand,
		})
		if err != nil {
			t.Fatalf("upsertRemoteHost hostA failed: %v", err)
		}
		hostB, err := upsertRemoteHost(RemoteHost{
			Name:        "gpu-b",
			Host:        "127.0.0.2",
			Port:        22,
			User:        "ubuntu",
			AuthMode:    RemoteAuthModePrivateKey,
			KeyPath:     "~/.ssh/id_ed25519",
			Labels:      []string{"prod", "gpu"},
			RuntimeMode: RemoteRuntimeModeOnDemand,
		})
		if err != nil {
			t.Fatalf("upsertRemoteHost hostB failed: %v", err)
		}
		poolAKey := workerPoolKey(hostA.ID, "picoclaw")
		poolBKey := workerPoolKey(hostB.ID, "picoclaw")
		poolA := orchestratorWorkerPool{key: poolAKey, ch: make(chan OrchestratorWorkerLease, 1)}
		poolB := orchestratorWorkerPool{key: poolBKey, ch: make(chan OrchestratorWorkerLease, 1)}
		poolA.ch <- OrchestratorWorkerLease{ID: "lease-a", HostID: hostA.ID, AgentID: "picoclaw"}
		poolB.ch <- OrchestratorWorkerLease{ID: "lease-b", HostID: hostB.ID, AgentID: "picoclaw"}
		lease, key, err := acquireWorkerForTask(context.Background(), OrchestratorTaskUnit{
			AgentID:    "picoclaw",
			HostLabels: []string{"gpu"},
		}, map[string]orchestratorWorkerPool{
			poolAKey: poolA,
			poolBKey: poolB,
		}, poolAKey)
		if err != nil {
			t.Fatalf("acquireWorkerForTask host-labels failed: %v", err)
		}
		if key != poolBKey || lease.ID != "lease-b" {
			t.Fatalf("unexpected host-label selection lease=%+v key=%q", lease, key)
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
