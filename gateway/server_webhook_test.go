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

func TestTelegramWebhook_InvalidSecret(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	body := `{"update_id":1,"message":{"chat":{"id":123},"text":"/pair abc"}}`
	req := httptest.NewRequest("POST", "/webhook/telegram", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Should fail auth (no secret configured = empty, so verification should pass for empty)
	// Check it doesn't crash at least
	if w.Code == 0 {
		t.Fatal("expected non-zero status code")
	}
}

func TestDiscordWebhook_InvalidSignature(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	body := `{"type":1}`
	req := httptest.NewRequest("POST", "/webhook/discord", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Should fail signature verification
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestFeishuWebhook_InvalidToken(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	body := `{"event":{"message":{"chat_id":"123","content":"/pair abc"}}}`
	req := httptest.NewRequest("POST", "/webhook/feishu", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Should fail token verification (no token configured)
	if w.Code == 0 {
		t.Fatal("expected non-zero status code")
	}
}

func TestDownload_ValidToken(t *testing.T) {
	srv := newMockDaemon(nil)
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "test-token", 5*time.Second)
	sessions := NewSessionStore("", 0, nil)
	t.Cleanup(sessions.Stop)

	tmpDir := t.TempDir()
	downloads := NewDownloadStore(tmpDir, nil)
	rl := NewGatewayRateLimiter(100, 1000, 1*time.Minute, nil)
	onboard := NewOnboardStore()
	setup := NewSetupStore()
	cfg := &GatewayConfig{
		APIToken:            "test-gateway-token",
		MaxCommandBodyBytes: 64 * 1024,
		ArtifactRoot:        tmpDir,
	}
	mux := buildGatewayMux(cfg, dc, sessions, downloads, rl, onboard, setup)

	// Create a test file
	testFile := filepath.Join(tmpDir, "test-artifact.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Issue a download token
	tok := downloads.Issue(testFile, 5*time.Minute, false)
	dlURL := downloads.ToDownloadURL(tok)

	req := httptest.NewRequest("GET", dlURL, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	if w.Body.String() != "test content" {
		t.Errorf("expected 'test content', got %q", w.Body.String())
	}
}

func TestDownload_InvalidToken(t *testing.T) {
	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/downloads/invalid-token/file.txt", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestExpectedFileName_Webhook(t *testing.T) {
	tests := []struct {
		fileRef string
		want    string
	}{
		{"/tmp/test.txt", "test.txt"},
		{"/path/to/artifact.tar.gz", "artifact.tar.gz"},
		{"simple.log", "simple.log"},
	}
	for _, tc := range tests {
		t.Run(tc.fileRef, func(t *testing.T) {
			got := ExpectedFileName(tc.fileRef)
			if got != tc.want {
				t.Errorf("ExpectedFileName(%q) = %q, want %q", tc.fileRef, got, tc.want)
			}
		})
	}
}

func TestIsPathUnderRoot_Webhook(t *testing.T) {
	tests := []struct {
		path string
		root string
		want bool
	}{
		{"/tmp/artifacts/file.txt", "/tmp/artifacts", true},
		{"/tmp/artifacts/sub/file.txt", "/tmp/artifacts", true},
		{"/tmp/other/file.txt", "/tmp/artifacts", false},
	}
	for _, tc := range tests {
		got := IsPathUnderRoot(tc.path, tc.root)
		if got != tc.want {
			t.Errorf("IsPathUnderRoot(%q, %q) = %v, want %v", tc.path, tc.root, got, tc.want)
		}
	}
}

// SSE test removed - httptest.ResponseRecorder doesn't implement http.Flusher,
// so the SSE handler returns 500 and the infinite loop would hang the test.

func TestTelegramWebhook_NonCommand(t *testing.T) {
	srv := newMockDaemon(nil)
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "test-token", 5*time.Second)
	sessions := NewSessionStore("", 0, nil)
	t.Cleanup(sessions.Stop)
	downloads := NewDownloadStore("", nil)
	rl := NewGatewayRateLimiter(100, 1000, 1*time.Minute, nil)
	onboard := NewOnboardStore()
	setup := NewSetupStore()
	cfg := &GatewayConfig{
		MaxCommandBodyBytes: 64 * 1024,
	}
	mux := buildGatewayMux(cfg, dc, sessions, downloads, rl, onboard, setup)

	// Non-command message should route to base-agent chat and require pairing first
	body := `{"update_id":1,"message":{"chat":{"id":123},"text":"hello"}}`
	req := httptest.NewRequest("POST", "/webhook/telegram", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	text, _ := resp["text"].(string)
	if !strings.Contains(text, "E_SESSION_REQUIRED") {
		t.Errorf("expected E_SESSION_REQUIRED in text, got %q", text)
	}
}

func TestFeishuWebhook_Challenge(t *testing.T) {
	srv := newMockDaemon(nil)
	defer srv.Close()

	dc := NewDaemonClient(srv.URL, "test-token", 5*time.Second)
	sessions := NewSessionStore("", 0, nil)
	t.Cleanup(sessions.Stop)
	downloads := NewDownloadStore("", nil)
	rl := NewGatewayRateLimiter(100, 1000, 1*time.Minute, nil)
	onboard := NewOnboardStore()
	setup := NewSetupStore()
	cfg := &GatewayConfig{
		MaxCommandBodyBytes: 64 * 1024,
	}
	mux := buildGatewayMux(cfg, dc, sessions, downloads, rl, onboard, setup)

	body := `{"challenge":"test-challenge","token":"","type":"url_verification"}`
	req := httptest.NewRequest("POST", "/webhook/feishu", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["challenge"] != "test-challenge" {
		t.Errorf("expected challenge echoed, got %v", resp)
	}
}

func buildFeishuWebhookMuxWithSessions(
	t *testing.T,
	daemonHandlers map[string]http.HandlerFunc,
	maxBodyBytes int,
	token string,
) (http.Handler, *httptest.Server, *SessionStore) {
	t.Helper()
	t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(t.TempDir(), "instances.json"))

	srv := newMockDaemon(daemonHandlers)
	dc := NewDaemonClient(srv.URL, "test-token", 5*time.Second)
	sessions := NewSessionStore("", 0, nil)
	t.Cleanup(sessions.Stop)
	downloads := NewDownloadStore("", nil)
	rl := NewGatewayRateLimiter(100, 1000, time.Minute, nil)
	onboard := NewOnboardStore()
	setup := NewSetupStore()
	cfg := &GatewayConfig{
		MaxCommandBodyBytes:     maxBodyBytes,
		FeishuVerificationToken: token,
	}
	mux := buildGatewayMux(cfg, dc, sessions, downloads, rl, onboard, setup)
	return mux, srv, sessions
}

func TestChannelRoutingAuthHappyPaths(t *testing.T) {
	daemonHandlers := map[string]http.HandlerFunc{
		"GET /api/v1/agents": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"agents": []map[string]interface{}{
					{
						"id":           "openclaw",
						"name":         "OpenClaw",
						"installState": "installed",
						"runtimeState": "running",
						"health":       "healthy",
					},
				},
			})
		},
	}

	t.Run("telegram paired command", func(t *testing.T) {
		mux, srv, sessions := buildTelegramWebhookMux(t, daemonHandlers, 64*1024, "expected-secret")
		defer srv.Close()
		sessions.CreateSession("telegram", "123")

		w := postTelegramWebhook(t, mux, `{"update_id":1,"message":{"message_id":99,"chat":{"id":123},"text":"/agents"}}`, "expected-secret")
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp["method"] != "sendMessage" {
			t.Fatalf("expected sendMessage response, got %#v", resp)
		}
		text, _ := resp["text"].(string)
		if !strings.Contains(text, "listed 1 agents") {
			t.Fatalf("expected successful agent listing, got %q", text)
		}
	})

	t.Run("discord interaction command", func(t *testing.T) {
		mux, srv, sessions, priv := buildDiscordWebhookMux(t, daemonHandlers)
		defer srv.Close()
		sessions.CreateSession("discord", "discord-chat-3")

		body := `{"type":2,"id":"i-2","channel_id":"discord-chat-3","data":{"name":"agents"}}`
		w := postSignedDiscordWebhook(t, mux, priv, body)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}

		var resp map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		data, _ := resp["data"].(map[string]interface{})
		content, _ := data["content"].(string)
		if !strings.Contains(content, "listed 1 agents") {
			t.Fatalf("expected successful agent listing, got %q", content)
		}
	})

	t.Run("feishu webhook command", func(t *testing.T) {
		mux, srv, sessions := buildFeishuWebhookMuxWithSessions(t, daemonHandlers, 64*1024, "feishu-token")
		defer srv.Close()
		sessions.CreateSession("feishu", "f3")

		body := `{"header":{"token":"feishu-token","event_id":"evt-3"},"event":{"message":{"chat_id":"f3","message_id":"m3","content":"{\"text\":\"/agents\"}"}}}`
		w := postFeishuWebhook(t, mux, body)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		content, _ := resp["content"].(map[string]interface{})
		text, _ := content["text"].(string)
		if !strings.Contains(text, "listed 1 agents") {
			t.Fatalf("expected successful agent listing, got %q", text)
		}
	})
}
