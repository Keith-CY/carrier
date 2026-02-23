package gateway

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// --- Discord signature verification ---

func TestVerifyDiscordSignature_Valid(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubHex := hex.EncodeToString(pub)

	body := []byte(`{"type":1}`)
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	message := append([]byte(timestamp), body...)
	sig := ed25519.Sign(priv, message)
	sigHex := hex.EncodeToString(sig)

	req := httptest.NewRequest(http.MethodPost, "/webhook/discord", strings.NewReader(string(body)))
	req.Header.Set("X-Signature-Ed25519", sigHex)
	req.Header.Set("X-Signature-Timestamp", timestamp)

	if !VerifyDiscordSignature(req, body, pubHex, 300) {
		t.Error("valid signature should verify")
	}
}

func TestVerifyDiscordSignature_InvalidSig(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubHex := hex.EncodeToString(pub)

	body := []byte(`{"type":1}`)
	timestamp := fmt.Sprintf("%d", time.Now().Unix())

	// Wrong signature (all zeros)
	badSig := make([]byte, 64)
	sigHex := hex.EncodeToString(badSig)

	req := httptest.NewRequest(http.MethodPost, "/webhook/discord", strings.NewReader(string(body)))
	req.Header.Set("X-Signature-Ed25519", sigHex)
	req.Header.Set("X-Signature-Timestamp", timestamp)

	if VerifyDiscordSignature(req, body, pubHex, 300) {
		t.Error("invalid signature should not verify")
	}
}

func TestVerifyDiscordSignature_ExpiredTimestamp(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubHex := hex.EncodeToString(pub)

	body := []byte(`{"type":1}`)
	// Timestamp far in the past
	timestamp := fmt.Sprintf("%d", time.Now().Unix()-600)
	message := append([]byte(timestamp), body...)
	sig := ed25519.Sign(priv, message)
	sigHex := hex.EncodeToString(sig)

	req := httptest.NewRequest(http.MethodPost, "/webhook/discord", strings.NewReader(string(body)))
	req.Header.Set("X-Signature-Ed25519", sigHex)
	req.Header.Set("X-Signature-Timestamp", timestamp)

	if VerifyDiscordSignature(req, body, pubHex, 300) {
		t.Error("expired timestamp should not verify")
	}
}

func TestVerifyDiscordSignature_MissingKey(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/webhook/discord", strings.NewReader("{}"))
	req.Header.Set("X-Signature-Ed25519", "aabb")
	req.Header.Set("X-Signature-Timestamp", "12345")
	if VerifyDiscordSignature(req, []byte("{}"), "", 300) {
		t.Error("missing public key should not verify")
	}
}

// --- Telegram ---

func TestVerifyTelegramSecret(t *testing.T) {
	tests := []struct {
		provided string
		expected string
		want     bool
	}{
		{"abc", "abc", true},
		{"abc", "def", false},
		{"", "abc", false},
		{"abc", "", true}, // no expected → always pass
		{"", "", true},
	}
	for _, tc := range tests {
		got := VerifyTelegramSecret(tc.provided, tc.expected)
		if got != tc.want {
			t.Errorf("VerifyTelegramSecret(%q, %q) = %v, want %v", tc.provided, tc.expected, got, tc.want)
		}
	}
}

func TestParseTelegramUpdate(t *testing.T) {
	payload := map[string]interface{}{
		"update_id": float64(12345),
		"message": map[string]interface{}{
			"message_id": float64(99),
			"text":       "/agents",
			"chat": map[string]interface{}{
				"id": float64(777),
			},
		},
	}
	nc := ParseTelegramUpdate(payload)
	if nc == nil {
		t.Fatal("expected NormalizedCommand, got nil")
	}
	if nc.Provider != "telegram" {
		t.Errorf("provider: %q", nc.Provider)
	}
	if nc.ChatID != "777" {
		t.Errorf("chatID: %q", nc.ChatID)
	}
	if nc.Command != "/agents" {
		t.Errorf("command: %q", nc.Command)
	}
}

func TestParseTelegramUpdate_NonCommand(t *testing.T) {
	payload := map[string]interface{}{
		"update_id": float64(1),
		"message": map[string]interface{}{
			"text": "Hello world",
			"chat": map[string]interface{}{"id": float64(1)},
		},
	}
	if nc := ParseTelegramUpdate(payload); nc != nil {
		t.Error("non-command text should return nil")
	}
}

