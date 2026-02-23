package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func buildTestMux(t *testing.T, daemonHandlers map[string]http.HandlerFunc) (http.Handler, *httptest.Server, *SessionStore) {
	t.Helper()
	t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(t.TempDir(), "instances.json"))
	srv := newMockDaemon(daemonHandlers)
	dc := NewDaemonClient(srv.URL, "test-token", 5*time.Second)
	sessions := NewSessionStore("", 0, nil)
	t.Cleanup(sessions.Stop)
	downloads := NewDownloadStore("", nil)
	rl := NewGatewayRateLimiter(100, 1000, 1*time.Minute, nil)
	onboard := NewOnboardStore()
	setup := NewSetupStore()
	cfg := &GatewayConfig{
		APIToken:            "test-gateway-token",
		MaxCommandBodyBytes: 64 * 1024,
		TelegramAPIBaseURL:  srv.URL,
	}
	mux := buildGatewayMux(cfg, dc, sessions, downloads, rl, onboard, setup)
	return mux, srv, sessions
}

func TestHealthz(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %q", body["status"])
	}
}

func TestCommand_MethodNotAllowed(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/command", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestCommand_NoToken(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	req := httptest.NewRequest("POST", "/command", strings.NewReader(`{"input":"telegram 123 r1 /pair abc"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestCommand_WrongToken(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	req := httptest.NewRequest("POST", "/command", strings.NewReader(`{"input":"telegram 123 r1 /pair abc"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestCommand_EmptyInput(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	req := httptest.NewRequest("POST", "/command", strings.NewReader(`{"input":""}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCommand_PairSuccess(t *testing.T) {
	mux, srv, _ := buildTestMux(t, map[string]http.HandlerFunc{
		"POST /api/v1/pairing/verify-consume": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"code":"abc","consumed":true}`)
		},
	})
	defer srv.Close()

	req := httptest.NewRequest("POST", "/command", strings.NewReader(`{"input":"telegram 123 r1 /pair abc"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp GatewayResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Result != "ok" {
		t.Errorf("expected ok, got %s: %s", resp.Result, resp.Message)
	}
}

func TestCommand_PlainTextBody(t *testing.T) {
	mux, srv, _ := buildTestMux(t, map[string]http.HandlerFunc{
		"POST /api/v1/pairing/verify-consume": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"code":"abc","consumed":true}`)
		},
	})
	defer srv.Close()

	req := httptest.NewRequest("POST", "/command", strings.NewReader("telegram 123 r1 /pair abc"))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestCommand_AuthRequired_NoPair(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	req := httptest.NewRequest("POST", "/command", strings.NewReader(`{"input":"telegram 123 r1 /agents"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestCommand_WithSessionToken(t *testing.T) {
	mux, srv, sessions := buildTestMux(t, map[string]http.HandlerFunc{
		"GET /api/v1/agents": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"agents": []interface{}{}})
		},
	})
	defer srv.Close()

	tok := pairAndGetSession(sessions, "telegram", "123")

	req := httptest.NewRequest("POST", "/command", strings.NewReader(
		fmt.Sprintf(`{"input":"telegram 123 r1 /agents","sessionToken":"%s"}`, tok)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestCommand_TokenQueryParam(t *testing.T) {
	mux, srv, _ := buildTestMux(t, map[string]http.HandlerFunc{
		"POST /api/v1/pairing/verify-consume": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{}`)
		},
	})
	defer srv.Close()

	req := httptest.NewRequest("POST", "/command?token=test-gateway-token", strings.NewReader(`{"input":"telegram 123 r1 /pair abc"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSetup_Post(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	req := httptest.NewRequest("POST", "/api/v1/setup", strings.NewReader(`{"provider":"telegram","token":"tok123"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSetup_Get(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/api/v1/setup", nil)
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSetup_MethodNotAllowed(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	req := httptest.NewRequest("DELETE", "/api/v1/setup", nil)
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestSetup_InvalidProvider(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	req := httptest.NewRequest("POST", "/api/v1/setup", strings.NewReader(`{"provider":"invalid"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSetup_MissingProvider(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	req := httptest.NewRequest("POST", "/api/v1/setup", strings.NewReader(`{"token":"tok"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSetup_LegacyAlias(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/setup", nil)
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestTelegramTransportStatusEndpoint(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	setTelegramTransportStatus("auto", "polling", telegramFallbackWebhookSetupFailed, "setWebhook failed: timeout", "Check webhook URL reachability.")

	req := httptest.NewRequest("GET", "/api/v1/telegram/transport", nil)
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"selected_mode":"polling"`) {
		t.Fatalf("expected polling mode in response, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"reason_code":"WEBHOOK_SETUP_FAILED"`) {
		t.Fatalf("expected reason code in response, got %s", w.Body.String())
	}
}

func TestSSELogs_NoToken(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/api/v1/logs/stream?agent=a1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestSSELogs_MissingAgent(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/api/v1/logs/stream", nil)
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSSELogs_MethodNotAllowed(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	req := httptest.NewRequest("POST", "/api/v1/logs/stream?agent=a1", nil)
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestDownloads_MethodNotAllowed(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	req := httptest.NewRequest("POST", "/downloads/something", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestDownloads_InvalidPath(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/downloads/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Either 400 or some error since path is invalid
	if w.Code == http.StatusOK {
		t.Fatal("expected non-200 for invalid download path")
	}
}

func TestWebhook_MethodNotAllowed(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	for _, path := range []string{"/webhook/telegram", "/webhook/discord", "/webhook/feishu"} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: expected 405, got %d", path, w.Code)
		}
	}
}

func TestRequestIDMiddleware(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	// With custom request ID
	req := httptest.NewRequest("GET", "/healthz", nil)
	req.Header.Set("X-Request-Id", "my-request-id")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Header().Get("X-Request-Id") != "my-request-id" {
		t.Errorf("expected X-Request-Id to be echoed, got %q", w.Header().Get("X-Request-Id"))
	}

	// Without request ID (auto-generated)
	req2 := httptest.NewRequest("GET", "/healthz", nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	if w2.Header().Get("X-Request-Id") == "" {
		t.Error("expected auto-generated X-Request-Id")
	}
}

func TestRequestIDMiddleware_StripsControlChars(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/healthz", nil)
	req.Header.Set("X-Request-Id", "id\x00\x1f\x7ftest")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Header().Get("X-Request-Id") != "idtest" {
		t.Errorf("expected control chars stripped, got %q", w.Header().Get("X-Request-Id"))
	}
}

func TestCheckGatewayToken(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		auth     string
		wantErr  bool
	}{
		{"no token required", "", "", false},
		{"valid token", "secret", "Bearer secret", false},
		{"missing token", "secret", "", true},
		{"wrong token", "secret", "Bearer wrong", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tc.auth != "" {
				req.Header.Set("Authorization", tc.auth)
			}
			err := checkGatewayToken(req, tc.expected)
			if tc.wantErr && err == nil {
				t.Error("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %s", err.msg)
			}
		})
	}
}

func TestIsLoopbackGateway(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"", true},
		{"localhost", true},
		{"127.0.0.1", true},
		{"::1", true},
		{"0.0.0.0", false},
		{"192.168.1.1", false},
	}
	for _, tc := range tests {
		got := isLoopbackGateway(tc.host)
		if got != tc.want {
			t.Errorf("isLoopbackGateway(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

func TestValidateCommandAuth(t *testing.T) {
	sessions := NewSessionStore("", 0, nil)
	t.Cleanup(sessions.Stop)
	tok := pairAndGetSession(sessions, "telegram", "123")

	tests := []struct {
		name         string
		input        string
		sessionToken string
		wantNil      bool
	}{
		{"pair needs no auth", "telegram 123 r1 /pair abc", "", true},
		{"valid auth", "telegram 123 r1 /agents", tok, true},
		{"no session", "telegram 999 r1 /agents", tok, false},
		{"no token", "telegram 123 r1 /agents", "", false},
		{"wrong token", "telegram 123 r1 /agents", "bad", false},
		{"too few fields", "telegram 123", "", true}, // let parser handle
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := validateCommandAuth(tc.input, tc.sessionToken, sessions)
			if tc.wantNil && result != nil {
				t.Errorf("expected nil, got %v", result)
			}
			if !tc.wantNil && result == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestReadBodyWithLimit(t *testing.T) {
	body := strings.Repeat("a", 100)
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	data, err := readBodyWithLimit(req, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != 100 {
		t.Errorf("expected 100 bytes, got %d", len(data))
	}

	// Over limit
	bigBody := strings.Repeat("b", 300)
	req2 := httptest.NewRequest("POST", "/", strings.NewReader(bigBody))
	_, err = readBodyWithLimit(req2, 200)
	if err == nil {
		t.Error("expected error for body exceeding limit")
	}
}

func TestReadBodyWithLimit_ContentLengthCheck(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader("small"))
	req.Header.Set("Content-Length", "999999")
	_, err := readBodyWithLimit(req, 100)
	if err == nil {
		t.Error("expected error for Content-Length exceeding limit")
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusCreated, map[string]string{"key": "value"})
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
		t.Error("expected application/json content type")
	}
	body, err := io.ReadAll(w.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), `"key":"value"`) {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestNewGatewayHTTPServer(t *testing.T) {
	handler := http.NewServeMux()
	srv := newGatewayHTTPServer(":0", handler)
	if srv.ReadHeaderTimeout != gatewayReadHeaderTimeout {
		t.Errorf("unexpected ReadHeaderTimeout: %v", srv.ReadHeaderTimeout)
	}
	if srv.WriteTimeout != gatewayWriteTimeout {
		t.Errorf("unexpected WriteTimeout: %v", srv.WriteTimeout)
	}
	if srv.IdleTimeout != gatewayIdleTimeout {
		t.Errorf("unexpected IdleTimeout: %v", srv.IdleTimeout)
	}
}

// --- /api/v1/providers ---

func TestProvidersEndpoint_OK(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/api/v1/providers", nil)
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["result"] != "ok" {
		t.Errorf("expected result ok, got %q", body["result"])
	}
	providers, ok := body["providers"].([]interface{})
	if !ok || len(providers) == 0 {
		t.Errorf("expected non-empty providers array, got %v", body["providers"])
	}
	byCategory, ok := body["by_category"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected by_category map, got %T", body["by_category"])
	}
	for _, cat := range []string{"builtin", "custom", "local"} {
		if _, ok := byCategory[cat]; !ok {
			t.Errorf("missing category %q in by_category", cat)
		}
	}
}

func TestProvidersEndpoint_Unauthorized(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/api/v1/providers", nil)
	// No auth header
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestProvidersEndpoint_MethodNotAllowed(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	req := httptest.NewRequest("POST", "/api/v1/providers", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestProvidersEndpoint_ContainsAnthropicAndVLLM(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/api/v1/providers", nil)
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "anthropic") {
		t.Errorf("expected 'anthropic' in response body")
	}
	if !strings.Contains(body, "vllm") {
		t.Errorf("expected 'vllm' in response body")
	}
	if !strings.Contains(body, "api_key") {
		t.Errorf("expected 'api_key' auth mode in response body")
	}
	if !strings.Contains(body, "none") {
		t.Errorf("expected 'none' auth mode in response body")
	}
}

func TestProvidersEndpoint_CarrierDefaultProviderReusable(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
	writeGatewayDefaultProviderConfig(t, "openai-codex", "openai-codex/gpt-5.3-codex", "OPENAI_CODEX_TOKEN")
	if _, err := saveProviderCredential("openai-codex", "codex-token-test"); err != nil {
		t.Fatalf("saveProviderCredential: %v", err)
	}

	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/api/v1/providers", nil)
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	defaultProvider, ok := body["carrier_default_provider"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected carrier_default_provider map, got %T", body["carrier_default_provider"])
	}
	if configured, _ := defaultProvider["configured"].(bool); !configured {
		t.Fatalf("expected configured=true, got %#v", defaultProvider)
	}
	if reusable, _ := defaultProvider["reusable"].(bool); !reusable {
		t.Fatalf("expected reusable=true, got %#v", defaultProvider)
	}
	if id := fmt.Sprintf("%v", defaultProvider["id"]); id != "openai-codex" {
		t.Fatalf("expected default id openai-codex, got %s", id)
	}
}

func TestAgentsEndpoint_ListSuccess(t *testing.T) {
	mux, srv, _ := buildTestMux(t, map[string]http.HandlerFunc{
		"GET /api/v1/agents": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"agents":[{"id":"openclaw","runtimeState":"running"}]}`)
		},
	})
	defer srv.Close()

	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "openclaw") {
		t.Fatalf("expected openclaw in response, got %s", w.Body.String())
	}
}

func TestAgentsEndpoint_StatusSuccess(t *testing.T) {
	mux, srv, _ := buildTestMux(t, map[string]http.HandlerFunc{
		"GET /api/v1/agents/openclaw/status": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"statuses":[{"id":"openclaw","runtimeState":"running"}]}`)
		},
	})
	defer srv.Close()

	req := httptest.NewRequest("GET", "/api/v1/agents/openclaw/status", nil)
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "openclaw") {
		t.Fatalf("expected openclaw in response, got %s", w.Body.String())
	}
}

func TestAgentsEndpoint_StartActionSuccess(t *testing.T) {
	mux, srv, _ := buildTestMux(t, map[string]http.HandlerFunc{
		"POST /api/v1/agents/openclaw/start": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{}`)
		},
	})
	defer srv.Close()

	req := httptest.NewRequest("POST", "/api/v1/agents/openclaw/start", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"action":"start"`) {
		t.Fatalf("expected action=start in response, got %s", w.Body.String())
	}
}

func TestAgentsEndpoint_StartActionPersistenceFailureReturnsPartialSuccess(t *testing.T) {
	mux, srv, _ := buildTestMux(t, map[string]http.HandlerFunc{
		"POST /api/v1/agents/openclaw/start": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{}`)
		},
	})
	defer srv.Close()

	tmp := t.TempDir()
	badStore := filepath.Join(tmp, "instances.json")
	if err := os.MkdirAll(badStore, 0o700); err != nil {
		t.Fatalf("prepare bad instance store path: %v", err)
	}
	t.Setenv("CARRIER_INSTANCE_STORE", badStore)

	req := httptest.NewRequest("POST", "/api/v1/agents/openclaw/start", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"errorCode":"E_STATE_PERSISTENCE"`) {
		t.Fatalf("expected E_STATE_PERSISTENCE, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"partialSuccess":true`) {
		t.Fatalf("expected partialSuccess=true, got %s", w.Body.String())
	}
}

func TestInstancesEndpoint_ListSuccess(t *testing.T) {
	mux, srv, _ := buildTestMux(t, map[string]http.HandlerFunc{
		"GET /api/v1/agents/picoclaw/status": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"statuses":[{"id":"picoclaw","runtimeState":"running"}]}`)
		},
	})
	defer srv.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := upsertManagedInstance(managedAgentInstance{
		ID:           "picoclaw-abc12345",
		Type:         "picoclaw",
		AgentID:      "picoclaw",
		GatewayURL:   "http://127.0.0.1:8787",
		RuntimeState: "running",
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("upsertManagedInstance: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/instances", nil)
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "picoclaw-abc12345") {
		t.Fatalf("expected instance id in response, got %s", w.Body.String())
	}
}

func TestInstancesEndpoint_StopSuccess(t *testing.T) {
	mux, srv, _ := buildTestMux(t, map[string]http.HandlerFunc{
		"POST /api/v1/agents/picoclaw/stop": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{}`)
		},
	})
	defer srv.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := upsertManagedInstance(managedAgentInstance{
		ID:           "picoclaw-xyz98765",
		Type:         "picoclaw",
		AgentID:      "picoclaw",
		GatewayURL:   "http://127.0.0.1:8787",
		RuntimeState: "running",
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("upsertManagedInstance: %v", err)
	}

	req := httptest.NewRequest("POST", "/api/v1/instances/picoclaw-xyz98765/stop", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	instances, _, err := loadManagedInstances()
	if err != nil {
		t.Fatalf("loadManagedInstances: %v", err)
	}
	if len(instances) != 1 || instances[0].RuntimeState != "stopped" {
		t.Fatalf("expected stopped instance persisted, got %+v", instances)
	}
}

func TestInstancesEndpoint_BackfillFromDaemonInstalledAgent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))

	mux, srv, _ := buildTestMux(t, map[string]http.HandlerFunc{
		"GET /api/v1/agents": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"agents":[{"id":"picoclaw","installState":"installed","runtimeState":"stopped"}]}`)
		},
		"GET /api/v1/agents/picoclaw/status": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"statuses":[{"id":"picoclaw","runtimeState":"stopped"}]}`)
		},
	})
	defer srv.Close()

	req := httptest.NewRequest("GET", "/api/v1/instances", nil)
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "picoclaw-default") {
		t.Fatalf("expected backfilled instance id in response, got %s", w.Body.String())
	}
	instances, _, err := loadManagedInstances()
	if err != nil {
		t.Fatalf("loadManagedInstances: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected 1 backfilled instance, got %d", len(instances))
	}
	if instances[0].AgentID != "picoclaw" {
		t.Fatalf("expected backfilled agentID=picoclaw, got %+v", instances[0])
	}
}

func TestAgentsEndpoint_InstallCreatesManagedInstance(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))

	mux, srv, _ := buildTestMux(t, map[string]http.HandlerFunc{
		"POST /api/v1/agents/openclaw/install": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{}`)
		},
	})
	defer srv.Close()

	req := httptest.NewRequest("POST", "/api/v1/agents/openclaw/install", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	instances, _, err := loadManagedInstances()
	if err != nil {
		t.Fatalf("loadManagedInstances: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected 1 managed instance, got %d", len(instances))
	}
	if instances[0].AgentID != "openclaw" {
		t.Fatalf("expected managed instance for openclaw, got %+v", instances[0])
	}
}

// --- /api/v1/add ---

func TestAddEndpoint_MethodNotAllowed(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/api/v1/add", nil)
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestAddEndpoint_ManagedAgentSuccess_OpenAndZeroClaw(t *testing.T) {
	cases := []struct {
		name         string
		agentID      string
		requestEnv   map[string]string
		expectedEnvs map[string]string
	}{
		{
			name:    "openclaw",
			agentID: "openclaw",
			requestEnv: map[string]string{
				"OPENCLAW_MODE": "managed",
			},
			expectedEnvs: map[string]string{
				"OPENCLAW_MODE":  "managed",
				"OPENAI_API_KEY": "sk-provider-token",
			},
		},
		{
			name:    "zeroclaw",
			agentID: "zeroclaw",
			requestEnv: map[string]string{
				"ZEROCLAW_REGION": "cn",
			},
			expectedEnvs: map[string]string{
				"ZEROCLAW_REGION":  "cn",
				"OPENAI_API_KEY":   "sk-provider-token",
				"ZEROCLAW_API_KEY": "sk-provider-token",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("HOME", tmp)
			t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))
			t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
			t.Setenv("CARRIER_TELEGRAM_BOT_TOKEN", "tg-token")

			installRoute := fmt.Sprintf("POST /api/v1/agents/%s/install", tc.agentID)
			startRoute := fmt.Sprintf("POST /api/v1/agents/%s/start", tc.agentID)
			installCalls := 0
			startCalls := 0

			mux, srv, _ := buildTestMux(t, map[string]http.HandlerFunc{
				installRoute: func(w http.ResponseWriter, r *http.Request) {
					installCalls++
					w.WriteHeader(http.StatusOK)
					fmt.Fprint(w, `{"status":"installed"}`)
				},
				startRoute: func(w http.ResponseWriter, r *http.Request) {
					startCalls++
					w.WriteHeader(http.StatusOK)
					fmt.Fprint(w, `{"status":"running"}`)
				},
			})
			defer srv.Close()

			for key := range tc.expectedEnvs {
				t.Setenv(key, "")
			}

			bodyMap := map[string]interface{}{
				"agentId":       tc.agentID,
				"channel":       "telegram",
				"providerId":    "openai",
				"providerToken": "sk-provider-token",
				"envVars":       tc.requestEnv,
			}
			bodyBytes, err := json.Marshal(bodyMap)
			if err != nil {
				t.Fatalf("marshal request body: %v", err)
			}

			req := httptest.NewRequest("POST", "/api/v1/add", strings.NewReader(string(bodyBytes)))
			req.Header.Set("Authorization", "Bearer test-gateway-token")
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}

			var resp map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("parse response: %v", err)
			}
			if got := strings.TrimSpace(fmt.Sprintf("%v", resp["result"])); got != "ok" {
				t.Fatalf("expected result=ok, got %q", got)
			}
			if got := strings.TrimSpace(fmt.Sprintf("%v", resp["agentId"])); got != tc.agentID {
				t.Fatalf("expected agentId=%s, got %q", tc.agentID, got)
			}
			instanceID := strings.TrimSpace(fmt.Sprintf("%v", resp["instanceId"]))
			if instanceID == "" {
				t.Fatalf("expected non-empty instanceId, got %#v", resp["instanceId"])
			}

			envKeysRaw, ok := resp["envKeys"].([]interface{})
			if !ok {
				t.Fatalf("expected envKeys array, got %#v", resp["envKeys"])
			}
			envKeySet := map[string]bool{}
			for _, item := range envKeysRaw {
				envKeySet[strings.TrimSpace(fmt.Sprintf("%v", item))] = true
			}
			for key, value := range tc.expectedEnvs {
				if !envKeySet[key] {
					t.Fatalf("expected envKeys to include %s, got %#v", key, envKeysRaw)
				}
				if got := os.Getenv(key); got != value {
					t.Fatalf("expected env var %s=%q, got %q", key, value, got)
				}
			}

			instances, _, err := loadManagedInstances()
			if err != nil {
				t.Fatalf("loadManagedInstances: %v", err)
			}
			if len(instances) != 1 {
				t.Fatalf("expected 1 managed instance, got %d", len(instances))
			}
			inst := instances[0]
			if inst.ID != instanceID {
				t.Fatalf("instance id mismatch: store=%s response=%s", inst.ID, instanceID)
			}
			if inst.Type != tc.agentID || inst.AgentID != tc.agentID {
				t.Fatalf("expected managed instance type/agent=%s, got %+v", tc.agentID, inst)
			}
			if strings.TrimSpace(inst.Channel) != "telegram" {
				t.Fatalf("expected channel=telegram, got %+v", inst)
			}
			if strings.TrimSpace(inst.Provider) != "openai" {
				t.Fatalf("expected provider=openai, got %+v", inst)
			}
			if inst.RuntimeState != "running" {
				t.Fatalf("expected runtime_state=running, got %+v", inst)
			}
			if strings.TrimSpace(inst.Workspace) == "" || strings.TrimSpace(inst.ConfigPath) == "" || strings.TrimSpace(inst.RecordPath) == "" {
				t.Fatalf("expected persisted paths in managed instance, got %+v", inst)
			}
			if strings.TrimSpace(inst.CreatedAt) == "" || strings.TrimSpace(inst.UpdatedAt) == "" {
				t.Fatalf("expected timestamps in managed instance, got %+v", inst)
			}
			if installCalls != 1 || startCalls != 1 {
				t.Fatalf("expected install/start calls = 1/1, got %d/%d", installCalls, startCalls)
			}
		})
	}
}

func TestAddEndpoint_OpenClawSuccess_AutoSelectsProviderAndChannel(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
	t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))
	writeGatewayDefaultProviderConfig(t, "openai-codex", "openai-codex/gpt-5.3-codex", "OPENAI_CODEX_TOKEN")
	t.Setenv("CARRIER_TELEGRAM_BOT_TOKEN", "tg-auto-token")

	if _, err := saveProviderCredential("openai-codex", "codex-auto-token"); err != nil {
		t.Fatalf("saveProviderCredential: %v", err)
	}

	mux, srv, _ := buildTestMux(t, map[string]http.HandlerFunc{
		"POST /api/v1/agents/openclaw/install": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status":"installed"}`)
		},
		"POST /api/v1/agents/openclaw/start": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status":"running"}`)
		},
	})
	defer srv.Close()

	req := httptest.NewRequest("POST", "/api/v1/add", strings.NewReader(`{"agentId":"openclaw","envVars":{"OPENCLAW_MODE":"managed"}}`))
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse add response: %v", err)
	}
	if got := strings.TrimSpace(fmt.Sprintf("%v", resp["result"])); got != "ok" {
		t.Fatalf("expected result=ok, got %q", got)
	}

	envKeysRaw, ok := resp["envKeys"].([]interface{})
	if !ok {
		t.Fatalf("expected envKeys array, got %#v", resp["envKeys"])
	}
	envKeySet := map[string]bool{}
	for _, item := range envKeysRaw {
		envKeySet[strings.TrimSpace(fmt.Sprintf("%v", item))] = true
	}
	for _, key := range []string{"OPENCLAW_MODE", "OPENAI_CODEX_TOKEN", "OPENAI_API_KEY"} {
		if !envKeySet[key] {
			t.Fatalf("expected envKeys to include %s, got %#v", key, envKeysRaw)
		}
	}

	instances, _, err := loadManagedInstances()
	if err != nil {
		t.Fatalf("loadManagedInstances: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected 1 managed instance, got %d", len(instances))
	}
	inst := instances[0]
	if inst.Provider != "openai-codex" {
		t.Fatalf("expected provider=openai-codex, got %+v", inst)
	}
	if inst.Channel != "telegram" {
		t.Fatalf("expected channel=telegram, got %+v", inst)
	}
}

func TestAddEndpoint_DaemonErrorIsSanitized(t *testing.T) {
	mux, srv, _ := buildTestMux(t, map[string]http.HandlerFunc{
		"POST /api/v1/agents/openclaw/install": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error":{"code":"E_COMMAND_FAILED","message":"install failed OPENAI_API_KEY=sk-secret-123"}}`)
		},
	})
	defer srv.Close()

	body := `{
		"agentId":"openclaw",
		"channel":"telegram",
		"channelToken":"tg-token",
		"providerId":"openai",
		"providerToken":"sk-test-token"
	}`
	req := httptest.NewRequest("POST", "/api/v1/add", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "sk-secret-123") {
		t.Fatalf("response should not leak secret token: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "install failed") {
		t.Fatalf("response should not expose internal daemon detail: %s", w.Body.String())
	}
}

func TestAddEndpoint_StatePersistenceFailureReturnsPartialSuccess(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	badStore := filepath.Join(tmp, "instances.json")
	if err := os.MkdirAll(badStore, 0o700); err != nil {
		t.Fatalf("prepare bad instance store path: %v", err)
	}

	mux, srv, _ := buildTestMux(t, map[string]http.HandlerFunc{
		"POST /api/v1/agents/openclaw/install": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status":"installed"}`)
		},
		"POST /api/v1/agents/openclaw/start": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status":"running"}`)
		},
	})
	defer srv.Close()
	t.Setenv("CARRIER_INSTANCE_STORE", badStore)

	body := `{
		"agentId":"openclaw",
		"channel":"telegram",
		"channelToken":"tg-token",
		"providerId":"openai",
		"providerToken":"sk-test-token"
	}`
	req := httptest.NewRequest("POST", "/api/v1/add", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"errorCode":"E_STATE_PERSISTENCE"`) {
		t.Fatalf("expected E_STATE_PERSISTENCE, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"partialSuccess":true`) {
		t.Fatalf("expected partialSuccess=true, got %s", w.Body.String())
	}
}

func TestAddEndpoint_PicoClawSuccess(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
	t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))

	mux, srv, _ := buildTestMux(t, map[string]http.HandlerFunc{
		"POST /api/v1/agents/picoclaw/install": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status":"installed"}`)
		},
		"POST /api/v1/agents/picoclaw/start": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status":"running"}`)
		},
		"GET /api/v1/agents/picoclaw/logs": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"lines":["PAIR_CODE: pair-0123456789abcdef0123456789abcdef"],"truncated":false}`)
		},
	})
	defer srv.Close()

	body := `{
		"agentId":"picoclaw",
		"channel":"telegram",
		"channelToken":"tg-token",
		"providerId":"openai",
		"providerToken":"sk-test-token",
		"reuseCredential":false,
		"envVars":{"FOO":"bar"}
	}`
	req := httptest.NewRequest("POST", "/api/v1/add", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"result":"ok"`) {
		t.Fatalf("expected ok result, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"pairCode":"pair-0123456789abcdef0123456789abcdef"`) {
		t.Fatalf("expected pairCode in response, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"pairRequired":true`) {
		t.Fatalf("expected pairRequired=true in response, got %s", w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse add response: %v", err)
	}
	instanceID, _ := resp["instanceId"].(string)
	if strings.TrimSpace(instanceID) == "" {
		t.Fatalf("expected non-empty instanceId, got %v", resp["instanceId"])
	}
	instances, _, err := loadManagedInstances()
	if err != nil {
		t.Fatalf("loadManagedInstances: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected 1 managed instance, got %d", len(instances))
	}
	if instances[0].ID != instanceID {
		t.Fatalf("instance id mismatch: store=%s response=%s", instances[0].ID, instanceID)
	}
	if !instances[0].PairRequired {
		t.Fatalf("expected instance pair_required=true, got %+v", instances[0])
	}
	if instances[0].RuntimeState != "pending_pair" {
		t.Fatalf("expected runtime_state pending_pair, got %+v", instances[0])
	}
}

func TestAddEndpoint_PicoClawSuccess_ReuseSavedOpenAICodexCredential(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
	t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))

	if _, err := saveProviderCredential("openai-codex", "codex-saved-token"); err != nil {
		t.Fatalf("saveProviderCredential: %v", err)
	}

	mux, srv, _ := buildTestMux(t, map[string]http.HandlerFunc{
		"POST /api/v1/agents/picoclaw/install": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status":"installed"}`)
		},
		"POST /api/v1/agents/picoclaw/start": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status":"running"}`)
		},
		"GET /api/v1/agents/picoclaw/logs": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"lines":[],"truncated":false}`)
		},
	})
	defer srv.Close()

	body := `{
		"agentId":"picoclaw",
		"channel":"telegram",
		"channelToken":"tg-token",
		"channelChatId":"418258935",
		"providerId":"openai-codex",
		"reuseCredential":true,
		"envVars":{"FOO":"bar"}
	}`
	req := httptest.NewRequest("POST", "/api/v1/add", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"result":"ok"`) {
		t.Fatalf("expected ok result, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"pairRequired":false`) {
		t.Fatalf("expected pairRequired=false in response, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"pairedChatId":"418258935"`) {
		t.Fatalf("expected pairedChatId in response, got %s", w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse add response: %v", err)
	}
	envKeysRaw, ok := resp["envKeys"].([]interface{})
	if !ok {
		t.Fatalf("expected envKeys array, got %#v", resp["envKeys"])
	}
	envKeySet := map[string]bool{}
	for _, item := range envKeysRaw {
		envKeySet[fmt.Sprintf("%v", item)] = true
	}
	for _, key := range []string{"FOO", "OPENAI_CODEX_TOKEN", "OPENAI_API_KEY"} {
		if !envKeySet[key] {
			t.Fatalf("expected envKeys to include %s, got %#v", key, envKeysRaw)
		}
	}

	instances, _, err := loadManagedInstances()
	if err != nil {
		t.Fatalf("loadManagedInstances: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected 1 managed instance, got %d", len(instances))
	}
	inst := instances[0]
	if inst.Provider != "openai-codex" {
		t.Fatalf("expected provider=openai-codex, got %+v", inst)
	}
	if inst.PairedChatID != "418258935" || inst.PairRequired {
		t.Fatalf("expected pre-paired chat state, got %+v", inst)
	}
	if inst.RuntimeState != "running" {
		t.Fatalf("expected runtime_state running, got %+v", inst)
	}

	cfgPath := filepath.Join(tmp, ".picoclaw", "config.json")
	cfgRaw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read picoclaw config: %v", err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(cfgRaw, &cfg); err != nil {
		t.Fatalf("parse picoclaw config: %v", err)
	}
	modelList, ok := cfg["model_list"].([]interface{})
	if !ok || len(modelList) != 1 {
		t.Fatalf("expected one model entry, got %#v", cfg["model_list"])
	}
	model, ok := modelList[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected model entry object, got %#v", modelList[0])
	}
	if got := strings.TrimSpace(fmt.Sprintf("%v", model["auth_method"])); got != "oauth" {
		t.Fatalf("expected auth_method oauth, got %q", got)
	}
	if _, hasAPIKey := model["api_key"]; hasAPIKey {
		t.Fatalf("did not expect oauth model api_key to be persisted, got %#v", model["api_key"])
	}

	authPath := filepath.Join(tmp, ".picoclaw", "auth.json")
	authRaw, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("read picoclaw auth store: %v", err)
	}
	if !strings.Contains(string(authRaw), "codex-saved-token") {
		t.Fatalf("expected auth store to contain saved codex token")
	}
}

func TestAddEndpoint_PicoClawSuccess_WithEnvFallbackChannelAndToken(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
	t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))
	t.Setenv("CARRIER_TELEGRAM_BOT_TOKEN", "tg-fallback-token")

	mux, srv, _ := buildTestMux(t, map[string]http.HandlerFunc{
		"POST /api/v1/agents/picoclaw/install": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status":"installed"}`)
		},
		"POST /api/v1/agents/picoclaw/start": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status":"running"}`)
		},
		"GET /api/v1/agents/picoclaw/logs": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"lines":[],"truncated":false}`)
		},
	})
	defer srv.Close()

	body := `{
		"agentId":"picoclaw",
		"providerId":"openai",
		"providerToken":"sk-test-token",
		"reuseCredential":false,
		"envVars":{"FOO":"bar"}
	}`
	req := httptest.NewRequest("POST", "/api/v1/add", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"result":"ok"`) {
		t.Fatalf("expected ok result, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"pairRequired":true`) {
		t.Fatalf("expected pairRequired=true in response, got %s", w.Body.String())
	}
}

