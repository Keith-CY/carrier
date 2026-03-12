package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDefaultRuntimeState_Branches(t *testing.T) {
	if got := defaultRuntimeState("running", "installed"); got != "running" {
		t.Fatalf("defaultRuntimeState(runtime, install) = %q, want running", got)
	}
	if got := defaultRuntimeState("", "installed"); got != "stopped" {
		t.Fatalf("defaultRuntimeState('', installed) = %q, want stopped", got)
	}
	if got := defaultRuntimeState("", "not_installed"); got != "unknown" {
		t.Fatalf("defaultRuntimeState('', not_installed) = %q, want unknown", got)
	}
}

func TestBackfillManagedInstancesFromDaemon_Branches(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/v1/instances", nil)

	t.Run("nil daemon returns unchanged", func(t *testing.T) {
		initial := []managedAgentInstance{{ID: "x", AgentID: "x"}}
		got, changed, err := backfillManagedInstancesFromDaemon(req, nil, initial, "req-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if changed {
			t.Fatal("expected changed=false")
		}
		if len(got) != 1 || got[0].ID != "x" {
			t.Fatalf("unexpected instances: %+v", got)
		}
	})

	t.Run("list agents error", func(t *testing.T) {
		_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"GET /api/v1/agents": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error": map[string]string{"code": "E_COMMAND_FAILED", "message": "boom"},
				})
			},
		})
		_, _, err := backfillManagedInstancesFromDaemon(req, daemon, nil, "req-2")
		if err == nil {
			t.Fatal("expected list agents error")
		}
	})

	t.Run("skip non-trackable agents", func(t *testing.T) {
		_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"GET /api/v1/agents": func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"agents": []map[string]interface{}{
						{"id": "x1", "installState": "not_installed", "runtimeState": "stopped"},
					},
				})
			},
		})
		got, changed, err := backfillManagedInstancesFromDaemon(req, daemon, nil, "req-3")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if changed || len(got) != 0 {
			t.Fatalf("expected no backfilled instances, got changed=%v instances=%+v", changed, got)
		}
	})

	t.Run("create new backfilled instance", func(t *testing.T) {
		_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"GET /api/v1/agents": func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"agents": []map[string]interface{}{
						{"id": "picoclaw", "installState": "installed", "runtimeState": "stopped"},
					},
				})
			},
		})
		got, changed, err := backfillManagedInstancesFromDaemon(req, daemon, nil, "req-4")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !changed || len(got) != 1 {
			t.Fatalf("expected one changed instance, got changed=%v instances=%+v", changed, got)
		}
		if got[0].ID != "picoclaw-default" || got[0].RuntimeState != "stopped" {
			t.Fatalf("unexpected backfilled instance: %+v", got[0])
		}
	})

	t.Run("update existing by agent id", func(t *testing.T) {
		_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"GET /api/v1/agents": func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"agents": []map[string]interface{}{
						{"id": "picoclaw", "installState": "installed", "runtimeState": "running"},
					},
				})
			},
		})
		initial := []managedAgentInstance{{
			ID:           "picoclaw-default",
			AgentID:      "picoclaw",
			Type:         "",
			GatewayURL:   "",
			RuntimeState: "unknown",
		}}
		got, changed, err := backfillManagedInstancesFromDaemon(req, daemon, initial, "req-5")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !changed {
			t.Fatal("expected changed=true")
		}
		if got[0].Type != "picoclaw" || got[0].RuntimeState != "running" || strings.TrimSpace(got[0].GatewayURL) == "" {
			t.Fatalf("unexpected updated instance: %+v", got[0])
		}
	})

	t.Run("default id conflict triggers generated id", func(t *testing.T) {
		_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"GET /api/v1/agents": func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"agents": []map[string]interface{}{
						{"id": "picoclaw", "installState": "installed", "runtimeState": "stopped"},
					},
				})
			},
		})
		initial := []managedAgentInstance{{
			ID:      "picoclaw-default",
			AgentID: "other-agent",
			Type:    "other-agent",
		}}
		got, changed, err := backfillManagedInstancesFromDaemon(req, daemon, initial, "req-6")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !changed || len(got) != 2 {
			t.Fatalf("expected one appended generated instance, got changed=%v instances=%+v", changed, got)
		}
		if got[1].ID == "picoclaw-default" || !strings.HasPrefix(got[1].ID, "picoclaw-") {
			t.Fatalf("expected generated picoclaw-* id, got %q", got[1].ID)
		}
	})
}

