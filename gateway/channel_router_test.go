package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestChannelRouterTelegramEnvelope(t *testing.T) {
	sessions := NewSessionStore("", 0, nil)
	t.Cleanup(sessions.Stop)
	session := sessions.CreateSession("telegram", "123")

	payload := map[string]interface{}{
		"update_id": float64(1001),
		"message": map[string]interface{}{
			"message_id": float64(99),
			"text":       "/agents",
			"chat": map[string]interface{}{
				"id": float64(123),
			},
		},
	}

	envelope := NormalizeTelegramInboundEnvelope(payload, sessions)
	if envelope == nil {
		t.Fatal("expected telegram envelope")
	}
	if envelope.Channel != "telegram" || envelope.ChatID != "123" {
		t.Fatalf("unexpected telegram identity: %+v", envelope)
	}
	if envelope.Kind != InboundEnvelopeKindCommand || envelope.Command != "/agents" {
		t.Fatalf("unexpected telegram command envelope: %+v", envelope)
	}
	if envelope.SessionToken != session.SessionToken {
		t.Fatalf("telegram session token = %q, want %q", envelope.SessionToken, session.SessionToken)
	}
	if envelope.RequestID == "" {
		t.Fatalf("expected telegram request ID, got %+v", envelope)
	}
}

func TestChannelRouterDiscordEnvelope(t *testing.T) {
	sessions := NewSessionStore("", 0, nil)
	t.Cleanup(sessions.Stop)
	session := sessions.CreateSession("discord", "discord-chat-3")

	payload := map[string]interface{}{
		"type":       float64(2),
		"id":         "i-2",
		"channel_id": "discord-chat-3",
		"data": map[string]interface{}{
			"name": "agents",
		},
	}

	envelope := NormalizeDiscordInboundEnvelope(payload, sessions)
	if envelope == nil {
		t.Fatal("expected discord envelope")
	}
	if envelope.Channel != "discord" || envelope.ChatID != "discord-chat-3" {
		t.Fatalf("unexpected discord identity: %+v", envelope)
	}
	if envelope.Kind != InboundEnvelopeKindCommand || envelope.Command != "/agents" {
		t.Fatalf("unexpected discord command envelope: %+v", envelope)
	}
	if envelope.SessionToken != session.SessionToken {
		t.Fatalf("discord session token = %q, want %q", envelope.SessionToken, session.SessionToken)
	}
	if envelope.Metadata["transport"] != "discord-webhook" {
		t.Fatalf("unexpected discord metadata: %+v", envelope.Metadata)
	}
}

func TestChannelRouterFeishuEnvelope(t *testing.T) {
	sessions := NewSessionStore("", 0, nil)
	t.Cleanup(sessions.Stop)
	session := sessions.CreateSession("feishu", "f-chat-1")

	payload := map[string]interface{}{
		"header": map[string]interface{}{
			"token":    "feishu-token",
			"event_id": "evt-1",
		},
		"event": map[string]interface{}{
			"message": map[string]interface{}{
				"chat_id":    "f-chat-1",
				"message_id": "m-1",
				"content":    "{\"text\":\"/agents\"}",
			},
		},
	}

	envelope := NormalizeFeishuInboundEnvelope(payload, sessions)
	if envelope == nil {
		t.Fatal("expected feishu envelope")
	}
	if envelope.Channel != "feishu" || envelope.ChatID != "f-chat-1" {
		t.Fatalf("unexpected feishu identity: %+v", envelope)
	}
	if envelope.Kind != InboundEnvelopeKindCommand || envelope.Command != "/agents" {
		t.Fatalf("unexpected feishu command envelope: %+v", envelope)
	}
	if envelope.SessionToken != session.SessionToken {
		t.Fatalf("feishu session token = %q, want %q", envelope.SessionToken, session.SessionToken)
	}
	if envelope.RequestID == "" {
		t.Fatalf("expected feishu request ID, got %+v", envelope)
	}
}

func TestChannelRouterDispatchesBaseagentChat(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, nil)
	defer srv.Close()

	for _, channel := range []string{"telegram", "discord", "feishu"} {
		envelope := &InboundChannelEnvelope{
			Channel:   channel,
			ChatID:    fmt.Sprintf("%s-chat", channel),
			RequestID: fmt.Sprintf("%s-req", channel),
			Kind:      InboundEnvelopeKindChat,
			Text:      "hello from channel",
		}
		resp := RouteInboundChannel(context.Background(), envelope, dc, sessions, downloads, nil, onboard)
		if resp.ErrorCode != "E_SESSION_REQUIRED" {
			t.Fatalf("%s chat route error = %q, want %q", channel, resp.ErrorCode, "E_SESSION_REQUIRED")
		}
	}
}

func TestChannelRouterDispatchesGatewayCommand(t *testing.T) {
	srv, dc, sessions, downloads, onboard := setupTestEnv(t, map[string]http.HandlerFunc{
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

	for _, channel := range []string{"telegram", "discord", "feishu"} {
		session := sessions.CreateSession(channel, channel+"-chat")
		envelope := &InboundChannelEnvelope{
			Channel:      channel,
			ChatID:       channel + "-chat",
			RequestID:    channel + "-req",
			SessionToken: session.SessionToken,
			Kind:         InboundEnvelopeKindCommand,
			Command:      "/agents",
		}
		resp := RouteInboundChannel(context.Background(), envelope, dc, sessions, downloads, nil, onboard)
		if resp.Result != "ok" {
			t.Fatalf("%s command route result = %q, message=%q", channel, resp.Result, resp.Message)
		}
		if resp.Message == "" {
			t.Fatalf("%s command route should return message", channel)
		}
	}
}
