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

func buildAuthStatusMux(t *testing.T, daemonHandlers map[string]http.HandlerFunc, setup *SetupStore) (http.Handler, *httptest.Server) {
	t.Helper()
	t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(t.TempDir(), "instances.json"))
	if setup == nil {
		setup = NewSetupStore()
	}
	srv := newMockDaemon(daemonHandlers)
	dc := NewDaemonClient(srv.URL, "test-token", 5*time.Second)
	sessions := NewSessionStore("", 0, nil)
	t.Cleanup(sessions.Stop)
	downloads := NewDownloadStore("", nil)
	rl := NewGatewayRateLimiter(100, 1000, time.Minute, nil)
	onboard := NewOnboardStore()
	cfg := &GatewayConfig{
		APIToken:            "test-gateway-token",
		MaxCommandBodyBytes: 64 * 1024,
		TelegramAPIBaseURL:  srv.URL,
	}
	return buildGatewayMux(cfg, dc, sessions, downloads, rl, onboard, setup), srv
}

func TestProviderAuthStatusAPI(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
	t.Setenv("OPENAI_API_KEY", "env-openai-key")
	if _, err := saveProviderCredential("openai-codex", "saved-codex-token"); err != nil {
		t.Fatalf("saveProviderCredential: %v", err)
	}

	mux, srv := buildAuthStatusMux(t, nil, nil)
	defer srv.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/providers", nil)
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Providers []ProviderAuthStatus `json:"providers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Providers) == 0 {
		t.Fatal("expected provider auth statuses")
	}

	byID := make(map[string]ProviderAuthStatus, len(resp.Providers))
	for _, provider := range resp.Providers {
		byID[provider.ID] = provider
	}

	openai := byID["openai"]
	if !openai.Configured || openai.Reusable {
		t.Fatalf("unexpected openai auth status: %+v", openai)
	}
	codex := byID["openai-codex"]
	if !codex.Configured || !codex.Reusable || !codex.HasSavedCredential {
		t.Fatalf("unexpected openai-codex auth status: %+v", codex)
	}
	ollama := byID["ollama"]
	if !ollama.Configured || !ollama.Reusable {
		t.Fatalf("unexpected ollama auth status: %+v", ollama)
	}
}

func TestChannelStatusAPI(t *testing.T) {
	setup := NewSetupStore()
	setup.Configure(ProviderDiscord, "discord-bot-token", "discord-public-key")

	mux, srv := buildAuthStatusMux(t, nil, setup)
	defer srv.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels", nil)
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Channels []ChannelStatus `json:"channels"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Channels) != 4 {
		t.Fatalf("expected 4 channel statuses, got %d", len(resp.Channels))
	}

	byID := make(map[string]ChannelStatus, len(resp.Channels))
	for _, channel := range resp.Channels {
		byID[channel.ID] = channel
	}
	if !byID["telegram"].SupportsPairing {
		t.Fatalf("expected telegram pairing support, got %+v", byID["telegram"])
	}
	if !byID["discord"].Configured || !byID["discord"].RequiresWebhookSecret {
		t.Fatalf("unexpected discord channel status: %+v", byID["discord"])
	}
	if byID["webui"].Configured || !byID["webui"].SupportsWebUI {
		t.Fatalf("unexpected webui channel status: %+v", byID["webui"])
	}
}

func TestOnboardReusesSharedAuthValidation(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CARRIER_CONFIG", filepath.Join(tmp, "config.v2.json"))
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))

	mux, srv := buildAuthStatusMux(t, map[string]http.HandlerFunc{
		"GET /api/v1/pairing/codes": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"codes":[]}`))
		},
		"POST /api/v1/pairing/codes": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"code":"pair-123","expiresAt":"2026-03-12T00:05:00Z"}`))
		},
	}, nil)
	defer srv.Close()

	t.Run("feishu secret requirement is shared between onboard and setup", func(t *testing.T) {
		onboardReq := httptest.NewRequest(http.MethodPost, "/api/v1/onboard", strings.NewReader(`{
			"channel":"feishu",
			"channelToken":"feishu-bot-token",
			"providerId":"openai",
			"providerToken":"sk-openai"
		}`))
		onboardReq.Header.Set("Authorization", "Bearer test-gateway-token")
		onboardReq.Header.Set("Content-Type", "application/json")
		onboardRec := httptest.NewRecorder()
		mux.ServeHTTP(onboardRec, onboardReq)
		if onboardRec.Code != http.StatusBadRequest {
			t.Fatalf("expected onboard 400, got %d body=%s", onboardRec.Code, onboardRec.Body.String())
		}
		if !strings.Contains(onboardRec.Body.String(), "Feishu requires channelSecret") {
			t.Fatalf("expected feishu secret validation in onboard, got %s", onboardRec.Body.String())
		}

		setupReq := httptest.NewRequest(http.MethodPost, "/api/v1/setup", strings.NewReader(`{
			"provider":"feishu",
			"token":"feishu-bot-token"
		}`))
		setupReq.Header.Set("Authorization", "Bearer test-gateway-token")
		setupReq.Header.Set("Content-Type", "application/json")
		setupRec := httptest.NewRecorder()
		mux.ServeHTTP(setupRec, setupReq)
		if setupRec.Code != http.StatusBadRequest {
			t.Fatalf("expected setup 400, got %d body=%s", setupRec.Code, setupRec.Body.String())
		}
		if !strings.Contains(setupRec.Body.String(), "Feishu requires webhook_secret") {
			t.Fatalf("expected feishu secret validation in setup, got %s", setupRec.Body.String())
		}
	})

	t.Run("successful onboard response carries shared auth status model", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/onboard", strings.NewReader(`{
			"channel":"telegram",
			"channelToken":"telegram-bot-token",
			"providerId":"openai",
			"providerToken":"sk-openai"
		}`))
		req.Header.Set("Authorization", "Bearer test-gateway-token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected onboard 200, got %d body=%s", rec.Code, rec.Body.String())
		}

		var resp struct {
			ProviderAuthStatus ProviderAuthStatus `json:"providerAuthStatus"`
			ChannelStatus      ChannelStatus      `json:"channelStatus"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode onboard response: %v", err)
		}
		if resp.ProviderAuthStatus.ID != "openai" || resp.ProviderAuthStatus.AuthMode != "api_key" {
			t.Fatalf("unexpected provider auth status: %+v", resp.ProviderAuthStatus)
		}
		if resp.ChannelStatus.ID != "telegram" || !resp.ChannelStatus.Configured {
			t.Fatalf("unexpected channel status: %+v", resp.ChannelStatus)
		}
	})
}