func TestAddEndpoint_PicoClawSuccess_WithPrefetchedTelegramChatID(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
	t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))

	mux, srv, _ := buildTestMux(t, map[string]http.HandlerFunc{
		"POST /api/v1/agents/picoclaw/install": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status":"installed"}`)
		},
		"POST /api/v1/agents/picoclaw/start": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status":"running"}`)
		},
		"POST /bottg-prefetched/getUpdates": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"ok":true,"result":[]}`)
		},
		"GET /api/v1/agents/picoclaw/logs": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"lines":[],"truncated":false}`)
		},
	})
	defer srv.Close()

	body := `{
		"agentId":"picoclaw",
		"channel":"telegram",
		"channelToken":"tg-prefetched",
		"channelChatId":"418258935",
		"providerId":"openai",
		"providerToken":"sk-test-token",
		"reuseCredential":false,
		"envVars":{"FOO":"bar"}
	}`
	req := httptest.NewRequest("POST", "/api/v1/add", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"pairRequired":false`) {
		t.Fatalf("expected pairRequired=false in response, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"pairedChatId":"418258935"`) {
		t.Fatalf("expected pairedChatId in response, got %s", w.Body.String())
	}

	instances, _, err := loadManagedInstances()
	if err != nil {
		t.Fatalf("loadManagedInstances: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected 1 managed instance, got %d", len(instances))
	}
	if instances[0].PairRequired {
		t.Fatalf("expected pair_required=false, got %+v", instances[0])
	}
	if instances[0].PairedChatID != "418258935" {
		t.Fatalf("expected paired chat id 418258935, got %+v", instances[0])
	}
	if instances[0].RuntimeState != "running" {
		t.Fatalf("expected runtime_state running, got %+v", instances[0])
	}

	cfgPath := filepath.Join(tmp, ".picoclaw", "config.json")
	cfgRaw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read picoclaw config: %v", err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(cfgRaw, &cfg); err != nil {
		t.Fatalf("parse picoclaw config: %v", err)
	}
	channels, ok := cfg["channels"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected channels object, got %#v", cfg["channels"])
	}
	telegram, ok := channels["telegram"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected channels.telegram object, got %#v", channels["telegram"])
	}
	allowFrom, ok := telegram["allow_from"].([]interface{})
	if !ok {
		t.Fatalf("expected allow_from list, got %#v", telegram["allow_from"])
	}
	if len(allowFrom) != 1 || fmt.Sprintf("%v", allowFrom[0]) != "418258935" {
		t.Fatalf("expected allow_from [418258935], got %#v", allowFrom)
	}
}

