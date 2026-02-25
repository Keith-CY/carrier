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

func buildTelegramWebhookMux(
	t *testing.T,
	daemonHandlers map[string]http.HandlerFunc,
	maxBodyBytes int,
	secret string,
) (http.Handler, *httptest.Server, *SessionStore) {
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
		MaxCommandBodyBytes:   maxBodyBytes,
		TelegramWebhookSecret: secret,
	}
	mux := buildGatewayMux(cfg, dc, sessions, downloads, rl, onboard, setup)
	return mux, srv, sessions
}

func postTelegramWebhook(t *testing.T, mux http.Handler, body string, secret string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/webhook/telegram", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(secret) != "" {
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", secret)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func TestTelegramWebhook_PayloadTooLarge(t *testing.T) {
	mux, srv, _ := buildTelegramWebhookMux(t, nil, 16, "")
	defer srv.Close()

	w := postTelegramWebhook(t, mux, `{"update_id":1,"message":{"chat":{"id":123},"text":"/pair abc"}}`, "")
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"errorCode":"E_PAYLOAD_TOO_LARGE"`) {
		t.Fatalf("expected E_PAYLOAD_TOO_LARGE, got %s", w.Body.String())
	}
}

func TestTelegramWebhook_InvalidSecret_Strict(t *testing.T) {
	mux, srv, _ := buildTelegramWebhookMux(t, nil, 64*1024, "expected-secret")
	defer srv.Close()

	w := postTelegramWebhook(t, mux, `{"update_id":1,"message":{"chat":{"id":123},"text":"/pair abc"}}`, "wrong-secret")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"errorCode":"E_TELEGRAM_VERIFICATION_FAILED"`) {
		t.Fatalf("expected E_TELEGRAM_VERIFICATION_FAILED, got %s", w.Body.String())
	}
}

func TestTelegramWebhook_InvalidJSON(t *testing.T) {
	mux, srv, _ := buildTelegramWebhookMux(t, nil, 64*1024, "expected-secret")
	defer srv.Close()

	w := postTelegramWebhook(t, mux, `{"update_id":1`, "expected-secret")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"errorCode":"E_USAGE"`) {
		t.Fatalf("expected E_USAGE, got %s", w.Body.String())
	}
}

func TestTelegramWebhook_IgnoreUnsupportedUpdate(t *testing.T) {
	mux, srv, _ := buildTelegramWebhookMux(t, nil, 64*1024, "")
	defer srv.Close()

	w := postTelegramWebhook(t, mux, `{"update_id":1}`, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["result"] != "ok" {
		t.Fatalf("expected result=ok, got %#v", resp)
	}
	msg, _ := resp["message"].(string)
	if !strings.Contains(msg, "ignored non-command telegram update") {
		t.Fatalf("expected ignore message, got %q", msg)
	}
}

func TestTelegramWebhook_CommandPairSuccess(t *testing.T) {
	mux, srv, _ := buildTelegramWebhookMux(t, map[string]http.HandlerFunc{
		"POST /api/v1/pairing/verify-consume": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"code":"abc","consumed":true}`))
		},
	}, 64*1024, "expected-secret")
	defer srv.Close()

	w := postTelegramWebhook(t, mux, `{"update_id":1,"message":{"message_id":99,"chat":{"id":123},"text":"/pair abc"}}`, "expected-secret")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["method"] != "sendMessage" {
		t.Fatalf("expected method=sendMessage, got %#v", resp)
	}
	text, _ := resp["text"].(string)
	if !strings.Contains(text, "paired") {
		t.Fatalf("expected pair success text, got %q", text)
	}
}

func TestTelegramWebhook_NonCommandRoutesToBaseAgentChat(t *testing.T) {
	mux, srv, _ := buildTelegramWebhookMux(t, nil, 64*1024, "")
	defer srv.Close()

	w := postTelegramWebhook(t, mux, `{"update_id":2,"message":{"message_id":100,"chat":{"id":123},"text":"hello"}}`, "")
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
	if !strings.Contains(text, "E_SESSION_REQUIRED") {
		t.Fatalf("expected E_SESSION_REQUIRED for unpaired base-agent chat, got %q", text)
	}
}
