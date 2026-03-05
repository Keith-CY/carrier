package gateway

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestOrchestratorExecutionLifecycle(t *testing.T) {
	configureSSHRunner(t, func(command string) remoteExecResult {
		switch {
		case strings.Contains(command, "if [ -f \"$HOME/.zeroclaw/config.toml\" ]"):
			return remoteExecResult{ExitCode: 0, Stdout: "0\n"}
		case strings.Contains(command, "zeroclaw-") && strings.Contains(command, "curl -fsSL"):
			return remoteExecResult{ExitCode: 0, Stdout: "installed\n"}
		case strings.Contains(command, "zeroclaw task run"):
			return remoteExecResult{
				ExitCode: 0,
				Stdout:   `{"output_text":"worker-output","sessionId":"sess-1","memory":{"contractId":"cid","contractDigest":"dig","syncState":"ready","syncedAt":"2026-03-05T00:00:00Z"}}`,
			}
		case strings.Contains(command, "rm -f \"$HOME/.zeroclaw/config.toml\""):
			return remoteExecResult{ExitCode: 0}
		default:
			return remoteExecResult{ExitCode: 0}
		}
	})

	mux := buildRemoteFeatureMux(t)
	hostID := createRemoteHostForTests(t, mux)

	createRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions", `{
		"goal":"run one zeroclaw task",
		"idempotencyKey":"exec-001",
		"requiredWorkers":[{"hostId":"`+hostID+`","agentId":"zeroclaw","count":1}],
		"taskUnits":[{"id":"t1","input":"hello world","timeoutMs":30000,"retryBudget":0}],
		"toolPolicy":{"mode":"restricted","allowedTools":["read_file"]},
		"approvalScope":"infrastructure_only",
		"maxConcurrency":4
	}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create execution status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	createPayload := decodeJSONMap(t, createRec)
	execMap, _ := createPayload["execution"].(map[string]interface{})
	execID := strings.TrimSpace(anyToString(execMap["id"]))
	if execID == "" {
		t.Fatalf("missing execution id: %+v", createPayload)
	}

	authRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions/"+execID+"/authorize", `{"approved":true,"actor":"tester","maxConcurrency":4}`)
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
		t.Fatalf("expected one task result, got %d payload=%+v", len(results), execution)
	}
	resultMap, _ := results[0].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(resultMap["status"])); got != string(OrchestratorTaskStatusCompleted) {
		t.Fatalf("expected task completed, got %q result=%+v", got, resultMap)
	}
	if got := strings.TrimSpace(anyToString(resultMap["output"])); got != "worker-output" {
		t.Fatalf("expected task output worker-output, got %q result=%+v", got, resultMap)
	}

	workers, _ := finalPayload["workers"].([]interface{})
	if len(workers) != 1 {
		t.Fatalf("expected one worker lease, got %d payload=%+v", len(workers), finalPayload)
	}
	worker, _ := workers[0].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(worker["state"])); got != string(OrchestratorWorkerStateReclaimed) {
		t.Fatalf("expected worker reclaimed, got %q worker=%+v", got, worker)
	}
}

func TestOrchestratorExecutionCreateIsIdempotent(t *testing.T) {
	mux := buildRemoteFeatureMux(t)
	hostID := createRemoteHostForTests(t, mux)

	body := `{
		"goal":"idempotency",
		"idempotencyKey":"idem-key-1",
		"requiredWorkers":[{"hostId":"` + hostID + `","agentId":"zeroclaw","count":1}],
		"taskUnits":[{"id":"t1","input":"hello"}],
		"approvalScope":"infrastructure_only"
	}`
	rec1 := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions", body)
	rec2 := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions", body)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("first create status=%d body=%s", rec1.Code, rec1.Body.String())
	}
	if rec2.Code != http.StatusOK {
		t.Fatalf("second create status=%d body=%s", rec2.Code, rec2.Body.String())
	}
	p1 := decodeJSONMap(t, rec1)
	p2 := decodeJSONMap(t, rec2)
	e1, _ := p1["execution"].(map[string]interface{})
	e2, _ := p2["execution"].(map[string]interface{})
	if anyToString(e1["id"]) != anyToString(e2["id"]) {
		t.Fatalf("expected same id for idempotent create, got %q and %q", anyToString(e1["id"]), anyToString(e2["id"]))
	}
}

func TestOrchestratorWorkersReclaimEndpoint(t *testing.T) {
	configureSSHRunner(t, func(command string) remoteExecResult {
		switch {
		case strings.Contains(command, "rm -f \"$HOME/.zeroclaw/config.toml\""):
			return remoteExecResult{ExitCode: 0}
		default:
			return remoteExecResult{ExitCode: 0}
		}
	})

	mux := buildRemoteFeatureMux(t)
	hostID := createRemoteHostForTests(t, mux)

	lease := OrchestratorWorkerLease{
		ID:             "lease-reclaim-1",
		ExecutionID:    "exec-reclaim-1",
		HostID:         hostID,
		AgentID:        "zeroclaw",
		State:          OrchestratorWorkerStateReady,
		Ephemeral:      true,
		InstalledByRun: true,
		HeartbeatAt:    time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339Nano),
		LeaseExpireAt:  time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339Nano),
		CreatedAt:      nowTimestamp(),
		UpdatedAt:      nowTimestamp(),
	}
	if _, err := upsertOrchestratorWorkerLease(lease); err != nil {
		t.Fatalf("upsert lease: %v", err)
	}

	reclaimRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/workers/reclaim", `{"idleTtlSec":30}`)
	if reclaimRec.Code != http.StatusOK {
		t.Fatalf("reclaim status=%d body=%s", reclaimRec.Code, reclaimRec.Body.String())
	}
	payload := decodeJSONMap(t, reclaimRec)
	reclaimMap, _ := payload["reclaim"].(map[string]interface{})
	if int(anyToFloat(reclaimMap["reclaimed"])) != 1 {
		t.Fatalf("expected reclaimed=1 payload=%+v", payload)
	}
}

func anyToFloat(value interface{}) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return 0
	}
}