func TestTelegramPairInitAndWait_Success(t *testing.T) {
	var pairCode string
	callCount := 0

	mux, srv, _ := buildTestMux(t, map[string]http.HandlerFunc{
		"POST /bottg-test-token/getUpdates": func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.WriteHeader(http.StatusOK)
			if callCount == 1 {
				fmt.Fprint(w, `{"ok":true,"result":[{"update_id":100,"message":{"chat":{"id":999},"text":"old message"}}]}`)
				return
			}
			fmt.Fprintf(w, `{"ok":true,"result":[{"update_id":101,"message":{"chat":{"id":418258935},"text":"/pair %s"}}]}`, pairCode)
		},
	})
	defer srv.Close()

	initReq := httptest.NewRequest("POST", "/api/v1/telegram/pair/init", strings.NewReader(`{"token":"tg-test-token"}`))
	initReq.Header.Set("Authorization", "Bearer test-gateway-token")
	initReq.Header.Set("Content-Type", "application/json")
	initRec := httptest.NewRecorder()
	mux.ServeHTTP(initRec, initReq)
	if initRec.Code != http.StatusOK {
		t.Fatalf("pair init expected 200, got %d: %s", initRec.Code, initRec.Body.String())
	}
	var initResp map[string]interface{}
	if err := json.Unmarshal(initRec.Body.Bytes(), &initResp); err != nil {
		t.Fatalf("parse pair init response: %v", err)
	}
	sessionID, _ := initResp["sessionId"].(string)
	pairCode, _ = initResp["pairCode"].(string)
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(pairCode) == "" {
		t.Fatalf("expected sessionId and pairCode, got %v", initResp)
	}

	waitReq := httptest.NewRequest("POST", "/api/v1/telegram/pair/wait", strings.NewReader(fmt.Sprintf(`{"sessionId":%q}`, sessionID)))
	waitReq.Header.Set("Authorization", "Bearer test-gateway-token")
	waitReq.Header.Set("Content-Type", "application/json")
	waitRec := httptest.NewRecorder()
	mux.ServeHTTP(waitRec, waitReq)
	if waitRec.Code != http.StatusOK {
		t.Fatalf("pair wait expected 200, got %d: %s", waitRec.Code, waitRec.Body.String())
	}
	var waitResp map[string]interface{}
	if err := json.Unmarshal(waitRec.Body.Bytes(), &waitResp); err != nil {
		t.Fatalf("parse pair wait response: %v", err)
	}
	if paired, _ := waitResp["paired"].(bool); !paired {
		t.Fatalf("expected paired=true, got %v", waitResp)
	}
	if chatID := fmt.Sprintf("%v", waitResp["chatId"]); chatID != "418258935" {
		t.Fatalf("expected chat id 418258935, got %s", chatID)
	}
}

func TestPairingSessionsEndpoint_ReturnsLatestFirst(t *testing.T) {
	mux, srv, sessions := buildTestMux(t, nil)
	defer srv.Close()

	sessions.CreateSession("telegram", "1001")
	time.Sleep(2 * time.Millisecond)
	sessions.CreateSession("telegram", "1002")
	sessions.CreateSession("discord", "2001")

	req := httptest.NewRequest("GET", "/api/v1/pairing/sessions?provider=telegram", nil)
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Sessions []struct {
			Provider string `json:"provider"`
			ChatID   string `json:"chatId"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if len(resp.Sessions) != 2 {
		t.Fatalf("expected 2 telegram sessions, got %#v", resp.Sessions)
	}
	if resp.Sessions[0].ChatID != "1002" {
		t.Fatalf("expected latest chat id first (1002), got %#v", resp.Sessions)
	}
	if resp.Sessions[0].Provider != "telegram" || resp.Sessions[1].Provider != "telegram" {
		t.Fatalf("expected provider telegram for all sessions, got %#v", resp.Sessions)
	}
}