func TestParseTelegramMessage_NonCommand(t *testing.T) {
	payload := map[string]interface{}{
		"update_id": float64(1),
		"message": map[string]interface{}{
			"text": "Hello world",
			"chat": map[string]interface{}{"id": float64(1)},
		},
	}
	msg := ParseTelegramMessage(payload)
	if msg == nil {
		t.Fatal("expected normalized message for non-command text")
	}
	if msg.Command != nil {
		t.Fatalf("expected nil command for non-command text, got %+v", msg.Command)
	}
	if msg.RawText != "Hello world" {
		t.Fatalf("raw text = %q, want %q", msg.RawText, "Hello world")
	}
}

// --- Feishu ---

func TestVerifyFeishuToken(t *testing.T) {
	payload := map[string]interface{}{
		"header": map[string]interface{}{
			"token": "mytoken",
		},
	}
	if !VerifyFeishuToken(payload, "mytoken") {
		t.Error("matching token should verify")
	}
	if VerifyFeishuToken(payload, "wrongtoken") {
		t.Error("wrong token should not verify")
	}
	if !VerifyFeishuToken(payload, "") {
		t.Error("empty expected token should always pass")
	}
}

func TestExtractFeishuChallenge(t *testing.T) {
	payload := map[string]interface{}{
		"type":      "url_verification",
		"challenge": "test-challenge",
	}
	challenge, ok := ExtractFeishuChallenge(payload)
	if !ok {
		t.Error("expected challenge to be extracted")
	}
	if challenge != "test-challenge" {
		t.Errorf("challenge: %q", challenge)
	}

	// Non-verification payload
	_, ok2 := ExtractFeishuChallenge(map[string]interface{}{"type": "event"})
	if ok2 {
		t.Error("non-verification payload should not have challenge")
	}
}

func TestParseFeishuEvent(t *testing.T) {
	payload := map[string]interface{}{
		"header": map[string]interface{}{
			"event_id": "ev1",
		},
		"event": map[string]interface{}{
			"message": map[string]interface{}{
				"chat_id": "chat-123",
				"content": `{"text":"/status"}`,
			},
		},
	}
	nc := ParseFeishuEvent(payload)
	if nc == nil {
		t.Fatal("expected NormalizedCommand, got nil")
	}
	if nc.Provider != "feishu" {
		t.Errorf("provider: %q", nc.Provider)
	}
	if nc.ChatID != "chat-123" {
		t.Errorf("chatID: %q", nc.ChatID)
	}
	if nc.Command != "/status" {
		t.Errorf("command: %q", nc.Command)
	}
}

func TestParseFeishuMessage_NonCommand(t *testing.T) {
	payload := map[string]interface{}{
		"header": map[string]interface{}{
			"event_id": "ev2",
		},
		"event": map[string]interface{}{
			"message": map[string]interface{}{
				"chat_id": "chat-456",
				"content": `{"text":"hello"}`,
			},
		},
	}
	msg := ParseFeishuMessage(payload)
	if msg == nil {
		t.Fatal("expected normalized message for non-command text")
	}
	if msg.Command != nil {
		t.Fatalf("expected nil command for non-command text, got %+v", msg.Command)
	}
	if msg.ChatID != "chat-456" {
		t.Fatalf("chatID = %q, want %q", msg.ChatID, "chat-456")
	}
}

// --- Discord payload parsing ---

func TestParseDiscordPayload_MessageCreate(t *testing.T) {
	payload := map[string]interface{}{
		"id":         "msg-1",
		"channel_id": "ch-abc",
		"content":    "/agents",
	}
	nc := ParseDiscordPayload(payload)
	if nc == nil {
		t.Fatal("expected NormalizedCommand")
	}
	if nc.ChatID != "ch-abc" {
		t.Errorf("chatID: %q", nc.ChatID)
	}
	if nc.Command != "/agents" {
		t.Errorf("command: %q", nc.Command)
	}
}

func TestParseDiscordPayload_SlashCommand(t *testing.T) {
	payload := map[string]interface{}{
		"type":       float64(2),
		"id":         "interaction-1",
		"channel_id": "ch-xyz",
		"data": map[string]interface{}{
			"name": "agents",
		},
	}
	nc := ParseDiscordPayload(payload)
	if nc == nil {
		t.Fatal("expected NormalizedCommand for slash command")
	}
	if nc.Command != "/agents" {
		t.Errorf("command: %q", nc.Command)
	}
}

