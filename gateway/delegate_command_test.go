package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHandleCommand_Delegate_Usage(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, nil)
	defer srv.Close()

	token := pairAndGetSession(sessions, "telegram", "chat-1")
	cmd := &GatewayCommand{
		Provider:     "telegram",
		ChatID:       "chat-1",
		RequestID:    "req-usage",
		Name:         CmdDelegate,
		Args:         []string{},
		SessionToken: token,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.Result != "error" {
		t.Fatalf("expected error, got %s: %s", resp.Result, resp.Message)
	}
	if resp.ErrorCode != "E_USAGE" {
		t.Fatalf("expected E_USAGE, got %s", resp.ErrorCode)
	}
}

func TestDelegateExecutionSeedsDelegatedMemoryState(t *testing.T) {
	execution, err := buildDelegateOrchestratorExecution(context.Background(), nil, " triage checkout incident ", []BaseAgentDecomposeTask{
		{ID: "task-a", Input: "collect checkout diagnostics", AgentID: "picoclaw"},
	}, "req-delegate-memory")
	if err != nil {
		t.Fatalf("buildDelegateOrchestratorExecution: %v", err)
	}
	if execution.AgentLifecycleMode != "delegated" {
		t.Fatalf("agentLifecycleMode = %q, want delegated", execution.AgentLifecycleMode)
	}
	if execution.MemoryBindingMode != "snapshot" {
		t.Fatalf("memoryBindingMode = %q, want snapshot", execution.MemoryBindingMode)
	}
	if len(execution.SourceScopes) != 0 {
		t.Fatalf("sourceScopes = %v, want empty", execution.SourceScopes)
	}
	if execution.SnapshotID != "" || execution.SnapshotDigest != "" {
		t.Fatalf("expected empty snapshot state, got snapshotId=%q snapshotDigest=%q", execution.SnapshotID, execution.SnapshotDigest)
	}
	if execution.ChildAgentID != "" || execution.ChildPerAgentMemoryID != "" {
		t.Fatalf("expected empty child state, got childAgentId=%q childPerAgentMemoryId=%q", execution.ChildAgentID, execution.ChildPerAgentMemoryID)
	}
	if execution.DistillRunID != "" || execution.CleanupStatus != "" {
		t.Fatalf("expected empty finalize state, got distillRunId=%q cleanupStatus=%q", execution.DistillRunID, execution.CleanupStatus)
	}
}

