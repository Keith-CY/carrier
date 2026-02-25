package gateway

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func buildDiscordWebhookMux(t *testing.T, daemonHandlers map[string]http.HandlerFunc) (http.Handler, *httptest.Server, *SessionStore, ed25519.PrivateKey) {
	t.Helper()
	t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(t.TempDir(), "instances.json"))

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}

	srv := newMockDaemon(daemonHandlers)
	dc := NewDaemonClient(srv.URL, "test-token", 5*time.Second)
	sessions := NewSessionStore("", 0, nil)
	t.Cleanup(sessions.Stop)
	downloads := NewDownloadStore("", nil)
	rl := NewGatewayRateLimiter(100, 1000, 1*time.Minute, nil)
	onboard := NewOnboardStore()
	setup := NewSetupStore()
	cfg := &GatewayConfig{
		MaxCommandBodyBytes: 64 * 1024,
		DiscordPublicKey:    hex.EncodeToString(pub),
	}
	mux := buildGatewayMux(cfg, dc, sessions, downloads, rl, onboard, setup)
	return mux, srv, sessions, priv
}

func postSignedDiscordWebhook(t *testing.T, mux http.Handler, priv ed25519.PrivateKey, body string) *httptest.ResponseRecorder {
	t.Helper()
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	message := append([]byte(timestamp), []byte(body)...)
	signature := ed25519.Sign(priv, message)

	req := httptest.NewRequest(http.MethodPost, "/webhook/discord", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature-Ed25519", hex.EncodeToString(signature))
	req.Header.Set("X-Signature-Timestamp", timestamp)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func TestDiscordWebhook_PingWithValidSignature(t *testing.T) {
	mux, srv, _, priv := buildDiscordWebhookMux(t, nil)
	defer srv.Close()

	w := postSignedDiscordWebhook(t, mux, priv, `{"type":1}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got, ok := resp["type"].(float64); !ok || got != 1 {
		t.Fatalf("expected ping response type=1, got %#v", resp)
	}
}

func TestDiscordWebhook_InvalidJSONWithValidSignature(t *testing.T) {
	mux, srv, _, priv := buildDiscordWebhookMux(t, nil)
	defer srv.Close()

	w := postSignedDiscordWebhook(t, mux, priv, `{"type":2`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["errorCode"] != "E_USAGE" {
		t.Fatalf("expected E_USAGE, got %#v", resp)
	}
}

func TestDiscordWebhook_UnsupportedPayloadWithValidSignature(t *testing.T) {
	mux, srv, _, priv := buildDiscordWebhookMux(t, nil)
	defer srv.Close()

	w := postSignedDiscordWebhook(t, mux, priv, `{"type":3}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["errorCode"] != "E_USAGE" {
		t.Fatalf("expected E_USAGE for unsupported payload, got %#v", resp)
	}
}

func TestDiscordWebhook_NonCommandMessageRoutesToBaseAgent(t *testing.T) {
	mux, srv, _, priv := buildDiscordWebhookMux(t, nil)
	defer srv.Close()

	body := `{"id":"m-1","channel_id":"discord-chat-1","content":"hello from discord"}`
	w := postSignedDiscordWebhook(t, mux, priv, body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	content, _ := resp["content"].(string)
	if !strings.Contains(content, "E_SESSION_REQUIRED") {
		t.Fatalf("expected pairing error content, got %q", content)
	}
}

func TestDiscordWebhook_SlashCommandWithoutSession(t *testing.T) {
	mux, srv, _, priv := buildDiscordWebhookMux(t, nil)
	defer srv.Close()

	body := `{"type":2,"id":"i-1","channel_id":"discord-chat-2","data":{"name":"agents"}}`
	w := postSignedDiscordWebhook(t, mux, priv, body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got, ok := resp["type"].(float64); !ok || got != 4 {
		t.Fatalf("expected interaction response wrapper type=4, got %#v", resp)
	}
	data, _ := resp["data"].(map[string]interface{})
	content, _ := data["content"].(string)
	if !strings.Contains(content, "E_SESSION_REQUIRED") {
		t.Fatalf("expected session required error in interaction data, got %q", content)
	}
}

func TestDiscordWebhook_SlashCommandWithSession(t *testing.T) {
	mux, srv, sessions, priv := buildDiscordWebhookMux(t, map[string]http.HandlerFunc{
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
	})
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
	if got, ok := resp["type"].(float64); !ok || got != 4 {
		t.Fatalf("expected interaction response wrapper type=4, got %#v", resp)
	}
	data, _ := resp["data"].(map[string]interface{})
	content, _ := data["content"].(string)
	if !strings.Contains(content, "listed 1 agents") {
		t.Fatalf("expected successful agent listing, got %q", content)
	}
}
