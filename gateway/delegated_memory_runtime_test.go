package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestProvisionDelegatedChildCreatesSnapshotAndWritablePerAgentMemory(t *testing.T) {
	t.Setenv("CARRIER_INSTANCE_STORE", t.TempDir()+"/instances.json")
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", t.TempDir()+"/remote-control.json")

	parent := managedAgentInstance{
		ID:                  "openclaw-main",
		AgentID:             "openclaw",
		Type:                "openclaw",
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

	execution := OrchestratorExecution{
		ID:                 "exec-1",
		Goal:               "delegate child provisioning",
		ApprovalScope:      "infrastructure_only",
		RequiredWorkers:    []OrchestratorRequiredWorker{{HostID: orchestratorLocalHostID, AgentID: "openclaw", Count: 1}},
		TaskUnits:          []OrchestratorTaskUnit{{ID: "task-1", Input: "summarize incident"}},
		RequiredMemory:     []string{"public", "shared:team"},
		AgentLifecycleMode: orchestratorAgentLifecycleMode,
		MemoryBindingMode:  orchestratorMemoryBindingMode,
		SourceScopes:       []string{"public", "shared:team"},
	}
	var err error
	execution, err = upsertOrchestratorExecution(execution)
	if err != nil {
		t.Fatalf("upsertOrchestratorExecution: %v", err)
	}

	var createCount int
	var snapshotCount int
	var mountCount int
	var createOwner string
	var createType string
	var snapshotSourceSubject string
	var snapshotTargetInstanceID string
	var snapshotSourceScopes []string
	var mountInstanceID string
	var mountSnapshotID string

	_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
		"POST /api/v2/memory/entries/create": func(w http.ResponseWriter, r *http.Request) {
			createCount++
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode create memory body: %v", err)
			}
			createOwner = strings.TrimSpace(anyToString(body["owner"]))
			createType = strings.TrimSpace(anyToString(body["type"]))
			writeJSON(w, http.StatusOK, map[string]any{
				"entry": map[string]any{
					"id":      strings.TrimSpace(anyToString(body["id"])),
					"name":    strings.TrimSpace(anyToString(body["name"])),
					"version": strings.TrimSpace(anyToString(body["version"])),
					"type":    strings.TrimSpace(anyToString(body["type"])),
					"owner":   strings.TrimSpace(anyToString(body["owner"])),
					"state":   "created",
				},
			})
		},
		"POST /api/v2/memory/instance/snapshot": func(w http.ResponseWriter, r *http.Request) {
			snapshotCount++
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode snapshot body: %v", err)
			}
			snapshotSourceSubject = strings.TrimSpace(anyToString(body["sourceSubject"]))
			snapshotTargetInstanceID = strings.TrimSpace(anyToString(body["targetInstanceId"]))
			rawScopes, _ := body["sourceScopes"].([]any)
			snapshotSourceScopes = snapshotSourceScopes[:0]
			for _, raw := range rawScopes {
				snapshotSourceScopes = append(snapshotSourceScopes, strings.TrimSpace(anyToString(raw)))
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"snapshot": map[string]any{
					"id":               "snap-1",
					"digest":           "sha256:snapshot-1",
					"scope":            "shared:snapshot-snap-1",
					"source_subject":   snapshotSourceSubject,
					"source_scopes":    snapshotSourceScopes,
					"target_instance":  snapshotTargetInstanceID,
					"targetInstanceId": snapshotTargetInstanceID,
				},
			})
		},
		"POST /api/v2/memory/instance/snapshot/mount": func(w http.ResponseWriter, r *http.Request) {
			mountCount++
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode mount body: %v", err)
			}
			mountInstanceID = strings.TrimSpace(anyToString(body["instanceId"]))
			mountSnapshotID = strings.TrimSpace(anyToString(body["snapshotId"]))
			writeJSON(w, http.StatusOK, map[string]any{"status": "mounted"})
		},
	})

	child, err := provisionDelegatedChild(
		context.Background(),
		daemon,
		&execution,
		OrchestratorTaskUnit{ID: "task-1", Input: "summarize incident"},
		OrchestratorWorkerLease{AgentID: "openclaw"},
		1,
	)
	if err != nil {
		t.Fatalf("provisionDelegatedChild returned error: %v", err)
	}
	if strings.TrimSpace(child.ID) == "" {
		t.Fatalf("child instance id is empty: %+v", child)
	}
	if child.AgentLifecycleMode != orchestratorAgentLifecycleMode {
		t.Fatalf("AgentLifecycleMode = %q, want %q", child.AgentLifecycleMode, orchestratorAgentLifecycleMode)
	}
	if child.MemoryBindingMode != orchestratorMemoryBindingMode {
		t.Fatalf("MemoryBindingMode = %q, want %q", child.MemoryBindingMode, orchestratorMemoryBindingMode)
	}
	if child.ParentAgentID != "openclaw-main" {
		t.Fatalf("ParentAgentID = %q, want openclaw-main", child.ParentAgentID)
	}
	if child.ParentExecutionID != "exec-1" {
		t.Fatalf("ParentExecutionID = %q, want exec-1", child.ParentExecutionID)
	}
	if child.TaskID != "task-1" {
		t.Fatalf("TaskID = %q, want task-1", child.TaskID)
	}
	if child.SnapshotID != "snap-1" {
		t.Fatalf("SnapshotID = %q, want snap-1", child.SnapshotID)
	}
	if child.SnapshotDigest != "sha256:snapshot-1" {
		t.Fatalf("SnapshotDigest = %q, want sha256:snapshot-1", child.SnapshotDigest)
	}
	if strings.TrimSpace(child.PerAgentMemoryID) == "" {
		t.Fatalf("PerAgentMemoryID is empty: %+v", child)
	}
	if child.DistillTarget != "per_agent" {
		t.Fatalf("DistillTarget = %q, want per_agent", child.DistillTarget)
	}
	if child.CleanupPolicy != "delete_after_distill" {
		t.Fatalf("CleanupPolicy = %q, want delete_after_distill", child.CleanupPolicy)
	}

	if createCount != 1 {
		t.Fatalf("create memory count = %d, want 1", createCount)
	}
	if createType != "per_agent" {
		t.Fatalf("create memory type = %q, want per_agent", createType)
	}
	if createOwner != child.ID {
		t.Fatalf("create memory owner = %q, want child instance id %q", createOwner, child.ID)
	}
	if snapshotCount != 1 {
		t.Fatalf("snapshot count = %d, want 1", snapshotCount)
	}
	if snapshotSourceSubject != "openclaw-main" {
		t.Fatalf("snapshot sourceSubject = %q, want openclaw-main", snapshotSourceSubject)
	}
	if snapshotTargetInstanceID != child.ID {
		t.Fatalf("snapshot targetInstanceId = %q, want %q", snapshotTargetInstanceID, child.ID)
	}
	if strings.Join(snapshotSourceScopes, ",") != "public,shared:team" {
		t.Fatalf("snapshot sourceScopes = %v, want [public shared:team]", snapshotSourceScopes)
	}
	if mountCount != 1 {
		t.Fatalf("mount count = %d, want 1", mountCount)
	}
	if mountInstanceID != child.ID {
		t.Fatalf("mount instanceId = %q, want %q", mountInstanceID, child.ID)
	}
	if mountSnapshotID != "snap-1" {
		t.Fatalf("mount snapshotId = %q, want snap-1", mountSnapshotID)
	}

	instances, _, err := loadManagedInstances()
	if err != nil {
		t.Fatalf("loadManagedInstances: %v", err)
	}
	idx := findManagedInstanceIndex(instances, child.ID)
	if idx < 0 {
		t.Fatalf("child instance %q not persisted: %+v", child.ID, instances)
	}

	updatedExecution, found, err := getOrchestratorExecution("exec-1")
	if err != nil {
		t.Fatalf("getOrchestratorExecution: %v", err)
	}
	if !found {
		t.Fatal("expected updated orchestrator execution to be persisted")
	}
	if updatedExecution.ChildAgentID != child.ID {
		t.Fatalf("ChildAgentID = %q, want %q", updatedExecution.ChildAgentID, child.ID)
	}
	if updatedExecution.ChildPerAgentMemoryID != child.PerAgentMemoryID {
		t.Fatalf("ChildPerAgentMemoryID = %q, want %q", updatedExecution.ChildPerAgentMemoryID, child.PerAgentMemoryID)
	}
	if updatedExecution.SnapshotID != "snap-1" {
		t.Fatalf("SnapshotID = %q, want snap-1", updatedExecution.SnapshotID)
	}
	if updatedExecution.SnapshotDigest != "sha256:snapshot-1" {
		t.Fatalf("SnapshotDigest = %q, want sha256:snapshot-1", updatedExecution.SnapshotDigest)
	}
}

