package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
		ID:         "picoclaw-prod",
		Type:       "picoclaw",
		AgentID:    "picoclaw",
		Isolation:  true,
		GatewayURL: "http://127.0.0.1:8787",
		Channel:    "telegram",
		Provider:   "openrouter",
		ModelSurface: &managedAgentModelSurface{
			DefaultProfile: "openrouter-fast",
			Profiles: []managedAgentModelProfile{
				{
					ProfileName:    "openrouter-fast",
					ModelAlias:     "flash",
					ModelID:        "google/gemini-2.0-flash-001",
					ProviderID:     "openrouter",
					ProviderKey:    "openrouter",
					ProtocolFamily: "openai-compatible",
					BaseURL:        "https://openrouter.ai/api/v1",
					AuthMethod:     "api_key",
					Primary:        true,
				},
				{
					ProfileName:    "openrouter-safe",
					ModelAlias:     "flash",
					ModelID:        "deepseek/deepseek-chat-v3-0324",
					ProviderID:     "openrouter",
					ProviderKey:    "openrouter",
					ProtocolFamily: "openai-compatible",
					BaseURL:        "https://openrouter.ai/api/v1",
					AuthMethod:     "api_key",
				},
			},
		},
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
					{"name": "workspace-inspection", "enabled": false},
				},
				"skillSummary": map[string]interface{}{
					"installedCount": 2,
					"enabledCount":   1,
					"disabledCount":  1,
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
		`"disabledCount":1`,
		`"count":1`,
		`"lastResult":"succeeded"`,
		`"defaultProfile":"openrouter-fast"`,
		`"modelAlias":"flash"`,
		`"modelId":"google/gemini-2.0-flash-001"`,
		`"protocolFamily":"openai-compatible"`,
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected response to contain %s, got %s", needle, body)
		}
	}
}

func TestHandleAgentLauncherTreatsEnvCredentialAsReady(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	storePath := filepath.Join(tmp, ".carrier", "instances.json")
	t.Setenv("CARRIER_INSTANCE_STORE", storePath)
	t.Setenv("OPENROUTER_API_KEY", "token-from-env")

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := saveManagedInstances(storePath, []managedAgentInstance{{
		ID:           "zeroclaw-local",
		Type:         "zeroclaw",
		AgentID:      "zeroclaw",
		Isolation:    true,
		GatewayURL:   "http://127.0.0.1:8787",
		Provider:     "openrouter",
		RuntimeState: "running",
		CreatedAt:    now,
		UpdatedAt:    now,
	}}); err != nil {
		t.Fatalf("saveManagedInstances: %v", err)
	}

	_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
		"GET /api/v1/agents/zeroclaw/status": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":           "zeroclaw",
				"runtimeState": "running",
				"health":       "healthy",
				"updatedAt":    now,
			})
		},
		"GET /api/v1/agents/zeroclaw/capabilities": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{})
		},
		"GET /api/base-agent/cron/jobs": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"jobs": []map[string]interface{}{}})
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/zeroclaw/launcher", nil)
	handleWebUIAgent(rec, req, "req-launcher-env", daemon)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, needle := range []string{
		`"provider":"openrouter"`,
		`"credentialConfigured":true`,
		`"credentialBackend":"env"`,
		`"ready":true`,
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected response to contain %s, got %s", needle, body)
		}
	}
}

func TestHandleAgentLauncherReturnsModelSurfaceFallbackMetadata(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	storePath := filepath.Join(tmp, ".carrier", "instances.json")
	t.Setenv("CARRIER_INSTANCE_STORE", storePath)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := saveManagedInstances(storePath, []managedAgentInstance{{
		ID:         "picoclaw-prod",
		Type:       "picoclaw",
		AgentID:    "picoclaw",
		Isolation:  true,
		GatewayURL: "http://127.0.0.1:8787",
		Provider:   "openrouter",
		ModelSurface: &managedAgentModelSurface{
			DefaultProfile: "openrouter-fast",
			Profiles: []managedAgentModelProfile{
				{
					ProfileName:    "openrouter-fast",
					ModelAlias:     "flash",
					ModelID:        "google/gemini-2.0-flash-001",
					ProviderID:     "openrouter",
					ProviderKey:    "openrouter",
					ProtocolFamily: "openai-compatible",
					Primary:        true,
				},
				{
					ProfileName:    "openrouter-safe",
					ModelAlias:     "flash",
					ModelID:        "deepseek/deepseek-chat-v3-0324",
					ProviderID:     "openrouter",
					ProviderKey:    "openrouter",
					ProtocolFamily: "openai-compatible",
				},
			},
		},
		RuntimeState: "running",
		CreatedAt:    now,
		UpdatedAt:    now,
	}}); err != nil {
		t.Fatalf("saveManagedInstances: %v", err)
	}

	_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
		"GET /api/v1/agents/picoclaw/status": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":           "picoclaw",
				"runtimeState": "running",
				"health":       "healthy",
				"updatedAt":    now,
			})
		},
		"GET /api/v1/agents/picoclaw/capabilities": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{})
		},
		"GET /api/base-agent/cron/jobs": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"jobs": []map[string]interface{}{}})
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/picoclaw/launcher", nil)
	handleWebUIAgent(rec, req, "req-launcher-fallback", daemon)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, needle := range []string{
		`"fallbackGroup":"openrouter:flash"`,
		`"aliasGroupSize":2`,
		`"primary":true`,
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected response to contain %s, got %s", needle, body)
		}
	}
}