func TestParseDiscordMessage_NonCommand(t *testing.T) {
	payload := map[string]interface{}{
		"id":         "msg-2",
		"channel_id": "ch-non-command",
		"content":    "hello",
	}
	msg := ParseDiscordMessage(payload)
	if msg == nil {
		t.Fatal("expected normalized message for non-command text")
	}
	if msg.Command != nil {
		t.Fatalf("expected nil command for non-command text, got %+v", msg.Command)
	}
	if msg.RawText != "hello" {
		t.Fatalf("raw text = %q, want %q", msg.RawText, "hello")
	}
}

// --- Renderers ---

func TestRenderTelegramResponse_OK(t *testing.T) {
	resp := GatewayResponse{
		RequestID:    "r1",
		Result:       "ok",
		Message:      "listed 3 agents",
		SessionToken: "session-abc",
	}
	rendered := RenderTelegramResponse(resp)
	text, _ := rendered["text"].(string)
	if !strings.Contains(text, "✅") {
		t.Errorf("ok response should contain ✅: %q", text)
	}
	if !strings.Contains(text, "session-abc") {
		t.Errorf("should contain session token: %q", text)
	}
}

func TestRenderTelegramResponse_Error(t *testing.T) {
	resp := GatewayResponse{
		RequestID: "r1",
		Result:    "error",
		ErrorCode: "E_SESSION_REQUIRED",
		Message:   "not paired",
	}
	rendered := RenderTelegramResponse(resp)
	text, _ := rendered["text"].(string)
	if !strings.Contains(text, "❌") {
		t.Errorf("error response should contain ❌: %q", text)
	}
	if !strings.Contains(text, "E_SESSION_REQUIRED") {
		t.Errorf("should contain error code: %q", text)
	}
}

func TestRenderTelegramWebhookResponse_OK(t *testing.T) {
	resp := GatewayResponse{
		RequestID:    "r1",
		Result:       "ok",
		Message:      "paired telegram:123",
		SessionToken: "session-abc",
	}
	rendered := RenderTelegramWebhookResponse(resp, "123")
	if rendered["method"] != "sendMessage" {
		t.Fatalf("expected method sendMessage, got %v", rendered["method"])
	}
	if rendered["chat_id"] != "123" {
		t.Fatalf("expected chat_id=123, got %v", rendered["chat_id"])
	}
	text, _ := rendered["text"].(string)
	if !strings.Contains(text, "session-abc") {
		t.Fatalf("expected session token in text, got %q", text)
	}
}

func TestRenderDiscordResponse(t *testing.T) {
	resp := GatewayResponse{Result: "ok", Message: "ok msg", DownloadURL: "/downloads/dl-1/file.zip"}
	rendered := RenderDiscordResponse(resp)
	content, _ := rendered["content"].(string)
	if !strings.Contains(content, "Download:") {
		t.Errorf("should contain download URL: %q", content)
	}
}

func TestRenderFeishuResponse(t *testing.T) {
	resp := GatewayResponse{Result: "error", ErrorCode: "E_TEST", Message: "test error"}
	rendered := RenderFeishuResponse(resp)
	if rendered["msg_type"] != "text" {
		t.Errorf("msg_type should be 'text', got %v", rendered["msg_type"])
	}
	content, _ := rendered["content"].(map[string]interface{})
	text, _ := content["text"].(string)
	if !strings.Contains(text, "E_TEST") {
		t.Errorf("should contain error code: %q", text)
	}
}

func TestToGatewayInput(t *testing.T) {
	nc := &NormalizedCommand{
		Provider:  "telegram",
		ChatID:    "123",
		RequestID: "req-1",
		Command:   "/agents",
		Args:      []string{},
	}
	got := ToGatewayInput(nc)
	if got != "telegram 123 req-1 /agents" {
		t.Errorf("ToGatewayInput: %q", got)
	}

	nc2 := &NormalizedCommand{
		Provider:  "discord",
		ChatID:    "ch",
		RequestID: "req-2",
		Command:   "/add",
		Args:      []string{"openclaw"},
	}
	got2 := ToGatewayInput(nc2)
	if got2 != "discord ch req-2 /add openclaw" {
		t.Errorf("ToGatewayInput with args: %q", got2)
	}
}
