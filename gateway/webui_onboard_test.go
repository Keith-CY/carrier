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

func TestWebUIOnboard_WebUIOnlyWithoutChannel(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CARRIER_CONFIG", filepath.Join(tmp, "config.v2.json"))
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")

	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	reqBody := `{
		"providerId":"openai",
		"providerToken":"sk-webui-only",
		"reuseCredential":false,
		"channel":"skip"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/onboard", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Result  string `json:"result"`
		Onboard struct {
			WebUIOnly bool `json:"webuiOnly"`
		} `json:"onboard"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Result != "ok" {
		t.Fatalf("result = %q, want ok", resp.Result)
	}
	if !resp.Onboard.WebUIOnly {
		t.Fatalf("webuiOnly = %v, want true", resp.Onboard.WebUIOnly)
	}

	cfgPath := filepath.Join(tmp, "config.v2.json")
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg struct {
		Channels     []map[string]any `json:"channels"`
		DefaultModel string           `json:"default_model"`
		ModelList    []struct {
			ProviderID string `json:"provider_id"`
		} `json:"model_list"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if len(cfg.Channels) != 0 {
		t.Fatalf("channels len = %d, want 0", len(cfg.Channels))
	}
	if cfg.DefaultModel != "openai-default" {
		t.Fatalf("default model = %q, want openai-default", cfg.DefaultModel)
	}
	if len(cfg.ModelList) != 1 {
		t.Fatalf("model list len = %d, want 1", len(cfg.ModelList))
	}
	if cfg.ModelList[0].ProviderID != "openai" {
		t.Fatalf("provider = %q, want openai", cfg.ModelList[0].ProviderID)
	}
}

func TestWebUIOnboard_DiscordIncludesPairStep(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CARRIER_CONFIG", filepath.Join(tmp, "config.v2.json"))
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")

	const pairCode = "pair-abcdef0123456789abcdef0123456789"
	pairExpires := time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339Nano)

	mux, srv, _ := buildTestMux(t, map[string]http.HandlerFunc{
		"GET /api/v1/pairing/codes": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"codes":[]}`))
		},
		"POST /api/v1/pairing/codes": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":      pairCode,
				"expiresAt": pairExpires,
			})
		},
	})
	defer srv.Close()

	reqBody := `{
		"providerId":"openai",
		"providerToken":"sk-discord",
		"reuseCredential":false,
		"channel":"discord",
		"channelToken":"discord-bot-token",
		"channelSecret":"discord-public-key"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/onboard", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Result  string `json:"result"`
		Onboard struct {
			Channel      string `json:"channel"`
			PairRequired bool   `json:"pairRequired"`
			PairCode     string `json:"pairCode"`
		} `json:"onboard"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Result != "ok" {
		t.Fatalf("result = %q, want ok", resp.Result)
	}
	if resp.Onboard.Channel != "discord" {
		t.Fatalf("channel = %q, want discord", resp.Onboard.Channel)
	}
	if !resp.Onboard.PairRequired {
		t.Fatalf("pairRequired = %v, want true", resp.Onboard.PairRequired)
	}
	if resp.Onboard.PairCode != pairCode {
		t.Fatalf("pairCode = %q, want %q", resp.Onboard.PairCode, pairCode)
	}

	cfgPath := filepath.Join(tmp, "config.v2.json")
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg struct {
		Channels []struct {
			ID            string `json:"id"`
			BotToken      string `json:"bot_token"`
			WebhookSecret string `json:"webhook_secret"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if len(cfg.Channels) != 1 {
		t.Fatalf("channels len = %d, want 1", len(cfg.Channels))
	}
	ch := cfg.Channels[0]
	if ch.ID != "discord" {
		t.Fatalf("channel id = %q, want discord", ch.ID)
	}
	if ch.BotToken != "discord-bot-token" {
		t.Fatalf("channel bot token mismatch")
	}
	if ch.WebhookSecret != "discord-public-key" {
		t.Fatalf("channel webhook secret mismatch")
	}
}

func TestWebUIOnboard_StatusEndpointsReflectUnifiedAuthState(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CARRIER_CONFIG", filepath.Join(tmp, "config.v2.json"))
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")

	mux, srv, _ := buildTestMux(t, map[string]http.HandlerFunc{
		"GET /api/v1/pairing/codes": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"codes":[]}`))
		},
		"POST /api/v1/pairing/codes": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":      "pair-status-check",
				"expiresAt": time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339Nano),
			})
		},
	})
	defer srv.Close()

	onboardReq := httptest.NewRequest(http.MethodPost, "/api/v1/onboard", strings.NewReader(`{
		"providerId":"openai",
		"providerToken":"sk-status-check",
		"reuseCredential":false,
		"channel":"discord",
		"channelToken":"discord-bot-token",
		"channelSecret":"discord-public-key"
	}`))
	onboardReq.Header.Set("Content-Type", "application/json")
	onboardReq.Header.Set("Authorization", "Bearer test-gateway-token")
	onboardRec := httptest.NewRecorder()
	mux.ServeHTTP(onboardRec, onboardReq)
	if onboardRec.Code != http.StatusOK {
		t.Fatalf("expected onboard 200, got %d, body=%s", onboardRec.Code, onboardRec.Body.String())
	}

	providersReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/providers", nil)
	providersReq.Header.Set("Authorization", "Bearer test-gateway-token")
	providersRec := httptest.NewRecorder()
	mux.ServeHTTP(providersRec, providersReq)
	if providersRec.Code != http.StatusOK {
		t.Fatalf("expected providers status 200, got %d", providersRec.Code)
	}
	var providerStatusResp struct {
		Providers []ProviderAuthStatus `json:"providers"`
	}
	if err := json.NewDecoder(providersRec.Body).Decode(&providerStatusResp); err != nil {
		t.Fatalf("decode provider auth status: %v", err)
	}
	foundOpenAI := false
	for _, status := range providerStatusResp.Providers {
		if status.ID == "openai" {
			foundOpenAI = true
			if !status.Configured {
				t.Fatalf("expected openai configured after onboarding")
			}
		}
	}
	if !foundOpenAI {
		t.Fatalf("expected openai in provider auth status response")
	}

	channelsReq := httptest.NewRequest(http.MethodGet, "/api/v1/channels", nil)
	channelsReq.Header.Set("Authorization", "Bearer test-gateway-token")
	channelsRec := httptest.NewRecorder()
	mux.ServeHTTP(channelsRec, channelsReq)
	if channelsRec.Code != http.StatusOK {
		t.Fatalf("expected channel status 200, got %d", channelsRec.Code)
	}
	var channelStatusResp struct {
		Channels []ChannelStatus `json:"channels"`
	}
	if err := json.NewDecoder(channelsRec.Body).Decode(&channelStatusResp); err != nil {
		t.Fatalf("decode channel status: %v", err)
	}
	foundDiscord := false
	for _, status := range channelStatusResp.Channels {
		if status.ID == "discord" {
			foundDiscord = true
			if !status.Configured {
				t.Fatalf("expected discord configured after onboarding")
			}
		}
	}
	if !foundDiscord {
		t.Fatalf("expected discord in channel status response")
	}
}
