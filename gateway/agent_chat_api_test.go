package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleWebUIAgentChatPassesThroughToDaemon(t *testing.T) {
	var gotActor string
	var gotRequestID string
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agents/picoclaw/chat" {
			http.NotFound(w, r)
			return
		}
		gotActor = r.Header.Get("X-Carrier-Actor")
		gotRequestID = r.Header.Get("X-Carrier-Request-Id")
		_, _ = w.Write([]byte(`{"agentId":"picoclaw","sessionId":"sess-1","message":"pong"}`))
	}))
	defer daemon.Close()

	client := NewDaemonClient(daemon.URL, "", 0)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/picoclaw/chat", strings.NewReader(`{"message":"hello","provider":"openrouter"}`))
	req.Header.Set("Content-Type", "application/json")

	handleWebUIAgent(rec, req, "req-chat-ok", client)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"message":"pong"`) {
		t.Fatalf("body=%s", rec.Body.String())
	}
	if gotActor != "webui:agents:chat" {
		t.Fatalf("actor=%q want webui:agents:chat", gotActor)
	}
	if gotRequestID != "req-chat-ok" {
		t.Fatalf("request id=%q want req-chat-ok", gotRequestID)
	}
}