func TestHandleCommand_Delegate_SubmitAndStatus(t *testing.T) {
	t.Setenv("CARRIER_DELEGATE_STORE", t.TempDir()+"/delegate-store.json")
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", t.TempDir()+"/remote-control.json")

	origStart := delegateStartExecutionFn
	origRemoteDiscover := delegateDiscoverRemoteWorkersFn
	origLocalDaemonClient := orchestratorLocalDaemonClientFn
	delegateStartExecutionFn = func(executionID string, daemon *DaemonClient) {
		runDelegateExecution(executionID, daemon)
	}
	delegateDiscoverRemoteWorkersFn = func(ctx context.Context, requestID string) ([]delegateWorker, error) {
		return []delegateWorker{}, nil
	}
	t.Cleanup(func() {
		delegateStartExecutionFn = origStart
		delegateDiscoverRemoteWorkersFn = origRemoteDiscover
		orchestratorLocalDaemonClientFn = origLocalDaemonClient
	})

	srv, dc, sessions, downloads, onboard := setupTestEnv(t, map[string]http.HandlerFunc{
		"POST /api/v1/base-agent/decompose": func(w http.ResponseWriter, r *http.Request) {
			var req map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if strings.TrimSpace(anyToString(req["goal"])) == "" {
				http.Error(w, "missing goal", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"tasks": []map[string]interface{}{
					{"id": "task-a", "input": "summarize latest logs", "agentId": "picoclaw"},
				},
			})
		},
		"GET /api/v1/agents": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"agents": []map[string]interface{}{
					{
						"id":           "picoclaw",
						"installState": "installed",
						"runtimeState": "running",
					},
				},
			})
		},
		"POST /api/v1/agents/picoclaw/chat": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"agentId":   "picoclaw",
				"sessionId": "sess-1",
				"message":   "task completed",
			})
		},
		"POST /api/v2/memory/entries/create": func(w http.ResponseWriter, r *http.Request) {
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"entry": map[string]interface{}{
					"id":    strings.TrimSpace(anyToString(body["id"])),
					"type":  "per_agent",
					"owner": strings.TrimSpace(anyToString(body["owner"])),
					"state": "created",
				},
			})
		},
		"POST /api/v2/memory/instance/distill": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"result": map[string]interface{}{
					"runId":     "distill-delegate-1",
					"status":    "completed",
					"outputIds": []string{},
				},
			})
		},
		"POST /api/v2/memory/records/upsert": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"record": map[string]interface{}{
					"id":    "parent-delegate-1",
					"scope": "agent:picoclaw",
					"type":  "note",
				},
			})
		},
		"POST /api/v2/memory/instance/purge": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"deleted": 1})
		},
		"POST /api/v2/memory/entries/archive": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "archived"})
		},
	})
	defer srv.Close()
	orchestratorLocalDaemonClientFn = func() *DaemonClient { return dc }

	token := pairAndGetSession(sessions, "telegram", "chat-2")
	submit := &GatewayCommand{
		Provider:     "telegram",
		ChatID:       "chat-2",
		RequestID:    "req-submit",
		Name:         CmdDelegate,
		Args:         []string{"analyze", "recent", "errors"},
		SessionToken: token,
	}
	submitResp := HandleCommand(context.Background(), submit, dc, sessions, downloads, nil, onboard)
	if submitResp.Result != "ok" {
		t.Fatalf("expected submit ok, got %s: %s", submitResp.Result, submitResp.Message)
	}

	executionID := parseDelegateExecutionID(submitResp.Message)
	if executionID == "" {
		t.Fatalf("expected execution id in message, got %q", submitResp.Message)
	}

	orchestrated := waitForDelegateOrchestratorExecution(t, executionID, 5*time.Second)
	if orchestrated.Status != OrchestratorExecutionStatusCompleted {
		t.Fatalf("expected orchestrator execution completed, got %+v", orchestrated)
	}
	if len(orchestrated.RequiredWorkers) != 1 {
		t.Fatalf("expected one required worker, got %+v", orchestrated.RequiredWorkers)
	}
	if orchestrated.RequiredWorkers[0].HostID != orchestratorLocalHostID || orchestrated.RequiredWorkers[0].AgentID != "picoclaw" {
		t.Fatalf("expected local picoclaw worker requirement, got %+v", orchestrated.RequiredWorkers[0])
	}
	if len(orchestrated.TaskUnits) != 1 || orchestrated.TaskUnits[0].AgentID != "picoclaw" {
		t.Fatalf("expected orchestrator task unit to target picoclaw, got %+v", orchestrated.TaskUnits)
	}
	if _, found, err := getDelegateExecution(executionID); err != nil {
		t.Fatalf("get delegate execution: %v", err)
	} else if found {
		t.Fatalf("expected /delegate compatibility flow to avoid delegate store writes, but found execution %s", executionID)
	}

	status := &GatewayCommand{
		Provider:     "telegram",
		ChatID:       "chat-2",
		RequestID:    "req-status",
		Name:         CmdDelegate,
		Args:         []string{"status", executionID},
		SessionToken: token,
	}
	statusResp := HandleCommand(context.Background(), status, dc, sessions, downloads, nil, onboard)
	if statusResp.Result != "ok" {
		t.Fatalf("expected status ok, got %s: %s", statusResp.Result, statusResp.Message)
	}
	if !strings.Contains(statusResp.Message, "status: completed") {
		t.Fatalf("expected completed status, got %q", statusResp.Message)
	}
	if !strings.Contains(statusResp.Message, "task-a") {
		t.Fatalf("expected task details in status response, got %q", statusResp.Message)
	}
	if !strings.Contains(statusResp.Message, "memory: binding=snapshot") {
		t.Fatalf("expected delegated memory summary in status response, got %q", statusResp.Message)
	}
	if !strings.Contains(statusResp.Message, "distill=distill-delegate-1") {
		t.Fatalf("expected distill status in status response, got %q", statusResp.Message)
	}
	if !strings.Contains(statusResp.Message, "cleanup=completed") {
		t.Fatalf("expected cleanup status in status response, got %q", statusResp.Message)
	}
}

