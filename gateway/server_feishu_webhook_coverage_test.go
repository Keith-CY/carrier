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

func buildFeishuWebhookMux(
	t *testing.T,
	daemonHandlers map[string]http.HandlerFunc,
	maxBodyBytes int,
	token string,
) (http.Handler, *httptest.Server) {
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
	return mux, srv
}

func postFeishuWebhook(t *testing.T, mux http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/webhook/feishu", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func TestFeishuWebhook_PayloadTooLarge(t *testing.T) {
	mux, srv := buildFeishuWebhookMux(t, nil, 12, "")
	defer srv.Close()

	w := postFeishuWebhook(t, mux, `{"header":{"token":"t"},"event":{"message":{"chat_id":"1","content":"{\"text\":\"/pair abc\"}"}}}`)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"errorCode":"E_PAYLOAD_TOO_LARGE"`) {
		t.Fatalf("expected E_PAYLOAD_TOO_LARGE, got %s", w.Body.String())
	}
}

func TestFeishuWebhook_InvalidJSON(t *testing.T) {
	mux, srv := buildFeishuWebhookMux(t, nil, 64*1024, "feishu-token")
	defer srv.Close()

	w := postFeishuWebhook(t, mux, `{"header":`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"errorCode":"E_USAGE"`) {
		t.Fatalf("expected E_USAGE, got %s", w.Body.String())
	}
}

func TestFeishuWebhook_InvalidVerificationToken(t *testing.T) {
	mux, srv := buildFeishuWebhookMux(t, nil, 64*1024, "feishu-token")
	defer srv.Close()

	body := `{"header":{"token":"wrong-token"},"event":{"message":{"chat_id":"f1","content":"{\"text\":\"/pair abc\"}"}}}`
	w := postFeishuWebhook(t, mux, body)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"errorCode":"E_FEISHU_VERIFICATION_FAILED"`) {
		t.Fatalf("expected E_FEISHU_VERIFICATION_FAILED, got %s", w.Body.String())
	}
}

func TestFeishuWebhook_IgnoreUnsupportedEvent(t *testing.T) {
	mux, srv := buildFeishuWebhookMux(t, nil, 64*1024, "feishu-token")
	defer srv.Close()

	body := `{"header":{"token":"feishu-token"},"event":{"sender":{"sender_id":"u1"}}}`
	w := postFeishuWebhook(t, mux, body)
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
	if !strings.Contains(msg, "ignored non-command feishu event") {
		t.Fatalf("expected ignore message, got %q", msg)
	}
}

func TestFeishuWebhook_CommandPairSuccess(t *testing.T) {
	mux, srv := buildFeishuWebhookMux(t, map[string]http.HandlerFunc{
		"POST /api/v1/pairing/verify-consume": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"code":"abc","consumed":true}`))
		},
	}, 64*1024, "feishu-token")
	defer srv.Close()

	body := `{"header":{"token":"feishu-token","event_id":"evt-1"},"event":{"message":{"chat_id":"f1","message_id":"m1","content":"{\"text\":\"/pair abc\"}"}}}`
	w := postFeishuWebhook(t, mux, body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["msg_type"] != "text" {
		t.Fatalf("expected msg_type=text, got %#v", resp)
	}
	content, _ := resp["content"].(map[string]interface{})
	text, _ := content["text"].(string)
	if !strings.Contains(text, "paired") {
		t.Fatalf("expected pair success text, got %q", text)
	}
}

func TestFeishuWebhook_NonCommandRoutesToBaseAgentChat(t *testing.T) {
	mux, srv := buildFeishuWebhookMux(t, nil, 64*1024, "feishu-token")
	defer srv.Close()

	body := `{"header":{"token":"feishu-token","event_id":"evt-2"},"event":{"message":{"chat_id":"f2","message_id":"m2","content":"{\"text\":\"hello\"}"}}}`
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
	if !strings.Contains(text, "E_SESSION_REQUIRED") {
		t.Fatalf("expected E_SESSION_REQUIRED for unpaired base-agent chat, got %q", text)
	}
}
