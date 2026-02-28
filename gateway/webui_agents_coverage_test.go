package gateway

import (
	crand "crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mustLoadManagedInstancesFile(t *testing.T, path string) []managedAgentInstance {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read instances file: %v", err)
	}
	var file managedAgentInstanceFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("unmarshal instances file: %v", err)
	}
	return file.Instances
}

func TestManagedInstancesPath_Branches(t *testing.T) {
	t.Run("env override", func(t *testing.T) {
		custom := filepath.Join(t.TempDir(), "custom-instances.json")
		t.Setenv("CARRIER_INSTANCE_STORE", custom)
		got, err := managedInstancesPath()
		if err != nil {
			t.Fatalf("managedInstancesPath: %v", err)
		}
		if got != custom {
			t.Fatalf("managedInstancesPath = %q, want %q", got, custom)
		}
	})

	t.Run("home fallback", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("CARRIER_INSTANCE_STORE", "")
		t.Setenv("HOME", home)
		got, err := managedInstancesPath()
		if err != nil {
			t.Fatalf("managedInstancesPath: %v", err)
		}
		want := filepath.Join(home, ".carrier", "instances.json")
		if got != want {
			t.Fatalf("managedInstancesPath = %q, want %q", got, want)
		}
	})
}

func TestManagedInstancesIO_ErrorBranches(t *testing.T) {
	t.Run("save path empty", func(t *testing.T) {
		if err := saveManagedInstances("", []managedAgentInstance{}); err == nil {
			t.Fatal("expected error for empty store path")
		}
	})

	t.Run("save mkdir failure", func(t *testing.T) {
		tmp := t.TempDir()
		blockingFile := filepath.Join(tmp, "blocking")
		if err := os.WriteFile(blockingFile, []byte("x"), 0o600); err != nil {
			t.Fatalf("write blocking file: %v", err)
		}
		storePath := filepath.Join(blockingFile, "instances.json")
		if err := saveManagedInstances(storePath, []managedAgentInstance{}); err == nil {
			t.Fatal("expected create dir failure")
		}
	})

	t.Run("load malformed json", func(t *testing.T) {
		tmp := t.TempDir()
		storePath := filepath.Join(tmp, "instances.json")
		if err := os.WriteFile(storePath, []byte("{bad json"), 0o600); err != nil {
			t.Fatalf("write malformed json: %v", err)
		}
		t.Setenv("CARRIER_INSTANCE_STORE", storePath)
		if _, _, err := loadManagedInstances(); err == nil {
			t.Fatal("expected parse error for malformed instance store")
		}
	})
}

func TestGenerateManagedInstanceID_Branches(t *testing.T) {
	t.Run("default prefix and format", func(t *testing.T) {
		id, err := generateManagedInstanceID("  ")
		if err != nil {
			t.Fatalf("generateManagedInstanceID: %v", err)
		}
		if !strings.HasPrefix(id, "agent-") {
			t.Fatalf("expected default prefix agent-, got %q", id)
		}
	})

	t.Run("rand read error", func(t *testing.T) {
		orig := crand.Reader
		crand.Reader = failingReader{}
		t.Cleanup(func() {
			crand.Reader = orig
		})

		if _, err := generateManagedInstanceID("picoclaw"); err == nil {
			t.Fatal("expected random-id generation error")
		}
	})
}

type failingReader struct{}

func (failingReader) Read(_ []byte) (int, error) {
	return 0, errors.New("forced random failure")
}