func TestUpdatePairingStateFromLogs_Branches(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/v1/instances", nil)

	if changed := updatePairingStateFromLogs(req, nil, nil, "req-0"); changed {
		t.Fatal("expected changed=false for nil daemon/instances")
	}

	t.Run("marks paired from logs", func(t *testing.T) {
		_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"GET /api/v1/agents/picoclaw/logs": func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"lines": []string{
						"PAIR_CODE: pair-abcdef0123456789abcdef0123456789",
						"paired telegram:418258935",
					},
					"truncated": false,
				})
			},
		})
		instances := []managedAgentInstance{{
			ID:           "picoclaw-1",
			Type:         "picoclaw",
			AgentID:      "picoclaw",
			Channel:      "telegram",
			PairRequired: true,
			RuntimeState: "pending_pair",
			PairCode:     "",
		}}
		changed := updatePairingStateFromLogs(req, daemon, instances, "req-paired")
		if !changed {
			t.Fatal("expected changed=true")
		}
		if instances[0].PairCode == "" || instances[0].PairedChatID != "418258935" || instances[0].PairRequired || instances[0].RuntimeState != "running" {
			t.Fatalf("unexpected paired state: %+v", instances[0])
		}
	})

	t.Run("no paired marker sets pending_pair", func(t *testing.T) {
		_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"GET /api/v1/agents/picoclaw/logs": func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"lines":     []string{"PAIR_CODE: pair-abcdef0123456789abcdef0123456789"},
					"truncated": false,
				})
			},
		})
		instances := []managedAgentInstance{{
			ID:           "picoclaw-2",
			Type:         "picoclaw",
			AgentID:      "picoclaw",
			Channel:      "telegram",
			PairRequired: false,
			RuntimeState: "running",
		}}
		changed := updatePairingStateFromLogs(req, daemon, instances, "req-unpaired")
		if !changed {
			t.Fatal("expected changed=true")
		}
		if !instances[0].PairRequired || instances[0].RuntimeState != "pending_pair" {
			t.Fatalf("unexpected pending-pair state: %+v", instances[0])
		}
	})
}

