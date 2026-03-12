package gateway

import (
	"encoding/json"
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

func TestHandleWebUIAgentCronPassesThroughToDaemon(t *testing.T) {
	var gotSchedule map[string]any
	var gotActor string
	var gotRequestID string
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/base-agent/cron/jobs":
			gotActor = r.Header.Get("X-Carrier-Actor")
			gotRequestID = r.Header.Get("X-Carrier-Request-Id")
			if got := r.URL.Query().Get("agentId"); got != "picoclaw" {
				t.Fatalf("agentId=%q want picoclaw", got)
			}
			_, _ = w.Write([]byte(`{"jobs":[{"id":"cron-1","agentId":"picoclaw","prompt":"check launcher","lastResult":"succeeded"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/base-agent/cron/schedule":
			_ = json.NewDecoder(r.Body).Decode(&gotSchedule)
			_, _ = w.Write([]byte(`{"id":"cron-2","agentId":"picoclaw","prompt":"check launcher","lastResult":"scheduled"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/base-agent/cron/cron-1/cancel":
			_, _ = w.Write([]byte(`{"id":"cron-1","agentId":"picoclaw","prompt":"check launcher","lastResult":"cancelled"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer daemon.Close()

	client := NewDaemonClient(daemon.URL, "", 0)

	listRec := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/agents/picoclaw/cron", nil)
	handleWebUIAgent(listRec, listReq, "req-cron-list", client)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	if !strings.Contains(listRec.Body.String(), `"id":"cron-1"`) {
		t.Fatalf("list body=%s", listRec.Body.String())
	}
	if gotActor != "webui:agents:cron:list" {
		t.Fatalf("list actor=%q want webui:agents:cron:list", gotActor)
	}
	if gotRequestID != "req-cron-list" {
		t.Fatalf("list request id=%q want req-cron-list", gotRequestID)
	}

	scheduleRec := httptest.NewRecorder()
	scheduleReq := httptest.NewRequest(http.MethodPost, "/api/v1/agents/picoclaw/cron", strings.NewReader(`{"message":"check launcher","provider":"openrouter","sessionId":"cron-sess"}`))
	scheduleReq.Header.Set("Content-Type", "application/json")
	handleWebUIAgent(scheduleRec, scheduleReq, "req-cron-schedule", client)
	if scheduleRec.Code != http.StatusOK {
		t.Fatalf("schedule status=%d body=%s", scheduleRec.Code, scheduleRec.Body.String())
	}
	if !strings.Contains(scheduleRec.Body.String(), `"id":"cron-2"`) {
		t.Fatalf("schedule body=%s", scheduleRec.Body.String())
	}
	if got := strings.TrimSpace(gotSchedule["agentId"].(string)); got != "picoclaw" {
		t.Fatalf("scheduled agentId=%q want picoclaw", got)
	}
	if got := strings.TrimSpace(gotSchedule["prompt"].(string)); got != "check launcher" {
		t.Fatalf("scheduled prompt=%q want check launcher", got)
	}
	if got := strings.TrimSpace(gotSchedule["sessionKey"].(string)); got != "openrouter:cron-sess" {
		t.Fatalf("scheduled sessionKey=%q want openrouter:cron-sess", got)
	}

	cancelRec := httptest.NewRecorder()
	cancelReq := httptest.NewRequest(http.MethodPost, "/api/v1/agents/picoclaw/cron/cron-1/cancel", nil)
	handleWebUIAgent(cancelRec, cancelReq, "req-cron-cancel", client)
	if cancelRec.Code != http.StatusOK {
		t.Fatalf("cancel status=%d body=%s", cancelRec.Code, cancelRec.Body.String())
	}
	if !strings.Contains(cancelRec.Body.String(), `"lastResult":"cancelled"`) {
		t.Fatalf("cancel body=%s", cancelRec.Body.String())
	}
}
