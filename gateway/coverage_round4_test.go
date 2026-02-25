package gateway

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGatewayURLFromRequest(t *testing.T) {
	if got := gatewayURLFromRequest(nil); got != "" {
		t.Fatalf("gatewayURLFromRequest(nil) = %q, want empty", got)
	}

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/", nil)
	if got := gatewayURLFromRequest(req); got != "http://gateway.local" {
		t.Fatalf("gatewayURLFromRequest(http) = %q, want %q", got, "http://gateway.local")
	}

	req = httptest.NewRequest(http.MethodGet, "https://ignored/", nil)
	req.TLS = &tls.ConnectionState{}
	req.Host = "upstream.local:443"
	req.Header.Set("X-Forwarded-Proto", "HTTPS")
	if got := gatewayURLFromRequest(req); got != "https://upstream.local:443" {
		t.Fatalf("gatewayURLFromRequest(forwarded) = %q, want %q", got, "https://upstream.local:443")
	}

	t.Setenv("CARRIER_GATEWAY_HOST", "0.0.0.0")
	t.Setenv("CARRIER_GATEWAY_PORT", "9988")
	req = httptest.NewRequest(http.MethodGet, "http://ignored/", nil)
	req.Host = ""
	if got := gatewayURLFromRequest(req); got != "http://127.0.0.1:9988" {
		t.Fatalf("gatewayURLFromRequest(env fallback) = %q, want %q", got, "http://127.0.0.1:9988")
	}
}

func TestManagedTimestampHelpers(t *testing.T) {
	if _, ok := parseManagedInstanceTimestamp(""); ok {
		t.Fatal("expected empty timestamp to be invalid")
	}
	if _, ok := parseManagedInstanceTimestamp("not-a-time"); ok {
		t.Fatal("expected invalid timestamp to be rejected")
	}

	tsRaw := "2026-02-25T00:00:00Z"
	parsed, ok := parseManagedInstanceTimestamp(tsRaw)
	if !ok || parsed.IsZero() {
		t.Fatalf("expected valid timestamp parse, got ok=%v parsed=%v", ok, parsed)
	}

	if got := parseManagedTimestamp(""); !got.IsZero() {
		t.Fatalf("expected zero time for empty input, got %v", got)
	}
	if got := parseManagedTimestamp("bad"); !got.IsZero() {
		t.Fatalf("expected zero time for invalid input, got %v", got)
	}
	if got := parseManagedTimestamp(tsRaw); got.IsZero() {
		t.Fatal("expected valid parseManagedTimestamp result")
	}
}

func TestLatestManagedInstanceForAgent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instances.json")
	t.Setenv("CARRIER_INSTANCE_STORE", path)

	instances := []managedAgentInstance{
		{
			ID:        "old",
			Type:      "openclaw",
			AgentID:   "openclaw",
			UpdatedAt: "2026-02-25T00:00:00Z",
		},
		{
			ID:        "new",
			Type:      "openclaw",
			AgentID:   "openclaw",
			UpdatedAt: "2026-02-26T00:00:00Z",
		},
		{
			ID:        "other",
			Type:      "picoclaw",
			AgentID:   "picoclaw",
			UpdatedAt: "2026-02-27T00:00:00Z",
		},
	}
	if err := saveManagedInstances(path, instances); err != nil {
		t.Fatalf("saveManagedInstances: %v", err)
	}

	got, ok := latestManagedInstanceForAgent("openclaw")
	if !ok || got.ID != "new" {
		t.Fatalf("latestManagedInstanceForAgent(openclaw) = %+v ok=%v, want id=new", got, ok)
	}

	if _, ok := latestManagedInstanceForAgent("missing"); ok {
		t.Fatal("expected no managed instance for missing agent")
	}
}

func TestLatestManagedInstanceForAgent_InvalidTimestampsFallbackToLast(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instances.json")
	t.Setenv("CARRIER_INSTANCE_STORE", path)

	instances := []managedAgentInstance{
		{ID: "first", AgentID: "openclaw", UpdatedAt: "bad-time-1"},
		{ID: "second", AgentID: "openclaw", UpdatedAt: "bad-time-2"},
	}
	if err := saveManagedInstances(path, instances); err != nil {
		t.Fatalf("saveManagedInstances: %v", err)
	}

	got, ok := latestManagedInstanceForAgent("openclaw")
	if !ok || got.ID != "second" {
		t.Fatalf("latest fallback = %+v ok=%v, want id=second", got, ok)
	}
}

func TestProviderCompatibilityAndResolveWebUIAddProviderID(t *testing.T) {
	writeGatewayDefaultProviderConfig(t, "openai", "openai/gpt-5.1", "OPENAI_API_KEY")
	if got := resolveWebUIAddProviderID("openclaw"); got != "openai" {
		t.Fatalf("resolveWebUIAddProviderID(openclaw) = %q, want %q", got, "openai")
	}

	if providerCompatibleForManagedAgent("openclaw", nil) {
		t.Fatal("expected nil provider to be incompatible for openclaw")
	}

	t.Setenv("OPENAI_API_KEY", "sk-test")
	withoutEnvVar := &LLMProvider{ID: "custom-no-env", EnvVar: ""}
	if !providerCompatibleForManagedAgent("openclaw", withoutEnvVar) {
		t.Fatal("expected provider with empty EnvVar to be compatible when OPENAI_API_KEY is present")
	}

	if !providerCompatibleForManagedAgent("unknown-agent", nil) {
		t.Fatal("expected unknown agent to allow provider compatibility by default")
	}
}