func TestHandleWebUIInstances_Branches(t *testing.T) {
	t.Run("method not allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/instances", nil)
		handleWebUIInstances(rec, req, "req-method", nil)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
	})

	t.Run("load instances failure", func(t *testing.T) {
		tmp := t.TempDir()
		badPath := filepath.Join(tmp, "instances.json")
		if err := os.MkdirAll(badPath, 0o700); err != nil {
			t.Fatalf("mkdir bad path: %v", err)
		}
		t.Setenv("CARRIER_INSTANCE_STORE", badPath)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/instances", nil)
		handleWebUIInstances(rec, req, "req-load", nil)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"errorCode":"E_INTERNAL"`) {
			t.Fatalf("expected E_INTERNAL, got %s", rec.Body.String())
		}
	})
}

func TestHandleWebUIInstances_SyncAndPersistBranches(t *testing.T) {
	t.Run("syncs runtime, pairing, and backfill", func(t *testing.T) {
		tmp := t.TempDir()
		storePath := filepath.Join(tmp, "instances.json")
		t.Setenv("CARRIER_INSTANCE_STORE", storePath)

		old := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339Nano)
		if err := saveManagedInstances(storePath, []managedAgentInstance{
			{
				ID:           "inst-1",
				Type:         "picoclaw",
				AgentID:      "picoclaw",
				Channel:      "telegram",
				PairRequired: true,
				RuntimeState: "pending_pair",
				CreatedAt:    old,
				UpdatedAt:    old,
			},
			{
				ID:           "inst-2",
				Type:         "alpha",
				AgentID:      "alpha",
				RuntimeState: "stopped",
				CreatedAt:    old,
				UpdatedAt:    old,
			},
		}); err != nil {
			t.Fatalf("saveManagedInstances: %v", err)
		}

		_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"GET /api/v1/agents/picoclaw/status": func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"statuses": []map[string]interface{}{{"id": "picoclaw", "runtimeState": "running"}},
				})
			},
			"GET /api/v1/agents/alpha/status": func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"statuses": []map[string]interface{}{{"id": "alpha", "runtimeState": "stopped"}},
				})
			},
			"GET /api/v1/agents": func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"agents": []map[string]interface{}{
						{"id": "picoclaw", "installState": "installed", "runtimeState": "running"},
						{"id": "alpha", "installState": "installed", "runtimeState": "stopped"},
						{"id": "gamma", "installState": "installed", "runtimeState": "stopped"},
					},
				})
			},
			"GET /api/v1/agents/picoclaw/logs": func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"lines": []string{
						"PAIR_CODE: pair-abcdef0123456789abcdef0123456789",
						"paired telegram:418258935",
					},
					"truncated": false,
				})
			},
		})

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/v1/instances", nil)
		handleWebUIInstances(rec, req, "req-sync", daemon)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var payload struct {
			Result    string                 `json:"result"`
			Instances []managedAgentInstance `json:"instances"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode payload: %v; body=%s", err, rec.Body.String())
		}
		if payload.Result != "ok" {
			t.Fatalf("expected result=ok, got %+v", payload)
		}
		if len(payload.Instances) != 3 {
			t.Fatalf("expected 3 instances, got %+v", payload.Instances)
		}

		var picoclawInst *managedAgentInstance
		var gammaInst *managedAgentInstance
		for i := range payload.Instances {
			inst := &payload.Instances[i]
			if inst.AgentID == "picoclaw" {
				picoclawInst = inst
			}
			if inst.AgentID == "gamma" {
				gammaInst = inst
			}
		}
		if picoclawInst == nil {
			t.Fatalf("expected picoclaw instance, got %+v", payload.Instances)
		}
		if picoclawInst.RuntimeState != "running" || picoclawInst.PairRequired || strings.TrimSpace(picoclawInst.PairedChatID) == "" {
			t.Fatalf("unexpected picoclaw synced state: %+v", *picoclawInst)
		}
		if gammaInst == nil || gammaInst.ID == "" || gammaInst.RuntimeState != "stopped" {
			t.Fatalf("expected backfilled gamma instance, got %+v", payload.Instances)
		}

		persisted, _, err := loadManagedInstances()
		if err != nil {
			t.Fatalf("loadManagedInstances: %v", err)
		}
		if len(persisted) != 3 {
			t.Fatalf("expected persisted instances=3, got %+v", persisted)
		}
	})

	t.Run("backfill errors are tolerated", func(t *testing.T) {
		tmp := t.TempDir()
		storePath := filepath.Join(tmp, "instances.json")
		t.Setenv("CARRIER_INSTANCE_STORE", storePath)

		old := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339Nano)
		if err := saveManagedInstances(storePath, []managedAgentInstance{{
			ID:           "inst-1",
			Type:         "picoclaw",
			AgentID:      "picoclaw",
			RuntimeState: "stopped",
			CreatedAt:    old,
			UpdatedAt:    old,
		}}); err != nil {
			t.Fatalf("saveManagedInstances: %v", err)
		}

		_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"GET /api/v1/agents/picoclaw/status": func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"statuses": []map[string]interface{}{{"id": "picoclaw", "runtimeState": "stopped"}},
				})
			},
			"GET /api/v1/agents": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error": map[string]string{"code": "E_COMMAND_FAILED", "message": "boom"},
				})
			},
		})

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/v1/instances", nil)
		handleWebUIInstances(rec, req, "req-backfill-error", daemon)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("persistence errors are tolerated", func(t *testing.T) {
		tmp := t.TempDir()
		storePath := filepath.Join(tmp, "instances.json")
		t.Setenv("CARRIER_INSTANCE_STORE", storePath)

		old := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339Nano)
		if err := saveManagedInstances(storePath, []managedAgentInstance{{
			ID:           "inst-1",
			Type:         "picoclaw",
			AgentID:      "picoclaw",
			RuntimeState: "stopped",
			CreatedAt:    old,
			UpdatedAt:    old,
		}}); err != nil {
			t.Fatalf("saveManagedInstances: %v", err)
		}
		if err := os.Chmod(storePath, 0o400); err != nil {
			t.Fatalf("chmod store read-only: %v", err)
		}

		_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"GET /api/v1/agents/picoclaw/status": func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"statuses": []map[string]interface{}{{"id": "picoclaw", "runtimeState": "running"}},
				})
			},
			"GET /api/v1/agents": func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"agents": []map[string]interface{}{
						{"id": "picoclaw", "installState": "installed", "runtimeState": "running"},
					},
				})
			},
		})

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/v1/instances", nil)
		handleWebUIInstances(rec, req, "req-persist-error", daemon)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"result":"ok"`) {
			t.Fatalf("expected ok payload, got %s", rec.Body.String())
		}
	})
}

func TestHandleWebUIInstance_Branches(t *testing.T) {
	tmp := t.TempDir()
	storePath := filepath.Join(tmp, "instances.json")
	t.Setenv("CARRIER_INSTANCE_STORE", storePath)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := saveManagedInstances(storePath, []managedAgentInstance{{
		ID:           "inst-1",
		Type:         "picoclaw",
		AgentID:      "picoclaw",
		Isolation:    true,
		RuntimeState: "running",
		CreatedAt:    now,
		UpdatedAt:    now,
	}}); err != nil {
		t.Fatalf("saveManagedInstances: %v", err)
	}

	var startIsolation any
	_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
		"GET /api/v1/agents/picoclaw/logs": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"lines":     []string{"line-1", "line-2"},
				"truncated": false,
			})
		},
		"POST /api/v1/agents/picoclaw/start": func(w http.ResponseWriter, r *http.Request) {
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
				startIsolation = payload["isolation"]
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		},
		"POST /api/v1/agents/picoclaw/stop": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		},
		"POST /api/v1/agents/picoclaw/uninstall": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{"code": "E_AGENT_NOT_FOUND", "message": "not found"},
			})
		},
	})

	t.Run("single-instance method guard", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/instances/inst-1", nil)
		handleWebUIInstance(rec, req, "req-single-guard", daemon)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
	})

	t.Run("unsupported action", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/instances/inst-1/restart", nil)
		handleWebUIInstance(rec, req, "req-unsupported", daemon)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})

	t.Run("logs action success", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/inst-1/logs?tail=10", nil)
		handleWebUIInstance(rec, req, "req-logs", daemon)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "line-1") {
			t.Fatalf("expected logs in response, got %s", rec.Body.String())
		}
	})

	t.Run("start action success", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/instances/inst-1/start", strings.NewReader(`{}`))
		handleWebUIInstance(rec, req, "req-start", daemon)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if startIsolation != true {
			t.Fatalf("expected isolation=true in start payload, got %#v", startIsolation)
		}
	})

	t.Run("uninstall tolerates daemon not found and removes instance", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/instances/inst-1/uninstall", strings.NewReader(`{}`))
		handleWebUIInstance(rec, req, "req-uninstall", daemon)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"action":"uninstall"`) {
			t.Fatalf("expected uninstall response, got %s", rec.Body.String())
		}
		instances, _, err := loadManagedInstances()
		if err != nil {
			t.Fatalf("loadManagedInstances: %v", err)
		}
		if len(instances) != 0 {
			t.Fatalf("expected instance removed after uninstall, got %+v", instances)
		}
	})
}

