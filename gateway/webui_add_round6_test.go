package gateway

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func callHandleWebUIAdd(t *testing.T, daemon *DaemonClient, body string) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "http://gateway.local/api/v1/add", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handleWebUIAdd(rec, req, "req-webui-add", daemon)

	var payload map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &payload)
	return rec, payload
}

func TestHandleWebUIAdd_InputValidationBranches(t *testing.T) {
	rec, payload := callHandleWebUIAdd(t, nil, `{"agentId":`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid json, got %d: %s", rec.Code, rec.Body.String())
	}
	if payload["errorCode"] != "E_USAGE" {
		t.Fatalf("expected E_USAGE, got %#v", payload)
	}

	rec, payload = callHandleWebUIAdd(t, nil, `{"agentId":"   "}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing agentId, got %d: %s", rec.Code, rec.Body.String())
	}
	if payload["errorCode"] != "E_USAGE" {
		t.Fatalf("expected E_USAGE for missing agentId, got %#v", payload)
	}
}

func TestHandleWebUIAdd_NonManagedErrorBranches(t *testing.T) {
	t.Run("generate instance id failure", func(t *testing.T) {
		_, err := generateManagedInstanceIDWithEntropy("worker", func(_ []byte) (int, error) {
			return 0, errors.New("random source unavailable")
		})
		if err == nil {
			t.Fatal("expected random generation failure")
		}
	})

	t.Run("daemon install failure", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))
		_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"POST /api/v1/agents/worker/install": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":{"code":"E_COMMAND_FAILED","message":"install failed"}}`))
			},
		})

		rec, _ := callHandleWebUIAdd(t, daemon, `{"agentId":"worker","instanceId":"worker-fixed"}`)
		if rec.Code == http.StatusOK {
			t.Fatalf("expected non-200 on install failure, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("daemon start failure", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))
		_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"POST /api/v1/agents/worker/install": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
			},
			"POST /api/v1/agents/worker/start": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":{"code":"E_COMMAND_FAILED","message":"start failed"}}`))
			},
		})

		rec, _ := callHandleWebUIAdd(t, daemon, `{"agentId":"worker","instanceId":"worker-fixed"}`)
		if rec.Code == http.StatusOK {
			t.Fatalf("expected non-200 on start failure, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("state persistence failure", func(t *testing.T) {
		tmp := t.TempDir()
		badStore := filepath.Join(tmp, "instances.json")
		if err := os.MkdirAll(badStore, 0o700); err != nil {
			t.Fatalf("prepare bad instance store path: %v", err)
		}
		t.Setenv("CARRIER_INSTANCE_STORE", badStore)
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

		rec, payload := callHandleWebUIAdd(t, daemon, `{"agentId":"worker","instanceId":"worker-fixed"}`)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500 for state persistence failure, got %d: %s", rec.Code, rec.Body.String())
		}
		if payload["errorCode"] != "E_STATE_PERSISTENCE" {
			t.Fatalf("expected E_STATE_PERSISTENCE, got %#v", payload)
		}
	})
}