func TestRenderDelegateOrchestratorExecutionStatusIncludesTaskMemoryLifecycle(t *testing.T) {
	rendered := renderDelegateOrchestratorExecutionStatus(OrchestratorExecution{
		ID:                "exec-1",
		Status:            OrchestratorExecutionStatusCompleted,
		Goal:              "summarize incidents",
		MemoryBindingMode: "snapshot",
		SourceScopes:      []string{"shared:team", "public:runbook"},
		SnapshotID:        "snap-1",
		SnapshotDigest:    "mem-abc123",
		ChildAgentID:      "child-1",
		DistillRunID:      "distill-1",
		CleanupStatus:     "cleanup_pending",
		DistillOutputs:    []string{"record-1"},
		TaskUnits: []OrchestratorTaskUnit{
			{ID: "task-a", Input: "collect evidence"},
		},
		Results: []OrchestratorTaskResult{
			{
				TaskID:    "task-a",
				Status:    OrchestratorTaskStatusCompleted,
				AgentID:   "picoclaw",
				LatencyMs: 42,
				Output:    "captured summary",
				DelegatedMemory: &OrchestratorDelegatedTaskMemoryState{
					ChildAgentID:    "child-1",
					SnapshotID:      "snap-1",
					SnapshotDigest:  "mem-abc123",
					DistillRunID:    "distill-1",
					CleanupStatus:   "completed",
					ParentRecordIDs: []string{"parent-rec-1"},
				},
			},
		},
	})

	if !strings.Contains(rendered, "memory: binding=snapshot sources=2 snapshot=snap-1@mem-abc123 child=child-1 distill=distill-1 cleanup=cleanup_pending outputs=1") {
		t.Fatalf("expected execution-level memory summary, got %q", rendered)
	}
	if !strings.Contains(rendered, "memory: child=child-1 snapshot=snap-1@mem-abc123 distill=distill-1 cleanup=completed writeback=1") {
		t.Fatalf("expected task-level memory summary, got %q", rendered)
	}
}

func TestHandleCommand_Delegate_StatusNotFound(t *testing.T) {
	t.Setenv("CARRIER_DELEGATE_STORE", t.TempDir()+"/delegate-store.json")

	srv, dc, sessions, downloads, onboard := setupTestEnv(t, nil)
	defer srv.Close()

	token := pairAndGetSession(sessions, "telegram", "chat-3")
	cmd := &GatewayCommand{
		Provider:     "telegram",
		ChatID:       "chat-3",
		RequestID:    "req-status-missing",
		Name:         CmdDelegate,
		Args:         []string{"status", "missing-execution"},
		SessionToken: token,
	}
	resp := HandleCommand(context.Background(), cmd, dc, sessions, downloads, nil, onboard)
	if resp.Result != "error" {
		t.Fatalf("expected error, got %s: %s", resp.Result, resp.Message)
	}
	if resp.ErrorCode != "E_NOT_FOUND" {
		t.Fatalf("expected E_NOT_FOUND, got %s", resp.ErrorCode)
	}
}