func TestUpsertManagedInstance_Branches(t *testing.T) {
	tmp := t.TempDir()
	storePath := filepath.Join(tmp, "instances.json")
	t.Setenv("CARRIER_INSTANCE_STORE", storePath)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	inst := managedAgentInstance{
		ID:           "picoclaw-default",
		Type:         "picoclaw",
		AgentID:      "picoclaw",
		RuntimeState: "stopped",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := upsertManagedInstance(inst); err != nil {
		t.Fatalf("upsertManagedInstance insert: %v", err)
	}
	instances := mustLoadManagedInstancesFile(t, storePath)
	if len(instances) != 1 || instances[0].RuntimeState != "stopped" {
		t.Fatalf("unexpected inserted instances: %+v", instances)
	}

	inst.RuntimeState = "running"
	if err := upsertManagedInstance(inst); err != nil {
		t.Fatalf("upsertManagedInstance update: %v", err)
	}
	instances = mustLoadManagedInstancesFile(t, storePath)
	if len(instances) != 1 || instances[0].RuntimeState != "running" {
		t.Fatalf("unexpected updated instances: %+v", instances)
	}
}

func TestHandleWebUIAgents_Branches(t *testing.T) {
	t.Run("method not allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", nil)
		handleWebUIAgents(rec, req, "req-method", nil)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
	})

	t.Run("daemon error", func(t *testing.T) {
		_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"GET /api/v1/agents": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error": map[string]string{"code": "E_COMMAND_FAILED", "message": "boom"},
				})
			},
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
		handleWebUIAgents(rec, req, "req-daemon-error", daemon)
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected 502, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("success", func(t *testing.T) {
		_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"GET /api/v1/agents": func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"agents": []map[string]interface{}{
						{"id": "picoclaw", "installState": "installed", "runtimeState": "running"},
					},
				})
			},
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
		handleWebUIAgents(rec, req, "req-ok", daemon)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"id":"picoclaw"`) {
			t.Fatalf("expected picoclaw in body, got %s", rec.Body.String())
		}
	})
}

func TestHandleWebUIAgent_Branches(t *testing.T) {
	t.Run("path guards", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/", nil)
		handleWebUIAgent(rec, req, "req-empty", nil)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}

		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/api/v1/agents/picoclaw", nil)
		handleWebUIAgent(rec, req, "req-missing-action", nil)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}

		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/api/v1/agents/picoclaw//extra", nil)
		handleWebUIAgent(rec, req, "req-missing-action", nil)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("status action", func(t *testing.T) {
		_, daemonErr, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"GET /api/v1/agents/picoclaw/status": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error": map[string]string{"code": "E_COMMAND_FAILED", "message": "boom"},
				})
			},
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/picoclaw/status", nil)
		handleWebUIAgent(rec, req, "req-status-method", daemonErr)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}

		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/api/v1/agents/picoclaw/status", nil)
		handleWebUIAgent(rec, req, "req-status-daemon-error", daemonErr)
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected 502, got %d: %s", rec.Code, rec.Body.String())
		}

		_, daemonEmpty, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"GET /api/v1/agents/picoclaw/status": func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"statuses": []map[string]interface{}{}})
			},
		})
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/api/v1/agents/picoclaw/status", nil)
		handleWebUIAgent(rec, req, "req-status-empty", daemonEmpty)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}

		_, daemonOK, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"GET /api/v1/agents/picoclaw/status": func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"statuses": []map[string]interface{}{
						{"id": "picoclaw", "runtimeState": "running"},
					},
				})
			},
		})
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/api/v1/agents/picoclaw/status", nil)
		handleWebUIAgent(rec, req, "req-status-ok", daemonOK)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("logs action", func(t *testing.T) {
		_, daemonErr, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"GET /api/v1/agents/picoclaw/logs": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error": map[string]string{"code": "E_COMMAND_FAILED", "message": "boom"},
				})
			},
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/picoclaw/logs", nil)
		handleWebUIAgent(rec, req, "req-logs-method", daemonErr)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}

		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/api/v1/agents/picoclaw/logs?tail=123", nil)
		handleWebUIAgent(rec, req, "req-logs-daemon-error", daemonErr)
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected 502, got %d: %s", rec.Code, rec.Body.String())
		}

		_, daemonOK, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"GET /api/v1/agents/picoclaw/logs": func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"lines":     []string{"line-1", "line-2"},
					"truncated": false,
				})
			},
		})
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/api/v1/agents/picoclaw/logs?tail=oops", nil)
		handleWebUIAgent(rec, req, "req-logs-ok", daemonOK)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"line-1"`) {
			t.Fatalf("expected logs payload, got %s", rec.Body.String())
		}
	})

	t.Run("unsupported action", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/picoclaw/unknown", nil)
		handleWebUIAgent(rec, req, "req-unsupported", nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})

	t.Run("lifecycle actions", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/picoclaw/install", nil)
		handleWebUIAgent(rec, req, "req-install-method", nil)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}

		tmp := t.TempDir()
		storePath := filepath.Join(tmp, "instances.json")
		t.Setenv("CARRIER_INSTANCE_STORE", storePath)
		_, daemonError, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"POST /api/v1/agents/picoclaw/start": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error": map[string]string{"code": "E_ALREADY_RUNNING", "message": "already running"},
				})
			},
		})
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/api/v1/agents/picoclaw/start", nil)
		handleWebUIAgent(rec, req, "req-start-daemon-error", daemonError)
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
		}

		badStore := filepath.Join(tmp, "bad-store")
		if err := os.MkdirAll(badStore, 0o700); err != nil {
			t.Fatalf("mkdir bad-store: %v", err)
		}
		t.Setenv("CARRIER_INSTANCE_STORE", badStore)
		_, daemonStartOK, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"POST /api/v1/agents/picoclaw/start": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
			},
		})
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/api/v1/agents/picoclaw/start", nil)
		handleWebUIAgent(rec, req, "req-start-sync-error", daemonStartOK)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"errorCode":"E_STATE_PERSISTENCE"`) {
			t.Fatalf("expected E_STATE_PERSISTENCE, got %s", rec.Body.String())
		}

		t.Setenv("CARRIER_INSTANCE_STORE", storePath)
		_, daemonInstallOK, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"POST /api/v1/agents/picoclaw/install": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
			},
		})
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "http://gateway.local/api/v1/agents/picoclaw/install", nil)
		handleWebUIAgent(rec, req, "req-install-ok", daemonInstallOK)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"action":"install"`) {
			t.Fatalf("expected install action in body, got %s", rec.Body.String())
		}
		instances := mustLoadManagedInstancesFile(t, storePath)
		if len(instances) != 1 || instances[0].RuntimeState != "stopped" {
			t.Fatalf("unexpected managed instances after install: %+v", instances)
		}
	})

	t.Run("start forwards stored isolation option", func(t *testing.T) {
		tmp := t.TempDir()
		storePath := filepath.Join(tmp, "instances.json")
		t.Setenv("CARRIER_INSTANCE_STORE", storePath)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if err := saveManagedInstances(storePath, []managedAgentInstance{{
			ID:           "picoclaw-default",
			AgentID:      "picoclaw",
			Isolation:    true,
			RuntimeState: "stopped",
			CreatedAt:    now,
			UpdatedAt:    now,
		}}); err != nil {
			t.Fatalf("saveManagedInstances: %v", err)
		}

		var startIsolation any
		_, daemonStartOK, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"POST /api/v1/agents/picoclaw/start": func(w http.ResponseWriter, r *http.Request) {
				var payload map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
					startIsolation = payload["isolation"]
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
			},
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://gateway.local/api/v1/agents/picoclaw/start", nil)
		handleWebUIAgent(rec, req, "req-start-isolation", daemonStartOK)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if startIsolation != true {
			t.Fatalf("expected start payload isolation=true, got %#v", startIsolation)
		}
	})
}

