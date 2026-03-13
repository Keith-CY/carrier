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
					ProfileName:      "openrouter-fast",
					ModelAlias:       "flash",
					ModelID:          "google/gemini-2.0-flash-001",
					ProviderID:       "openrouter",
					ProviderKey:      "openrouter",
					ProtocolFamily:   "openai-compatible",
					BaseURL:          "https://openrouter.ai/api/v1",
					AuthMethod:       "api_key",
					TimeoutMs:        45000,
					RetryBudget:      2,
					FallbackStrategy: "ordered",
					Primary:          true,
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
		"GET /api/v1/agents/picoclaw/subagents": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jobs": []map[string]any{
					{"jobId": "subagent-1", "task": "collect diagnostics", "status": "completed", "result": "done"},
				},
			})
		},
		"GET /api/v1/agents/picoclaw/sessions": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sessions": []map[string]any{
					{"key": "telegram:prod", "messageCount": 12, "summaryLength": 128, "updatedAt": now},
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
		`"subagent-1"`,
		`"telegram:prod"`,
		`"lastResult":"succeeded"`,
		`"defaultProfile":"openrouter-fast"`,
		`"modelAlias":"flash"`,
		`"modelId":"google/gemini-2.0-flash-001"`,
		`"protocolFamily":"openai-compatible"`,
		`"timeoutMs":45000`,
		`"retryBudget":2`,
		`"fallbackStrategy":"ordered"`,
		`"mediaRuntime":{"provider":"openrouter","status":"ready"`,
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected response to contain %s, got %s", needle, body)
		}
	}
}

func TestHandleAgentLauncherReturnsStructuredRemediations(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	storePath := filepath.Join(tmp, ".carrier", "instances.json")
	t.Setenv("CARRIER_INSTANCE_STORE", storePath)
	t.Setenv("CARRIER_TRANSCRIPTION_PROVIDER", "openrouter")

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := saveManagedInstances(storePath, []managedAgentInstance{{
		ID:           "agent-alpha",
		Type:         "zeroclaw",
		AgentID:      "agent-alpha",
		Provider:     "openrouter",
		RuntimeState: "stopped",
		CreatedAt:    now,
		UpdatedAt:    now,
	}}); err != nil {
		t.Fatalf("saveManagedInstances: %v", err)
	}

	_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
		"GET /api/v1/agents/agent-alpha/status": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":           "agent-alpha",
				"runtimeState": "stopped",
				"health":       "degraded",
				"heartbeat": map[string]interface{}{
					"state":          "stale",
					"ageSeconds":     240,
					"lastActivityAt": now,
				},
				"updatedAt": now,
			})
		},
		"GET /api/v1/agents/agent-alpha/capabilities": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"mcp": map[string]interface{}{
					"servers": []map[string]interface{}{
						{"name": "repo", "health": "degraded", "attached": false, "remediationHint": "Attach MCP before expecting tools to appear."},
					},
				},
			})
		},
		"GET /api/base-agent/cron/jobs": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jobs": []map[string]interface{}{
					{"id": "cron-1", "prompt": "check launcher", "paused": true, "lastResult": "paused"},
				},
			})
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-alpha/launcher", nil)
	handleWebUIAgent(rec, req, "req-launcher-remediation", daemon)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, needle := range []string{
		`"remediations":[`,
		`"category":"provider"`,
		`"category":"heartbeat"`,
		`"category":"cron"`,
		`"category":"mcp"`,
		`"detail":"provider=openrouter auth=api_key"`,
		`"detail":"state=stale age=240s"`,
		`"detail":"job=cron-1 last=paused"`,
		`"detail":"server=repo health=degraded"`,
		`"mediaRuntime":{"provider":"openrouter","status":"unavailable"`,
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
			RequestedAlias:    "flash",
			RequestedModel:    "deepseek/deepseek-chat-v3-0324",
			ResolvedModel:     "deepseek/deepseek-chat-v3-0324",
			ResolvedProfile:   "openrouter-safe",
			FallbackGroup:     "openrouter:flash",
			SelectionStrategy: "explicit_model",
			SelectionOrdinal:  1,
			OverrideHit:       true,
			FallbackHit:       true,
			LastRunAt:         now,
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
		`"resolvedProfile":"openrouter-safe"`,
		`"fallbackGroup":"openrouter:flash"`,
		`"selectionStrategy":"explicit_model"`,
		`"selectionOrdinal":1`,
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

func TestHandleAgentModelsDiscoverShowsDriftFromManagedConfig(t *testing.T) {
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
				ProviderID:  "openrouter",
				ProviderKey: "openrouter",
				Primary:     true,
			}},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}}); err != nil {
		t.Fatalf("saveManagedInstances: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/zeroclaw/models/discover", nil)
	handleWebUIAgent(rec, req, "req-models-discover", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, needle := range []string{
		`"agentId":"zeroclaw"`,
		`"driftState":"drifted"`,
		`"driftReason":"stored model surface differs from config-discovered model surface"`,
		`"modelSurface":{"defaultProfile":"stale"`,
		`"discoveredModelSurface":{"defaultProfile":"openrouter_fast"`,
		`"modelId":"google/gemini-2.0-flash-001"`,
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected response to contain %s, got %s", needle, body)
		}
	}
}