func TestHandleWebUIInstance_ErrorBranches(t *testing.T) {
	t.Run("missing instance path", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/", nil)
		handleWebUIInstance(rec, req, "req-missing-path", nil)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("load error", func(t *testing.T) {
		tmp := t.TempDir()
		badPath := filepath.Join(tmp, "instances.json")
		if err := os.MkdirAll(badPath, 0o700); err != nil {
			t.Fatalf("mkdir bad path: %v", err)
		}
		t.Setenv("CARRIER_INSTANCE_STORE", badPath)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/inst-1", nil)
		handleWebUIInstance(rec, req, "req-load-error", nil)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("instance not found", func(t *testing.T) {
		tmp := t.TempDir()
		storePath := filepath.Join(tmp, "instances.json")
		t.Setenv("CARRIER_INSTANCE_STORE", storePath)
		if err := saveManagedInstances(storePath, []managedAgentInstance{}); err != nil {
			t.Fatalf("saveManagedInstances: %v", err)
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/inst-404", nil)
		handleWebUIInstance(rec, req, "req-not-found", nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("logs method not allowed", func(t *testing.T) {
		tmp := t.TempDir()
		storePath := filepath.Join(tmp, "instances.json")
		t.Setenv("CARRIER_INSTANCE_STORE", storePath)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if err := saveManagedInstances(storePath, []managedAgentInstance{{ID: "inst-1", AgentID: "picoclaw", CreatedAt: now, UpdatedAt: now}}); err != nil {
			t.Fatalf("saveManagedInstances: %v", err)
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/instances/inst-1/logs", strings.NewReader(`{}`))
		handleWebUIInstance(rec, req, "req-logs-method", nil)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("logs daemon error", func(t *testing.T) {
		tmp := t.TempDir()
		storePath := filepath.Join(tmp, "instances.json")
		t.Setenv("CARRIER_INSTANCE_STORE", storePath)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if err := saveManagedInstances(storePath, []managedAgentInstance{{ID: "inst-1", AgentID: "picoclaw", CreatedAt: now, UpdatedAt: now}}); err != nil {
			t.Fatalf("saveManagedInstances: %v", err)
		}
		_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"GET /api/v1/agents/picoclaw/logs": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error": map[string]string{"code": "E_COMMAND_FAILED", "message": "boom"},
				})
			},
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/inst-1/logs", nil)
		handleWebUIInstance(rec, req, "req-logs-daemon-error", daemon)
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected 502, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("start method not allowed", func(t *testing.T) {
		tmp := t.TempDir()
		storePath := filepath.Join(tmp, "instances.json")
		t.Setenv("CARRIER_INSTANCE_STORE", storePath)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if err := saveManagedInstances(storePath, []managedAgentInstance{{ID: "inst-1", AgentID: "picoclaw", CreatedAt: now, UpdatedAt: now}}); err != nil {
			t.Fatalf("saveManagedInstances: %v", err)
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/inst-1/start", nil)
		handleWebUIInstance(rec, req, "req-start-method", nil)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("start daemon error", func(t *testing.T) {
		tmp := t.TempDir()
		storePath := filepath.Join(tmp, "instances.json")
		t.Setenv("CARRIER_INSTANCE_STORE", storePath)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if err := saveManagedInstances(storePath, []managedAgentInstance{{ID: "inst-1", AgentID: "picoclaw", CreatedAt: now, UpdatedAt: now}}); err != nil {
			t.Fatalf("saveManagedInstances: %v", err)
		}
		_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"POST /api/v1/agents/picoclaw/start": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error": map[string]string{"code": "E_ALREADY_RUNNING", "message": "already running"},
				})
			},
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/instances/inst-1/start", strings.NewReader(`{}`))
		handleWebUIInstance(rec, req, "req-start-daemon-error", daemon)
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("start persistence error", func(t *testing.T) {
		tmp := t.TempDir()
		storePath := filepath.Join(tmp, "instances.json")
		t.Setenv("CARRIER_INSTANCE_STORE", storePath)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if err := saveManagedInstances(storePath, []managedAgentInstance{{ID: "inst-1", AgentID: "picoclaw", CreatedAt: now, UpdatedAt: now}}); err != nil {
			t.Fatalf("saveManagedInstances: %v", err)
		}
		_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"POST /api/v1/agents/picoclaw/start": func(w http.ResponseWriter, r *http.Request) {
				if err := os.Remove(storePath); err != nil {
					t.Fatalf("remove store file: %v", err)
				}
				if err := os.Mkdir(storePath, 0o700); err != nil {
					t.Fatalf("replace store with dir: %v", err)
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
			},
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/instances/inst-1/start", strings.NewReader(`{}`))
		handleWebUIInstance(rec, req, "req-start-save-error", daemon)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"errorCode":"E_STATE_PERSISTENCE"`) {
			t.Fatalf("expected E_STATE_PERSISTENCE, got %s", rec.Body.String())
		}
	})

	t.Run("stop daemon error", func(t *testing.T) {
		tmp := t.TempDir()
		storePath := filepath.Join(tmp, "instances.json")
		t.Setenv("CARRIER_INSTANCE_STORE", storePath)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if err := saveManagedInstances(storePath, []managedAgentInstance{{ID: "inst-1", AgentID: "picoclaw", CreatedAt: now, UpdatedAt: now}}); err != nil {
			t.Fatalf("saveManagedInstances: %v", err)
		}
		_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"POST /api/v1/agents/picoclaw/stop": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error": map[string]string{"code": "E_ALREADY_STOPPED", "message": "already stopped"},
				})
			},
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/instances/inst-1/stop", strings.NewReader(`{}`))
		handleWebUIInstance(rec, req, "req-stop-daemon-error", daemon)
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("stop persistence error", func(t *testing.T) {
		tmp := t.TempDir()
		storePath := filepath.Join(tmp, "instances.json")
		t.Setenv("CARRIER_INSTANCE_STORE", storePath)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if err := saveManagedInstances(storePath, []managedAgentInstance{{ID: "inst-1", AgentID: "picoclaw", CreatedAt: now, UpdatedAt: now}}); err != nil {
			t.Fatalf("saveManagedInstances: %v", err)
		}
		_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"POST /api/v1/agents/picoclaw/stop": func(w http.ResponseWriter, r *http.Request) {
				if err := os.Remove(storePath); err != nil {
					t.Fatalf("remove store file: %v", err)
				}
				if err := os.Mkdir(storePath, 0o700); err != nil {
					t.Fatalf("replace store with dir: %v", err)
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
			},
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/instances/inst-1/stop", strings.NewReader(`{}`))
		handleWebUIInstance(rec, req, "req-stop-save-error", daemon)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"errorCode":"E_STATE_PERSISTENCE"`) {
			t.Fatalf("expected E_STATE_PERSISTENCE, got %s", rec.Body.String())
		}
	})

	t.Run("uninstall daemon error", func(t *testing.T) {
		tmp := t.TempDir()
		storePath := filepath.Join(tmp, "instances.json")
		t.Setenv("CARRIER_INSTANCE_STORE", storePath)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if err := saveManagedInstances(storePath, []managedAgentInstance{{ID: "inst-1", AgentID: "picoclaw", CreatedAt: now, UpdatedAt: now}}); err != nil {
			t.Fatalf("saveManagedInstances: %v", err)
		}
		_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"POST /api/v1/agents/picoclaw/stop": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
			},
			"POST /api/v1/agents/picoclaw/uninstall": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error": map[string]string{"code": "E_COMMAND_FAILED", "message": "boom"},
				})
			},
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/instances/inst-1/uninstall", strings.NewReader(`{}`))
		handleWebUIInstance(rec, req, "req-uninstall-daemon-error", daemon)
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected 502, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("uninstall warning on cleanup error", func(t *testing.T) {
		tmp := t.TempDir()
		storePath := filepath.Join(tmp, "instances.json")
		t.Setenv("CARRIER_INSTANCE_STORE", storePath)
		recordDir := filepath.Join(tmp, "record-dir")
		if err := os.MkdirAll(recordDir, 0o700); err != nil {
			t.Fatalf("mkdir record-dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(recordDir, "keep.log"), []byte("x"), 0o600); err != nil {
			t.Fatalf("seed record-dir: %v", err)
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if err := saveManagedInstances(storePath, []managedAgentInstance{{
			ID:         "inst-1",
			AgentID:    "picoclaw",
			RecordPath: recordDir, // non-empty directory makes os.Remove fail and sets warning
			CreatedAt:  now,
			UpdatedAt:  now,
		}}); err != nil {
			t.Fatalf("saveManagedInstances: %v", err)
		}
		_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"POST /api/v1/agents/picoclaw/stop": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
			},
			"POST /api/v1/agents/picoclaw/uninstall": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error": map[string]string{"code": "E_AGENT_NOT_FOUND", "message": "not found"},
				})
			},
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/instances/inst-1/uninstall", strings.NewReader(`{}`))
		handleWebUIInstance(rec, req, "req-uninstall-warning", daemon)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"warning"`) {
			t.Fatalf("expected warning in response, got %s", rec.Body.String())
		}
	})

	t.Run("uninstall persistence error", func(t *testing.T) {
		tmp := t.TempDir()
		storePath := filepath.Join(tmp, "instances.json")
		t.Setenv("CARRIER_INSTANCE_STORE", storePath)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if err := saveManagedInstances(storePath, []managedAgentInstance{{ID: "inst-1", AgentID: "picoclaw", CreatedAt: now, UpdatedAt: now}}); err != nil {
			t.Fatalf("saveManagedInstances: %v", err)
		}
		_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"POST /api/v1/agents/picoclaw/stop": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
			},
			"POST /api/v1/agents/picoclaw/uninstall": func(w http.ResponseWriter, r *http.Request) {
				if err := os.Remove(storePath); err != nil {
					t.Fatalf("remove store file: %v", err)
				}
				if err := os.Mkdir(storePath, 0o700); err != nil {
					t.Fatalf("replace store with dir: %v", err)
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
			},
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/instances/inst-1/uninstall", strings.NewReader(`{}`))
		handleWebUIInstance(rec, req, "req-uninstall-save-error", daemon)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"errorCode":"E_STATE_PERSISTENCE"`) {
			t.Fatalf("expected E_STATE_PERSISTENCE, got %s", rec.Body.String())
		}
	})
}

func TestMergeManagedRuntimeState_Branches(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/v1/instances", nil)

	if changed := mergeManagedRuntimeState(req, nil, nil, "req-0"); changed {
		t.Fatal("expected changed=false for nil daemon/instances")
	}

	t.Run("no agent IDs to query", func(t *testing.T) {
		instances := []managedAgentInstance{{ID: "x", AgentID: ""}}
		if changed := mergeManagedRuntimeState(req, nil, instances, "req-no-agent"); changed {
			t.Fatal("expected changed=false")
		}
	})

	t.Run("status errors and empty statuses return unchanged", func(t *testing.T) {
		_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"GET /api/v1/agents/a1/status": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error": map[string]string{"code": "E_COMMAND_FAILED", "message": "boom"},
				})
			},
			"GET /api/v1/agents/a2/status": func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"statuses": []map[string]interface{}{}})
			},
		})
		instances := []managedAgentInstance{
			{ID: "i1", AgentID: "a1", RuntimeState: "stopped"},
			{ID: "i2", AgentID: "a2", RuntimeState: "stopped"},
		}
		if changed := mergeManagedRuntimeState(req, daemon, instances, "req-status-error"); changed {
			t.Fatal("expected changed=false")
		}
	})

	t.Run("runtime empty is ignored", func(t *testing.T) {
		_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"GET /api/v1/agents/a1/status": func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"statuses": []map[string]interface{}{{"id": "a1", "runtimeState": ""}},
				})
			},
		})
		instances := []managedAgentInstance{{ID: "i1", AgentID: "a1", RuntimeState: "stopped"}}
		if changed := mergeManagedRuntimeState(req, daemon, instances, "req-runtime-empty"); changed {
			t.Fatal("expected changed=false when daemon runtime is empty")
		}
	})

	t.Run("deduplicates agent status fetch and updates all matching instances", func(t *testing.T) {
		var statusCalls int32
		_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"GET /api/v1/agents/a1/status": func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&statusCalls, 1)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"statuses": []map[string]interface{}{{"id": "a1", "runtimeState": "running"}},
				})
			},
		})
		instances := []managedAgentInstance{
			{ID: "i1", AgentID: "a1", RuntimeState: "stopped"},
			{ID: "i2", AgentID: "A1", RuntimeState: "stopped"},
			{ID: "i3", AgentID: "a3", RuntimeState: "unknown"},
		}
		changed := mergeManagedRuntimeState(req, daemon, instances, "req-update")
		if !changed {
			t.Fatal("expected changed=true")
		}
		if instances[0].RuntimeState != "running" || instances[1].RuntimeState != "running" {
			t.Fatalf("expected matching instances updated to running, got %+v", instances)
		}
		if instances[2].RuntimeState != "unknown" {
			t.Fatalf("expected unrelated instance unchanged, got %+v", instances[2])
		}
		if got := atomic.LoadInt32(&statusCalls); got != 1 {
			t.Fatalf("expected one status call for deduplicated agent id, got %d", got)
		}
	})
}