func TestHandleCommand_Delegate_RespectsTaskAgentPreference(t *testing.T) {
	t.Setenv("CARRIER_DELEGATE_STORE", t.TempDir()+"/delegate-store.json")
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", t.TempDir()+"/remote-control.json")

	origStart := delegateStartExecutionFn
	origRemoteDiscover := delegateDiscoverRemoteWorkersFn
	origLocalDaemonClient := orchestratorLocalDaemonClientFn
	delegateStartExecutionFn = func(executionID string, daemon *DaemonClient) {
		runDelegateExecution(executionID, daemon)
	}
	delegateDiscoverRemoteWorkersFn = func(ctx context.Context, requestID string) ([]delegateWorker, error) {
		return []delegateWorker{}, nil
	}
	t.Cleanup(func() {
		delegateStartExecutionFn = origStart
		delegateDiscoverRemoteWorkersFn = origRemoteDiscover
		orchestratorLocalDaemonClientFn = origLocalDaemonClient
	})

	var picoclawCalls int32
	var zeroclawCalls int32

	srv, dc, sessions, downloads, onboard := setupTestEnv(t, map[string]http.HandlerFunc{
		"POST /api/v1/base-agent/decompose": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"tasks": []map[string]interface{}{
					{"id": "task-a", "input": "collect diagnostics", "agentId": "zeroclaw"},
					{"id": "task-b", "input": "summarize diagnostics", "agentId": "zeroclaw"},
				},
			})
		},
		"GET /api/v1/agents": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"agents": []map[string]interface{}{
					{
						"id":           "picoclaw",
						"installState": "installed",
						"runtimeState": "running",
					},
					{
						"id":           "zeroclaw",
						"installState": "installed",
						"runtimeState": "running",
					},
				},
			})
		},
		"POST /api/v1/agents/picoclaw/chat": func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&picoclawCalls, 1)
			http.Error(w, "picoclaw should not run zeroclaw-only tasks", http.StatusInternalServerError)
		},
		"POST /api/v1/agents/zeroclaw/chat": func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&zeroclawCalls, 1)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"agentId":   "zeroclaw",
				"sessionId": "sess-z",
				"message":   "zeroclaw task completed",
			})
		},
		"POST /api/v2/memory/entries/create": func(w http.ResponseWriter, r *http.Request) {
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"entry": map[string]interface{}{
					"id":    strings.TrimSpace(anyToString(body["id"])),
					"type":  "per_agent",
					"owner": strings.TrimSpace(anyToString(body["owner"])),
					"state": "created",
				},
			})
		},
		"POST /api/v2/memory/instance/distill": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"result": map[string]interface{}{
					"runId":     "distill-delegate-z",
					"status":    "completed",
					"outputIds": []string{},
				},
			})
		},
		"POST /api/v2/memory/records/upsert": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"record": map[string]interface{}{
					"id":    "parent-delegate-z",
					"scope": "agent:zeroclaw",
					"type":  "note",
				},
			})
		},
		"POST /api/v2/memory/instance/purge": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"deleted": 1})
		},
		"POST /api/v2/memory/entries/archive": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "archived"})
		},
	})
	defer srv.Close()
	orchestratorLocalDaemonClientFn = func() *DaemonClient { return dc }

	token := pairAndGetSession(sessions, "telegram", "chat-pref")
	submit := &GatewayCommand{
		Provider:     "telegram",
		ChatID:       "chat-pref",
		RequestID:    "req-pref-submit",
		Name:         CmdDelegate,
		Args:         []string{"triage", "incident"},
		SessionToken: token,
	}
	submitResp := HandleCommand(context.Background(), submit, dc, sessions, downloads, nil, onboard)
	if submitResp.Result != "ok" {
		t.Fatalf("expected submit ok, got %s: %s", submitResp.Result, submitResp.Message)
	}

	executionID := parseDelegateExecutionID(submitResp.Message)
	if executionID == "" {
		t.Fatalf("expected execution id in message, got %q", submitResp.Message)
	}
	orchestrated := waitForDelegateOrchestratorExecution(t, executionID, 5*time.Second)
	if orchestrated.Status != OrchestratorExecutionStatusCompleted {
		t.Fatalf("expected orchestrator execution completed, got %+v", orchestrated)
	}

	status := &GatewayCommand{
		Provider:     "telegram",
		ChatID:       "chat-pref",
		RequestID:    "req-pref-status",
		Name:         CmdDelegate,
		Args:         []string{"status", executionID},
		SessionToken: token,
	}
	statusResp := HandleCommand(context.Background(), status, dc, sessions, downloads, nil, onboard)
	if statusResp.Result != "ok" {
		t.Fatalf("expected status ok, got %s: %s", statusResp.Result, statusResp.Message)
	}
	if !strings.Contains(statusResp.Message, "status: completed") {
		t.Fatalf("expected completed status, got %q", statusResp.Message)
	}
	if !strings.Contains(statusResp.Message, "task-a [completed] target=zeroclaw") {
		t.Fatalf("expected task-a to target zeroclaw, got %q", statusResp.Message)
	}
	if !strings.Contains(statusResp.Message, "task-b [completed] target=zeroclaw") {
		t.Fatalf("expected task-b to target zeroclaw, got %q", statusResp.Message)
	}
	if atomic.LoadInt32(&picoclawCalls) != 0 {
		t.Fatalf("expected picoclaw not to be called, got %d calls", atomic.LoadInt32(&picoclawCalls))
	}
	if atomic.LoadInt32(&zeroclawCalls) != 2 {
		t.Fatalf("expected zeroclaw to be called twice, got %d calls", atomic.LoadInt32(&zeroclawCalls))
	}
}