func TestSyncManagedInstanceByAgentAction_Branches(t *testing.T) {
	t.Run("load error", func(t *testing.T) {
		tmp := t.TempDir()
		badPath := filepath.Join(tmp, "instances.json")
		if err := os.MkdirAll(badPath, 0o700); err != nil {
			t.Fatalf("mkdir bad path: %v", err)
		}
		t.Setenv("CARRIER_INSTANCE_STORE", badPath)
		req := httptest.NewRequest(http.MethodPost, "http://gateway.local/api/v1/agents/picoclaw/start", nil)
		if err := syncManagedInstanceByAgentAction(req, "picoclaw", "start"); err == nil {
			t.Fatal("expected load error")
		}
	})

	t.Run("install updates existing empty runtime", func(t *testing.T) {
		tmp := t.TempDir()
		storePath := filepath.Join(tmp, "instances.json")
		t.Setenv("CARRIER_INSTANCE_STORE", storePath)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if err := saveManagedInstances(storePath, []managedAgentInstance{{
			ID: "picoclaw-default", AgentID: "picoclaw", RuntimeState: "", CreatedAt: now, UpdatedAt: now,
		}}); err != nil {
			t.Fatalf("saveManagedInstances: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "http://gateway.local/api/v1/agents/picoclaw/install", nil)
		if err := syncManagedInstanceByAgentAction(req, "picoclaw", "install"); err != nil {
			t.Fatalf("syncManagedInstanceByAgentAction: %v", err)
		}
		instances := mustLoadManagedInstancesFile(t, storePath)
		if len(instances) != 1 || instances[0].RuntimeState != "stopped" {
			t.Fatalf("unexpected managed instances: %+v", instances)
		}
	})

	t.Run("install creates generated id on default conflict", func(t *testing.T) {
		tmp := t.TempDir()
		storePath := filepath.Join(tmp, "instances.json")
		t.Setenv("CARRIER_INSTANCE_STORE", storePath)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if err := saveManagedInstances(storePath, []managedAgentInstance{{
			ID: "picoclaw-default", AgentID: "other-agent", RuntimeState: "running", CreatedAt: now, UpdatedAt: now,
		}}); err != nil {
			t.Fatalf("saveManagedInstances: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "http://gateway.local/api/v1/agents/picoclaw/install", nil)
		if err := syncManagedInstanceByAgentAction(req, "picoclaw", "install"); err != nil {
			t.Fatalf("syncManagedInstanceByAgentAction: %v", err)
		}
		instances := mustLoadManagedInstancesFile(t, storePath)
		if len(instances) != 2 {
			t.Fatalf("expected 2 instances, got %+v", instances)
		}
		if instances[1].ID == "picoclaw-default" || !strings.HasPrefix(instances[1].ID, "picoclaw-") {
			t.Fatalf("expected generated picoclaw-* id, got %q", instances[1].ID)
		}
		if instances[1].RuntimeState != "stopped" {
			t.Fatalf("expected stopped runtime state, got %q", instances[1].RuntimeState)
		}
	})

	t.Run("start and stop update runtime state", func(t *testing.T) {
		tmp := t.TempDir()
		storePath := filepath.Join(tmp, "instances.json")
		t.Setenv("CARRIER_INSTANCE_STORE", storePath)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if err := saveManagedInstances(storePath, []managedAgentInstance{{
			ID: "picoclaw-default", AgentID: "picoclaw", RuntimeState: "stopped", CreatedAt: now, UpdatedAt: now,
		}}); err != nil {
			t.Fatalf("saveManagedInstances: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "http://gateway.local/api/v1/agents/picoclaw/start", nil)
		if err := syncManagedInstanceByAgentAction(req, "picoclaw", "start"); err != nil {
			t.Fatalf("sync start: %v", err)
		}
		instances := mustLoadManagedInstancesFile(t, storePath)
		if instances[0].RuntimeState != "running" {
			t.Fatalf("expected running after start, got %q", instances[0].RuntimeState)
		}
		req = httptest.NewRequest(http.MethodPost, "http://gateway.local/api/v1/agents/picoclaw/stop", nil)
		if err := syncManagedInstanceByAgentAction(req, "picoclaw", "stop"); err != nil {
			t.Fatalf("sync stop: %v", err)
		}
		instances = mustLoadManagedInstancesFile(t, storePath)
		if instances[0].RuntimeState != "stopped" {
			t.Fatalf("expected stopped after stop, got %q", instances[0].RuntimeState)
		}
	})

	t.Run("start creates missing instance", func(t *testing.T) {
		tmp := t.TempDir()
		storePath := filepath.Join(tmp, "instances.json")
		t.Setenv("CARRIER_INSTANCE_STORE", storePath)
		req := httptest.NewRequest(http.MethodPost, "http://gateway.local/api/v1/agents/picoclaw/start", nil)
		if err := syncManagedInstanceByAgentAction(req, "picoclaw", "start"); err != nil {
			t.Fatalf("syncManagedInstanceByAgentAction: %v", err)
		}
		instances := mustLoadManagedInstancesFile(t, storePath)
		if len(instances) != 1 || instances[0].RuntimeState != "running" {
			t.Fatalf("unexpected managed instances: %+v", instances)
		}
	})

	t.Run("uninstall missing returns nil", func(t *testing.T) {
		tmp := t.TempDir()
		storePath := filepath.Join(tmp, "instances.json")
		t.Setenv("CARRIER_INSTANCE_STORE", storePath)
		if err := saveManagedInstances(storePath, []managedAgentInstance{}); err != nil {
			t.Fatalf("saveManagedInstances: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "http://gateway.local/api/v1/agents/picoclaw/uninstall", nil)
		if err := syncManagedInstanceByAgentAction(req, "picoclaw", "uninstall"); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("uninstall existing tolerates cleanup failure", func(t *testing.T) {
		tmp := t.TempDir()
		storePath := filepath.Join(tmp, "instances.json")
		recordDir := filepath.Join(tmp, "record-dir")
		if err := os.MkdirAll(recordDir, 0o700); err != nil {
			t.Fatalf("mkdir record-dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(recordDir, "keep.log"), []byte("x"), 0o600); err != nil {
			t.Fatalf("seed record-dir: %v", err)
		}
		t.Setenv("CARRIER_INSTANCE_STORE", storePath)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if err := saveManagedInstances(storePath, []managedAgentInstance{{
			ID: "picoclaw-default", AgentID: "picoclaw", RecordPath: recordDir, CreatedAt: now, UpdatedAt: now,
		}}); err != nil {
			t.Fatalf("saveManagedInstances: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "http://gateway.local/api/v1/agents/picoclaw/uninstall", nil)
		if err := syncManagedInstanceByAgentAction(req, "picoclaw", "uninstall"); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		instances := mustLoadManagedInstancesFile(t, storePath)
		if len(instances) != 0 {
			t.Fatalf("expected empty instances after uninstall, got %+v", instances)
		}
	})

	t.Run("unknown action is ignored", func(t *testing.T) {
		tmp := t.TempDir()
		storePath := filepath.Join(tmp, "instances.json")
		t.Setenv("CARRIER_INSTANCE_STORE", storePath)
		if err := saveManagedInstances(storePath, []managedAgentInstance{}); err != nil {
			t.Fatalf("saveManagedInstances: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "http://gateway.local/api/v1/agents/picoclaw/noop", nil)
		if err := syncManagedInstanceByAgentAction(req, "picoclaw", "noop"); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})
}
