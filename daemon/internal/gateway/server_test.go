package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func buildTestMux(daemonHandlers map[string]http.HandlerFunc) (http.Handler, *httptest.Server, *SessionStore) {
	srv := newMockDaemon(daemonHandlers)
	dc := NewDaemonClient(srv.URL, "test-token", 5*time.Second)
	sessions := NewSessionStore("", 0, nil)
	downloads := NewDownloadStore("", nil)
	rl := NewGatewayRateLimiter(100, 1000, 1*time.Minute, nil)
	onboard := NewOnboardStore()
	setup := NewSetupStore()
	cfg := &GatewayConfig{
		APIToken:            "test-gateway-token",
		MaxCommandBodyBytes: 64 * 1024,
	}
	mux := buildGatewayMux(cfg, dc, sessions, downloads, rl, onboard, setup)
	return mux, srv, sessions
}

func TestHealthz(t *testing.T) {
	mux, srv, _ := buildTestMux(nil)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]string
	_ = json.NewDecoder(w.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %q", body["status"])
	}
}

func TestCommand_MethodNotAllowed(t *testing.T) {
	mux, srv, _ := buildTestMux(nil)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/command", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestCommand_NoToken(t *testing.T) {
	mux, srv, _ := buildTestMux(nil)
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
	mux, srv, _ := buildTestMux(nil)
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
	mux, srv, _ := buildTestMux(nil)
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
	mux, srv, _ := buildTestMux(map[string]http.HandlerFunc{
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
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Result != "ok" {
		t.Errorf("expected ok, got %s: %s", resp.Result, resp.Message)
	}
}

func TestCommand_PlainTextBody(t *testing.T) {
	mux, srv, _ := buildTestMux(map[string]http.HandlerFunc{
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
	mux, srv, _ := buildTestMux(nil)
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
	mux, srv, sessions := buildTestMux(map[string]http.HandlerFunc{
		"GET /api/v1/agents": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{"agents": []interface{}{}})
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
	mux, srv, _ := buildTestMux(map[string]http.HandlerFunc{
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
	mux, srv, _ := buildTestMux(nil)
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
	mux, srv, _ := buildTestMux(nil)
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
	mux, srv, _ := buildTestMux(nil)
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
	mux, srv, _ := buildTestMux(nil)
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
	mux, srv, _ := buildTestMux(nil)
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
	mux, srv, _ := buildTestMux(nil)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/setup", nil)
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSSELogs_NoToken(t *testing.T) {
	mux, srv, _ := buildTestMux(nil)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/api/v1/logs/stream?agent=a1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestSSELogs_MissingAgent(t *testing.T) {
	mux, srv, _ := buildTestMux(nil)
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
	mux, srv, _ := buildTestMux(nil)
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
	mux, srv, _ := buildTestMux(nil)
	defer srv.Close()

	req := httptest.NewRequest("POST", "/downloads/something", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestDownloads_InvalidPath(t *testing.T) {
	mux, srv, _ := buildTestMux(nil)
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
	mux, srv, _ := buildTestMux(nil)
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
	mux, srv, _ := buildTestMux(nil)
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
	mux, srv, _ := buildTestMux(nil)
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
	body, _ := io.ReadAll(w.Body)
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
	if srv.IdleTimeout != gatewayIdleTimeout {
		t.Errorf("unexpected IdleTimeout: %v", srv.IdleTimeout)
	}
}