func parseDelegateExecutionID(message string) string {
	re := regexp.MustCompile(`execution accepted:\s+([a-zA-Z0-9-]+)`)
	matches := re.FindStringSubmatch(strings.TrimSpace(message))
	if len(matches) != 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func waitForDelegateOrchestratorExecution(t *testing.T, executionID string, timeout time.Duration) OrchestratorExecution {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		execution, found, err := getOrchestratorExecution(executionID)
		if err == nil && found {
			switch execution.Status {
			case OrchestratorExecutionStatusCompleted, OrchestratorExecutionStatusFailed, OrchestratorExecutionStatusCancelled, OrchestratorExecutionStatusDeclined:
				return execution
			}
		}
		time.Sleep(25 * time.Millisecond)
	}

	execution, found, err := getOrchestratorExecution(executionID)
	if err != nil {
		t.Fatalf("timed out waiting for orchestrator execution %s: %v", executionID, err)
	}
	if !found {
		t.Fatalf("timed out waiting for orchestrator execution %s to appear", executionID)
	}
	t.Fatalf("timed out waiting for orchestrator execution %s to finish; last status=%s", executionID, execution.Status)
	return OrchestratorExecution{}
}

func TestUpsertDelegateExecution_TrimsOlderEntries(t *testing.T) {
	t.Setenv("CARRIER_DELEGATE_STORE", t.TempDir()+"/delegate-store.json")
	t.Setenv("CARRIER_DELEGATE_STORE_MAX_EXECUTIONS", "2")

	makeExecution := func(id string, updatedAt time.Time) delegateExecution {
		ts := updatedAt.UTC().Format(time.RFC3339Nano)
		return delegateExecution{
			ID:        id,
			Goal:      "test",
			Status:    delegateExecutionStatusQueued,
			TaskUnits: []BaseAgentDecomposeTask{{ID: "task-1", Input: "noop"}},
			CreatedAt: ts,
			UpdatedAt: ts,
		}
	}

	old := makeExecution("exec-old", time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC))
	mid := makeExecution("exec-mid", time.Date(2026, 3, 8, 0, 1, 0, 0, time.UTC))
	newest := makeExecution("exec-new", time.Date(2026, 3, 8, 0, 2, 0, 0, time.UTC))

	if _, err := upsertDelegateExecution(old); err != nil {
		t.Fatalf("upsert old execution: %v", err)
	}
	if _, err := upsertDelegateExecution(mid); err != nil {
		t.Fatalf("upsert mid execution: %v", err)
	}
	if _, err := upsertDelegateExecution(newest); err != nil {
		t.Fatalf("upsert newest execution: %v", err)
	}

	executions, err := listDelegateExecutions(0)
	if err != nil {
		t.Fatalf("list executions: %v", err)
	}
	if len(executions) != 2 {
		t.Fatalf("expected 2 executions after trim, got %d", len(executions))
	}
	if executions[0].ID != "exec-new" || executions[1].ID != "exec-mid" {
		t.Fatalf("unexpected retained execution order: %#v", []string{executions[0].ID, executions[1].ID})
	}

	if _, found, err := getDelegateExecution("exec-old"); err != nil {
		t.Fatalf("get old execution: %v", err)
	} else if found {
		t.Fatalf("expected old execution to be trimmed")
	}
}