func TestHandleAgentModelsUpdatesDefaultProfile(t *testing.T) {
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
					ModelAlias:     "flash-safe",
					ModelID:        "deepseek/deepseek-chat-v3-0324",
					ProviderID:     "openrouter",
					ProviderKey:    "openrouter",
					ProtocolFamily: "openai-compatible",
				},
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}}); err != nil {
		t.Fatalf("saveManagedInstances: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/picoclaw/models/default", strings.NewReader(`{"profileName":"openrouter-safe"}`))
	handleWebUIAgent(rec, req, "req-models-default", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, needle := range []string{
		`"defaultProfile":"openrouter-safe"`,
		`"profileName":"openrouter-fast"`,
		`"profileName":"openrouter-safe"`,
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected response to contain %s, got %s", needle, body)
		}
	}

	instances, _, err := loadManagedInstances()
	if err != nil {
		t.Fatalf("loadManagedInstances: %v", err)
	}
	idx := findManagedInstanceIndexByAgentID(instances, "picoclaw")
	if idx < 0 {
		t.Fatal("expected updated managed instance")
	}
	if instances[idx].ModelSurface == nil || strings.TrimSpace(instances[idx].ModelSurface.DefaultProfile) != "openrouter-safe" {
		t.Fatalf("expected updated default profile, got %+v", instances[idx].ModelSurface)
	}
}

func TestHandleAgentModelsUpdatesProfileAndPersistsConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	storePath := filepath.Join(tmp, ".carrier", "instances.json")
	t.Setenv("CARRIER_INSTANCE_STORE", storePath)

	configDir := filepath.Join(tmp, ".picoclaw")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	configPath := filepath.Join(configDir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{
  "agents": {
    "defaults": {
      "workspace": "/tmp/workspace",
      "provider": "openrouter",
      "model": "openrouter-fast"
    }
  },
  "model_list": [
    {
      "model_name": "openrouter-fast",
      "model_alias": "flash",
      "model": "google/gemini-2.0-flash-001",
      "protocol_family": "openai-compatible"
    },
    {
      "model_name": "openrouter-safe",
      "model_alias": "flash-safe",
      "model": "deepseek/deepseek-chat-v3-0324",
      "protocol_family": "openai-compatible"
    }
  ],
  "provider_profiles": {
    "openrouter-fast": {
      "provider": "openrouter",
      "provider_id": "openrouter",
      "protocol_family": "openai-compatible",
      "model_alias": "flash",
      "model": "google/gemini-2.0-flash-001",
      "credential_ref": "openrouter"
    },
    "openrouter-safe": {
      "provider": "openrouter",
      "provider_id": "openrouter",
      "protocol_family": "openai-compatible",
      "model_alias": "flash-safe",
      "model": "deepseek/deepseek-chat-v3-0324",
      "credential_ref": "openrouter"
    }
  },
  "providers": {
    "openrouter": {
      "credential_ref": "openrouter"
    }
  }
}
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := saveManagedInstances(storePath, []managedAgentInstance{{
		ID:         "picoclaw-prod",
		Type:       "picoclaw",
		AgentID:    "picoclaw",
		ConfigPath: configPath,
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
					ProfileName:      "openrouter-safe",
					ModelAlias:       "flash-safe",
					ModelID:          "deepseek/deepseek-chat-v3-0324",
					ProviderID:       "openrouter",
					ProviderKey:      "openrouter",
					ProtocolFamily:   "openai-compatible",
					TimeoutMs:        45000,
					RetryBudget:      2,
					FallbackStrategy: "ordered",
				},
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}}); err != nil {
		t.Fatalf("saveManagedInstances: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/picoclaw/models/profile", strings.NewReader(`{
		"profileName":"openrouter-safe",
		"modelAlias":"flash-safe-v2",
		"modelId":"anthropic/claude-sonnet-4.6",
		"providerId":"anthropic",
		"baseUrl":"https://api.anthropic.com/v1",
		"authMethod":"api_key",
		"timeoutMs":60000,
		"retryBudget":4,
		"fallbackStrategy":"round_robin"
	}`))
	handleWebUIAgent(rec, req, "req-models-profile", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, needle := range []string{
		`"profileName":"openrouter-safe"`,
		`"modelAlias":"flash-safe-v2"`,
		`"modelId":"anthropic/claude-sonnet-4.6"`,
		`"providerId":"anthropic"`,
		`"timeoutMs":60000`,
		`"retryBudget":4`,
		`"fallbackStrategy":"round_robin"`,
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected response to contain %s, got %s", needle, body)
		}
	}

	instances, _, err := loadManagedInstances()
	if err != nil {
		t.Fatalf("loadManagedInstances: %v", err)
	}
	idx := findManagedInstanceIndexByAgentID(instances, "picoclaw")
	if idx < 0 {
		t.Fatal("expected updated managed instance")
	}
	gotProfile := instances[idx].ModelSurface.Profiles[1]
	if gotProfile.ModelAlias != "flash-safe-v2" || gotProfile.ModelID != "anthropic/claude-sonnet-4.6" || gotProfile.ProviderID != "anthropic" || gotProfile.TimeoutMs != 60000 || gotProfile.RetryBudget != 4 || gotProfile.FallbackStrategy != "round_robin" {
		t.Fatalf("unexpected updated profile: %+v", gotProfile)
	}

	updatedRaw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read updated config: %v", err)
	}
	updatedText := string(updatedRaw)
	for _, needle := range []string{
		`"model_alias": "flash-safe-v2"`,
		`"model": "anthropic/claude-sonnet-4.6"`,
		`"provider_id": "anthropic"`,
		`"base_url": "https://api.anthropic.com/v1"`,
		`"auth_method": "api_key"`,
		`"credential_ref": "openrouter"`,
	} {
		if !strings.Contains(updatedText, needle) {
			t.Fatalf("expected config to contain %s, got %s", needle, updatedText)
		}
	}
}