func TestHandleAgentLauncherReturnsLastModelRunMetadata(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	storePath := filepath.Join(tmp, ".carrier", "instances.json")
	t.Setenv("CARRIER_INSTANCE_STORE", storePath)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := saveManagedInstances(storePath, []managedAgentInstance{{
		ID:         "picoclaw-prod",
		Type:       "picoclaw",
		AgentID:    "picoclaw",
		Isolation:  true,
		GatewayURL: "http://127.0.0.1:8787",
		Provider:   "openrouter",
		ModelRuntime: &managedAgentModelRuntime{
			RequestedAlias: "flash",
			RequestedModel: "deepseek/deepseek-chat-v3-0324",
			ResolvedModel:  "deepseek/deepseek-chat-v3-0324",
			FallbackGroup:  "openrouter:flash",
			OverrideHit:    true,
			FallbackHit:    true,
			LastRunAt:      now,
		},
		RuntimeState: "running",
		CreatedAt:    now,
		UpdatedAt:    now,
	}}); err != nil {
		t.Fatalf("saveManagedInstances: %v", err)
	}

	_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
		"GET /api/v1/agents/picoclaw/status": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":           "picoclaw",
				"runtimeState": "running",
				"health":       "healthy",
				"updatedAt":    now,
			})
		},
		"GET /api/v1/agents/picoclaw/capabilities": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{})
		},
		"GET /api/base-agent/cron/jobs": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"jobs": []map[string]interface{}{}})
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/picoclaw/launcher", nil)
	handleWebUIAgent(rec, req, "req-launcher-last-model", daemon)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, needle := range []string{
		`"lastModelRun":{`,
		`"requestedAlias":"flash"`,
		`"requestedModel":"deepseek/deepseek-chat-v3-0324"`,
		`"resolvedModel":"deepseek/deepseek-chat-v3-0324"`,
		`"fallbackGroup":"openrouter:flash"`,
		`"overrideHit":true`,
		`"fallbackHit":true`,
		`"lastRunAt":"`,
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected response to contain %s, got %s", needle, body)
		}
	}
}

func TestHandleAgentModelsReturnsStoredSurface(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	storePath := filepath.Join(tmp, ".carrier", "instances.json")
	t.Setenv("CARRIER_INSTANCE_STORE", storePath)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := saveManagedInstances(storePath, []managedAgentInstance{{
		ID:         "picoclaw-prod",
		Type:       "picoclaw",
		AgentID:    "picoclaw",
		ConfigPath: filepath.Join(tmp, ".picoclaw", "config.json"),
		ModelSurface: &managedAgentModelSurface{
			DefaultProfile: "openrouter-fast",
			Profiles: []managedAgentModelProfile{{
				ProfileName:    "openrouter-fast",
				ModelAlias:     "flash",
				ModelID:        "google/gemini-2.0-flash-001",
				ProviderID:     "openrouter",
				ProviderKey:    "openrouter",
				ProtocolFamily: "openai-compatible",
				BaseURL:        "https://openrouter.ai/api/v1",
				Primary:        true,
			}},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}}); err != nil {
		t.Fatalf("saveManagedInstances: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/picoclaw/models", nil)
	handleWebUIAgent(rec, req, "req-models-get", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, needle := range []string{
		`"agentId":"picoclaw"`,
		`"instanceId":"picoclaw-prod"`,
		`"configPath":"`,
		`"defaultProfile":"openrouter-fast"`,
		`"modelAlias":"flash"`,
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected response to contain %s, got %s", needle, body)
		}
	}
}

func TestHandleAgentModelsSyncsFromManagedConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	storePath := filepath.Join(tmp, ".carrier", "instances.json")
	t.Setenv("CARRIER_INSTANCE_STORE", storePath)

	cfgDir := filepath.Join(tmp, ".zeroclaw")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	configPath := filepath.Join(cfgDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(`
default_provider = "openrouter"
default_model = "google/gemini-2.0-flash-001"

[provider_profiles.openrouter_fast]
protocol_family = "openai-compatible"
provider = "openrouter"
provider_id = "openrouter"
model_alias = "flash"
model = "google/gemini-2.0-flash-001"
base_url = "https://openrouter.ai/api/v1"

[provider_profiles.openrouter_safe]
protocol_family = "openai-compatible"
provider = "openrouter"
provider_id = "openrouter"
model_alias = "flash"
model = "deepseek/deepseek-chat-v3-0324"
base_url = "https://openrouter.ai/api/v1"
`), 0o600); err != nil {
		t.Fatalf("write zeroclaw config: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := saveManagedInstances(storePath, []managedAgentInstance{{
		ID:         "zeroclaw-local",
		Type:       "zeroclaw",
		AgentID:    "zeroclaw",
		ConfigPath: configPath,
		ModelSurface: &managedAgentModelSurface{
			DefaultProfile: "stale",
			Profiles: []managedAgentModelProfile{{
				ProfileName: "stale",
				ModelAlias:  "stale",
				ModelID:     "old/model",
				Primary:     true,
			}},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}}); err != nil {
		t.Fatalf("saveManagedInstances: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/zeroclaw/models/sync", nil)
	handleWebUIAgent(rec, req, "req-models-sync", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, needle := range []string{
		`"synced":true`,
		`"defaultProfile":"openrouter_fast"`,
		`"modelId":"google/gemini-2.0-flash-001"`,
		`"modelId":"deepseek/deepseek-chat-v3-0324"`,
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected response to contain %s, got %s", needle, body)
		}
	}

	instances, _, err := loadManagedInstances()
	if err != nil {
		t.Fatalf("loadManagedInstances: %v", err)
	}
	idx := findManagedInstanceIndexByAgentID(instances, "zeroclaw")
	if idx < 0 {
		t.Fatal("expected synced managed instance")
	}
	if instances[idx].ModelSurface == nil || strings.TrimSpace(instances[idx].ModelSurface.DefaultProfile) != "openrouter_fast" {
		t.Fatalf("expected synced model surface, got %+v", instances[idx].ModelSurface)
	}
}
