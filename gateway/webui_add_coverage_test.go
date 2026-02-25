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

func TestHandleWebUIAdd_NonManagedAgentSuccess(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))

	_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
		"POST /api/v1/agents/worker/install": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		},
		"POST /api/v1/agents/worker/start": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		},
	})

	req := httptest.NewRequest(http.MethodPost, "http://gateway.local/api/v1/add", strings.NewReader(`{"agentId":"worker"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handleWebUIAdd(rec, req, "req-non-managed", daemon)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if strings.TrimSpace(resp["result"].(string)) != "ok" {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if strings.TrimSpace(resp["agentId"].(string)) != "worker" {
		t.Fatalf("expected agentId worker, got %#v", resp["agentId"])
	}
	if strings.TrimSpace(resp["instanceId"].(string)) == "" {
		t.Fatalf("expected non-empty instanceId, got %#v", resp["instanceId"])
	}

	instances, _, err := loadManagedInstances()
	if err != nil {
		t.Fatalf("loadManagedInstances: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(instances))
	}
	if instances[0].AgentID != "worker" || instances[0].Type != "worker" {
		t.Fatalf("unexpected managed instance: %+v", instances[0])
	}
	if instances[0].RuntimeState != "running" {
		t.Fatalf("expected runtime_state running, got %+v", instances[0])
	}
}

func TestResolveWebUIAddProviderID_FallbackOrder(t *testing.T) {
	t.Run("invalid default falls back to latest instance provider", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))
		writeGatewayDefaultProviderConfig(t, "unknown-provider", "unknown/model", "UNKNOWN_KEY")

		now := time.Now().UTC().Format(time.RFC3339Nano)
		path := filepath.Join(tmp, "instances.json")
		if err := saveManagedInstances(path, []managedAgentInstance{
			{
				ID:        "openclaw-1",
				Type:      "openclaw",
				AgentID:   "openclaw",
				Provider:  "anthropic",
				UpdatedAt: now,
			},
		}); err != nil {
			t.Fatalf("saveManagedInstances: %v", err)
		}

		if got := resolveWebUIAddProviderID("openclaw"); got != "anthropic" {
			t.Fatalf("resolveWebUIAddProviderID(openclaw) = %q, want anthropic", got)
		}
	})

	t.Run("saved credential fallback", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("CARRIER_CONFIG", filepath.Join(tmp, "missing-config.json"))
		t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))
		t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
		t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))

		if _, err := saveProviderCredential("anthropic", "saved-token-1"); err != nil {
			t.Fatalf("saveProviderCredential: %v", err)
		}
		if got := resolveWebUIAddProviderID("openclaw"); got != "anthropic" {
			t.Fatalf("resolveWebUIAddProviderID(openclaw) = %q, want anthropic", got)
		}
	})

	t.Run("falls back to openai-codex when no default instance or credential", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("CARRIER_CONFIG", filepath.Join(tmp, "missing-config.json"))
		t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))
		t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
		t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))

		if got := resolveWebUIAddProviderID("openclaw"); got != "openai-codex" {
			t.Fatalf("resolveWebUIAddProviderID(openclaw) = %q, want openai-codex", got)
		}
	})
}

func TestInferManagedChannelID_Branches(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))

	if got := inferManagedChannelID("openclaw"); got != "" {
		t.Fatalf("inferManagedChannelID(openclaw) = %q, want empty", got)
	}

	older := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	newer := time.Now().UTC().Format(time.RFC3339Nano)
	path := filepath.Join(tmp, "instances.json")
	if err := saveManagedInstances(path, []managedAgentInstance{
		{
			ID:        "openclaw-old",
			Type:      "openclaw",
			AgentID:   "openclaw",
			Channel:   "telegram",
			UpdatedAt: older,
		},
		{
			ID:        "openclaw-new",
			Type:      "openclaw",
			AgentID:   "openclaw",
			Channel:   "discord",
			UpdatedAt: newer,
		},
	}); err != nil {
		t.Fatalf("saveManagedInstances: %v", err)
	}

	if got := inferManagedChannelID("openclaw"); got != "discord" {
		t.Fatalf("inferManagedChannelID(openclaw) = %q, want discord", got)
	}
}