func TestHandleWebUIAdd_ManagedValidationAndCredentialBranches(t *testing.T) {
	t.Run("unsupported channel", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))
		_, daemon, _, _, _ := setupTestEnv(t, nil)

		rec, payload := callHandleWebUIAdd(t, daemon, `{"agentId":"openclaw","channel":"discord","channelToken":"x","providerId":"openai","providerToken":"k"}`)
		if rec.Code != http.StatusBadRequest || payload["errorCode"] != "E_USAGE" {
			t.Fatalf("expected unsupported channel E_USAGE, got %d %#v", rec.Code, payload)
		}
	})

	t.Run("missing channel token", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))
		t.Setenv("CARRIER_TELEGRAM_BOT_TOKEN", "")
		_, daemon, _, _, _ := setupTestEnv(t, nil)

		rec, payload := callHandleWebUIAdd(t, daemon, `{"agentId":"openclaw","channel":"telegram","providerId":"openai","providerToken":"k"}`)
		if rec.Code != http.StatusBadRequest || payload["errorCode"] != "E_USAGE" {
			t.Fatalf("expected channel token required E_USAGE, got %d %#v", rec.Code, payload)
		}
	})

	t.Run("invalid provider id", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))
		_, daemon, _, _, _ := setupTestEnv(t, nil)

		rec, payload := callHandleWebUIAdd(t, daemon, `{"agentId":"openclaw","channel":"telegram","channelToken":"tg","providerId":"unknown"}`)
		if rec.Code != http.StatusBadRequest || payload["errorCode"] != "E_PROVIDER_NOT_FOUND" {
			t.Fatalf("expected E_PROVIDER_NOT_FOUND, got %d %#v", rec.Code, payload)
		}
	})

	t.Run("provider credential missing", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))
		t.Setenv("OPENAI_API_KEY", "")
		_, daemon, _, _, _ := setupTestEnv(t, nil)

		rec, payload := callHandleWebUIAdd(t, daemon, `{"agentId":"openclaw","channel":"telegram","channelToken":"tg","providerId":"openai"}`)
		if rec.Code != http.StatusBadRequest || payload["errorCode"] != "E_AUTH_INPUT" {
			t.Fatalf("expected E_AUTH_INPUT, got %d %#v", rec.Code, payload)
		}
	})

	t.Run("reuse credential load failure", func(t *testing.T) {
		tmp := t.TempDir()
		storePath := filepath.Join(tmp, "credentials.json")
		if err := os.WriteFile(storePath, []byte("{broken-json"), 0o600); err != nil {
			t.Fatalf("write malformed credential store: %v", err)
		}
		t.Setenv("HOME", tmp)
		t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
		t.Setenv("CARRIER_CREDENTIAL_STORE", storePath)
		t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))
		_, daemon, _, _, _ := setupTestEnv(t, nil)

		rec, payload := callHandleWebUIAdd(t, daemon, `{"agentId":"openclaw","channel":"telegram","channelToken":"tg","providerId":"openai-codex","reuseCredential":true}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		if payload["errorCode"] != "E_AUTH_INPUT" {
			t.Fatalf("expected E_AUTH_INPUT on credential read failure, got %#v", payload)
		}
	})

	t.Run("required env key fallback failure", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))
		t.Setenv("OPENAI_COMPATIBLE_API_KEY", "")
		_, daemon, _, _, _ := setupTestEnv(t, nil)

		rec, payload := callHandleWebUIAdd(t, daemon, `{"agentId":"openclaw","channel":"telegram","channelToken":"tg","providerId":"openai-compatible"}`)
		if rec.Code != http.StatusBadRequest || payload["errorCode"] != "E_AUTH_INPUT" {
			t.Fatalf("expected required env E_AUTH_INPUT, got %d %#v", rec.Code, payload)
		}
	})

	t.Run("invalid prefetched telegram chat id", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))
		_, daemon, _, _, _ := setupTestEnv(t, nil)

		rec, payload := callHandleWebUIAdd(t, daemon, `{"agentId":"picoclaw","channel":"telegram","channelToken":"tg","providerId":"openai","providerToken":"sk","channelChatId":"abc"}`)
		if rec.Code != http.StatusBadRequest || payload["errorCode"] != "E_USAGE" {
			t.Fatalf("expected invalid chat id E_USAGE, got %d %#v", rec.Code, payload)
		}
	})
}

func TestHandleWebUIAdd_ManagedEnvAndDaemonFailureBranches(t *testing.T) {
	t.Run("apply env vars failure", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))
		_, daemon, _, _, _ := setupTestEnv(t, nil)

		body := `{"agentId":"openclaw","channel":"telegram","channelToken":"tg","providerId":"openai","providerToken":"sk","envVars":{"BAD=KEY":"x"}}`
		rec, payload := callHandleWebUIAdd(t, daemon, body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
		if payload["errorCode"] != "E_ENV" {
			t.Fatalf("expected E_ENV, got %#v", payload)
		}
	})

	t.Run("managed install failure", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))
		_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"POST /api/v1/agents/openclaw/install": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":{"code":"E_COMMAND_FAILED","message":"install failed"}}`))
			},
		})

		body := `{"agentId":"openclaw","channel":"telegram","channelToken":"tg","providerId":"openai","providerToken":"sk"}`
		rec, _ := callHandleWebUIAdd(t, daemon, body)
		if rec.Code == http.StatusOK {
			t.Fatalf("expected non-200 on managed install failure, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("managed start failure", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))
		_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"POST /api/v1/agents/openclaw/install": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
			},
			"POST /api/v1/agents/openclaw/start": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":{"code":"E_COMMAND_FAILED","message":"start failed"}}`))
			},
		})

		body := `{"agentId":"openclaw","channel":"telegram","channelToken":"tg","providerId":"openai","providerToken":"sk"}`
		rec, _ := callHandleWebUIAdd(t, daemon, body)
		if rec.Code == http.StatusOK {
			t.Fatalf("expected non-200 on managed start failure, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