func TestFinalizeDelegatedChildDistillsWritebackAndCleansUp(t *testing.T) {
	t.Setenv("CARRIER_INSTANCE_STORE", t.TempDir()+"/instances.json")
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", t.TempDir()+"/remote-control.json")

	child := managedAgentInstance{
		ID:                 "child-1",
		Type:               "openclaw",
		AgentID:            "openclaw",
		RuntimeState:       "delegated",
		AgentLifecycleMode: orchestratorAgentLifecycleMode,
		MemoryBindingMode:  orchestratorMemoryBindingMode,
		PerAgentMemoryID:   "per-agent-child-1",
		ParentAgentID:      "openclaw-main",
		ParentExecutionID:  "exec-1",
		TaskID:             "task-1",
		SnapshotID:         "snap-1",
		SnapshotDigest:     "sha256:snapshot-1",
		DistillTarget:      delegatedDistillTargetPerAgent,
		CleanupPolicy:      delegatedCleanupPolicyDeleteDistill,
		CreatedAt:          nowTimestamp(),
		UpdatedAt:          nowTimestamp(),
	}
	if err := upsertManagedInstance(child); err != nil {
		t.Fatalf("upsertManagedInstance(child): %v", err)
	}

	execution := OrchestratorExecution{
		ID:                    "exec-1",
		Goal:                  "delegate child finalize",
		ApprovalScope:         "infrastructure_only",
		RequiredWorkers:       []OrchestratorRequiredWorker{{HostID: orchestratorLocalHostID, AgentID: "openclaw", Count: 1}},
		TaskUnits:             []OrchestratorTaskUnit{{ID: "task-1", Input: "summarize incident"}},
		RequiredMemory:        []string{"public", "shared:team"},
		AgentLifecycleMode:    orchestratorAgentLifecycleMode,
		MemoryBindingMode:     orchestratorMemoryBindingMode,
		SourceScopes:          []string{"public", "shared:team"},
		ChildAgentID:          child.ID,
		ChildPerAgentMemoryID: child.PerAgentMemoryID,
		SnapshotID:            child.SnapshotID,
		SnapshotDigest:        child.SnapshotDigest,
	}
	var err error
	execution, err = upsertOrchestratorExecution(execution)
	if err != nil {
		t.Fatalf("upsertOrchestratorExecution: %v", err)
	}

	order := make([]string, 0, 8)
	var upsertSubject string
	var upsertScope string
	var upsertProvenance string
	var upsertSummary string
	var purgeInstanceID string
	var purgeScope string
	var deleteSnapshotID string
	var archiveMemoryID string

	_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
		"POST /api/v2/memory/instance/distill": func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "distill")
			writeJSON(w, http.StatusOK, map[string]any{
				"result": map[string]any{
					"runId":      "distill-1",
					"instanceId": child.ID,
					"scope":      "agent:" + child.ID,
					"status":     "completed",
					"outputIds":  []string{"distilled-1", "distilled-2"},
				},
			})
		},
		"POST /api/v2/memory/get": func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "get")
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode get body: %v", err)
			}
			recordID := strings.TrimSpace(anyToString(body["id"]))
			summary := "first distilled summary"
			if recordID == "distilled-2" {
				summary = "second distilled summary"
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"record": map[string]any{
					"id":             recordID,
					"scope":          "agent:" + child.ID,
					"type":           "note",
					"contentSummary": summary,
					"provenance":     "distill:distill-1",
				},
			})
		},
		"POST /api/v2/memory/records/upsert": func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "upsert")
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode upsert body: %v", err)
			}
			upsertSubject = strings.TrimSpace(anyToString(body["subject"]))
			upsertScope = strings.TrimSpace(anyToString(body["scope"]))
			upsertProvenance = strings.TrimSpace(anyToString(body["provenance"]))
			upsertSummary = strings.TrimSpace(anyToString(body["contentSummary"]))
			writeJSON(w, http.StatusOK, map[string]any{
				"record": map[string]any{
					"id":             "parent-rec-1",
					"scope":          upsertScope,
					"type":           "note",
					"contentSummary": upsertSummary,
					"provenance":     upsertProvenance,
				},
			})
		},
		"POST /api/v2/memory/instance/purge": func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "purge")
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode purge body: %v", err)
			}
			purgeInstanceID = strings.TrimSpace(anyToString(body["instanceId"]))
			purgeScope = strings.TrimSpace(anyToString(body["scope"]))
			writeJSON(w, http.StatusOK, map[string]any{"deleted": 2})
		},
		"POST /api/v2/memory/instance/snapshot/delete": func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "delete_snapshot")
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode delete snapshot body: %v", err)
			}
			deleteSnapshotID = strings.TrimSpace(anyToString(body["snapshotId"]))
			writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
		},
		"POST /api/v2/memory/entries/archive": func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "archive_entry")
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode archive entry body: %v", err)
			}
			archiveMemoryID = strings.TrimSpace(anyToString(body["id"]))
			writeJSON(w, http.StatusOK, map[string]any{"status": "archived"})
		},
	})

	result, finalizeErr := finalizeDelegatedChild(
		context.Background(),
		daemon,
		&execution,
		OrchestratorTaskUnit{ID: "task-1", Input: "summarize incident"},
		child,
		OrchestratorTaskResult{
			TaskID:   "task-1",
			Status:   OrchestratorTaskStatusCompleted,
			AgentID:  "openclaw",
			Output:   "delegated output",
			Attempts: 1,
		},
	)
	if finalizeErr != nil {
		t.Fatalf("finalizeDelegatedChild returned error: %v", finalizeErr)
	}

	if strings.Join(order, ",") != "distill,get,get,upsert,purge,delete_snapshot,archive_entry" {
		t.Fatalf("finalize order = %v, want [distill get get upsert purge delete_snapshot archive_entry]", order)
	}
	if result.DelegatedMemory == nil {
		t.Fatalf("expected delegated memory result, got %+v", result)
	}
	if result.DelegatedMemory.ChildAgentID != child.ID {
		t.Fatalf("ChildAgentID = %q, want %q", result.DelegatedMemory.ChildAgentID, child.ID)
	}
	if result.DelegatedMemory.DistillRunID != "distill-1" {
		t.Fatalf("DistillRunID = %q, want distill-1", result.DelegatedMemory.DistillRunID)
	}
	if result.DelegatedMemory.CleanupStatus != "completed" {
		t.Fatalf("CleanupStatus = %q, want completed", result.DelegatedMemory.CleanupStatus)
	}
	if strings.Join(result.DelegatedMemory.ParentRecordIDs, ",") != "parent-rec-1" {
		t.Fatalf("ParentRecordIDs = %v, want [parent-rec-1]", result.DelegatedMemory.ParentRecordIDs)
	}
	if upsertSubject != "openclaw-main" {
		t.Fatalf("write-back subject = %q, want openclaw-main", upsertSubject)
	}
	if upsertScope != "agent:openclaw-main" {
		t.Fatalf("write-back scope = %q, want agent:openclaw-main", upsertScope)
	}
	if !strings.Contains(upsertProvenance, child.ID) || !strings.Contains(upsertProvenance, "distill-1") {
		t.Fatalf("write-back provenance = %q, want child id and distill run", upsertProvenance)
	}
	if !strings.Contains(upsertSummary, "first distilled summary") || !strings.Contains(upsertSummary, "second distilled summary") {
		t.Fatalf("write-back summary = %q, want distilled summaries", upsertSummary)
	}
	if purgeInstanceID != child.ID || purgeScope != "agent:"+child.ID {
		t.Fatalf("purge target = %q/%q, want %q/%q", purgeInstanceID, purgeScope, child.ID, "agent:"+child.ID)
	}
	if deleteSnapshotID != child.SnapshotID {
		t.Fatalf("delete snapshot id = %q, want %q", deleteSnapshotID, child.SnapshotID)
	}
	if archiveMemoryID != child.PerAgentMemoryID {
		t.Fatalf("archive memory id = %q, want %q", archiveMemoryID, child.PerAgentMemoryID)
	}

	instances, _, err := loadManagedInstances()
	if err != nil {
		t.Fatalf("loadManagedInstances: %v", err)
	}
	if findManagedInstanceIndex(instances, child.ID) >= 0 {
		t.Fatalf("expected child instance cleanup, instances=%+v", instances)
	}

	updatedExecution, found, err := getOrchestratorExecution("exec-1")
	if err != nil {
		t.Fatalf("getOrchestratorExecution: %v", err)
	}
	if !found {
		t.Fatal("expected updated execution after finalize")
	}
	if updatedExecution.DistillRunID != "distill-1" {
		t.Fatalf("execution DistillRunID = %q, want distill-1", updatedExecution.DistillRunID)
	}
	if updatedExecution.CleanupStatus != "completed" {
		t.Fatalf("execution CleanupStatus = %q, want completed", updatedExecution.CleanupStatus)
	}
}
