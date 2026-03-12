package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHandleAgentLauncherRejectsWrongMethod(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/picoclaw/launcher", nil)
	handleWebUIAgent(rec, req, "req-launcher-method", nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleAgentLauncherReturnsSummary(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	storePath := filepath.Join(tmp, ".carrier", "instances.json")
	t.Setenv("CARRIER_INSTANCE_STORE", storePath)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := saveManagedInstances(storePath, []managedAgentInstance{{
		ID:           "picoclaw-prod",
		Type:         "picoclaw",
		AgentID:      "picoclaw",
		Isolation:    true,
		GatewayURL:   "http://127.0.0.1:8787",
		Channel:      "telegram",
		Provider:     "openrouter",
		PairRequired: false,
		PairedChatID: "123456",
		RuntimeState: "running",
		CreatedAt:    now,
		UpdatedAt:    now,
	}}); err != nil {
		t.Fatalf("saveManagedInstances: %v", err)
	}
	if _, err := saveProviderCredential("openrouter", "token-openrouter"); err != nil {
		t.Fatalf("saveProviderCredential: %v", err)
	}

	_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
		"GET /api/v1/agents/picoclaw/status": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":           "picoclaw",
				"runtimeState": "running",
				"health":       "healthy",
				"memory": map[string]interface{}{
					"contractId":     "mem-prod",
					"contractDigest": "sha256:abc",
				},
				"heartbeat": map[string]interface{}{
					"state":          "fresh",
					"ageSeconds":     2,
					"lastActivityAt": now,
				},
				"updatedAt": now,
			})
		},
		"GET /api/v1/agents/picoclaw/capabilities": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"skills": []map[string]interface{}{
					{"name": "toolbox", "enabled": true},
				},
				"mcp": map[string]interface{}{
					"servers": []map[string]interface{}{
						{"name": "repo", "health": "healthy", "visibleToolCount": 3},
					},
				},
			})
		},
		"GET /api/base-agent/cron/jobs": func(w http.ResponseWriter, r *http.Request) {
			if got := r.URL.Query().Get("agentId"); got != "picoclaw" {
				t.Fatalf("agentId query=%q want picoclaw", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jobs": []map[string]interface{}{
					{
						"id":         "cron-1",
						"agentId":    "picoclaw",
						"prompt":     "check launcher",
						"nextRunAt":  now,
						"lastResult": "succeeded",
						"lastRunAt":  now,
					},
				},
			})
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/picoclaw/launcher", nil)
	handleWebUIAgent(rec, req, "req-launcher-ok", daemon)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, needle := range []string{
		`"agentId":"picoclaw"`,
		`"state":"fresh"`,
		`"contractId":"mem-prod"`,
		`"provider":"openrouter"`,
		`"credentialConfigured":true`,
		`"channel":"telegram"`,
		`"toolbox"`,
		`"count":1`,
		`"lastResult":"succeeded"`,
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected response to contain %s, got %s", needle, body)
		}
	}
}