func TestProviderPayloadHelpers(t *testing.T) {
	flat := flattenDiscordOptions([]interface{}{
		map[string]interface{}{"value": "top"},
		map[string]interface{}{
			"options": []interface{}{
				map[string]interface{}{"value": "nested-1"},
				map[string]interface{}{"options": []interface{}{map[string]interface{}{"value": "nested-2"}}},
			},
		},
	})
	if got := strings.Join(flat, ","); got != "top,nested-1,nested-2" {
		t.Fatalf("flattenDiscordOptions result = %q", got)
	}
	if got := flattenDiscordOptions("bad-type"); got != nil {
		t.Fatalf("expected nil for non-array options, got %#v", got)
	}

	if got := parseFeishuText(`{"text":"hello"}`); got != "hello" {
		t.Fatalf("parseFeishuText(json) = %q, want hello", got)
	}
	if got := parseFeishuText(`{"x":"y"}`); got != "" {
		t.Fatalf("parseFeishuText(json-without-text) = %q, want empty", got)
	}
	if got := parseFeishuText("plain-text"); got != "plain-text" {
		t.Fatalf("parseFeishuText(plain) = %q, want plain-text", got)
	}
	if got := parseFeishuText(map[string]interface{}{"text": "from-map"}); got != "from-map" {
		t.Fatalf("parseFeishuText(map) = %q, want from-map", got)
	}
}

func TestProcessBaseAgentChatPaths(t *testing.T) {
	t.Run("empty message ignored", func(t *testing.T) {
		_, dc, sessions, _, _ := setupTestEnv(t, nil)
		resp := processBaseAgentChat(context.Background(), "telegram", "100", "r1", "   ", dc, sessions, nil)
		if resp.Result != "ok" || !strings.Contains(resp.Message, "empty message ignored") {
			t.Fatalf("unexpected response: %+v", resp)
		}
	})

	t.Run("requires pairing session", func(t *testing.T) {
		_, dc, sessions, _, _ := setupTestEnv(t, nil)
		resp := processBaseAgentChat(context.Background(), "telegram", "101", "r2", "hello", dc, sessions, nil)
		if resp.Result != "error" || resp.ErrorCode != "E_SESSION_REQUIRED" {
			t.Fatalf("unexpected response: %+v", resp)
		}
	})

	t.Run("rate limited", func(t *testing.T) {
		_, dc, sessions, _, _ := setupTestEnv(t, nil)
		_ = pairAndGetSession(sessions, "telegram", "102")
		rl := NewGatewayRateLimiter(0, 100, time.Minute, nil)

		resp := processBaseAgentChat(context.Background(), "telegram", "102", "r3", "hello", dc, sessions, rl)
		if resp.Result != "error" || resp.ErrorCode != "E_RATE_LIMITED" {
			t.Fatalf("unexpected response: %+v", resp)
		}
	})

	t.Run("daemon error mapped", func(t *testing.T) {
		_, dc, sessions, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"POST /api/v1/base-agent/chat": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error": map[string]string{"code": "E_USAGE", "message": "invalid chat payload"},
				})
			},
		})
		_ = pairAndGetSession(sessions, "telegram", "103")

		resp := processBaseAgentChat(context.Background(), "telegram", "103", "r4", "hello", dc, sessions, nil)
		if resp.Result != "error" || resp.ErrorCode != "E_USAGE" {
			t.Fatalf("unexpected response: %+v", resp)
		}
	})

	t.Run("success with empty daemon message uses fallback", func(t *testing.T) {
		_, dc, sessions, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
			"POST /api/v1/base-agent/chat": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]string{"message": "   "})
			},
		})
		_ = pairAndGetSession(sessions, "telegram", "104")

		resp := processBaseAgentChat(context.Background(), "telegram", "104", "r5", "hello", dc, sessions, nil)
		if resp.Result != "ok" || resp.Message != "base agent completed with no output" {
			t.Fatalf("unexpected response: %+v", resp)
		}
	})
}

func TestIsDaemonAgentNotFound(t *testing.T) {
	if !isDaemonAgentNotFound(&DaemonClientError{Code: " E_AGENT_NOT_FOUND "}) {
		t.Fatal("expected E_AGENT_NOT_FOUND to be detected")
	}
	if isDaemonAgentNotFound(&DaemonClientError{Code: "E_USAGE"}) {
		t.Fatal("expected non-not-found code to be false")
	}
	if isDaemonAgentNotFound(nil) {
		t.Fatal("expected nil error to be false")
	}
}

func TestLatestManagedInstanceForAgentLoadError(t *testing.T) {
	badPath := filepath.Join(t.TempDir(), "instances.json")
	if err := os.WriteFile(badPath, []byte("{invalid"), 0o600); err != nil {
		t.Fatalf("write bad instance store: %v", err)
	}
	t.Setenv("CARRIER_INSTANCE_STORE", badPath)

	if _, ok := latestManagedInstanceForAgent("openclaw"); ok {
		t.Fatal("expected false when managed instance store cannot be parsed")
	}
}
