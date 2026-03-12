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
	var gotBody map[string]any
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agents/picoclaw/chat" {
			http.NotFound(w, r)
			return
		}
		gotActor = r.Header.Get("X-Carrier-Actor")
		gotRequestID = r.Header.Get("X-Carrier-Request-Id")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_, _ = w.Write([]byte(`{"agentId":"picoclaw","sessionId":"sess-1","message":"pong"}`))
	}))
	defer daemon.Close()

	client := NewDaemonClient(daemon.URL, "", 0)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/picoclaw/chat", strings.NewReader(`{"message":"hello","provider":"openrouter","modelAlias":"flash","model":"google/gemini-2.0-flash-001"}`))
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
	if gotBody["modelAlias"] != "flash" || gotBody["model"] != "google/gemini-2.0-flash-001" {
		t.Fatalf("unexpected forwarded body: %#v", gotBody)
	}
}

func TestHandleWebUIAgentCronPassesThroughToDaemon(t *testing.T) {
	var gotSchedule map[string]any
	var gotActor string
	var gotRequestID string
	var runCalls int
	var pauseCalls int
	var resumeCalls int
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
		case r.Method == http.MethodPost && r.URL.Path == "/api/base-agent/cron/cron-1/run":
			runCalls++
			_, _ = w.Write([]byte(`{"id":"cron-1","agentId":"picoclaw","prompt":"check launcher","lastResult":"succeeded","history":[{"trigger":"manual","result":"succeeded"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/base-agent/cron/cron-1/pause":
			pauseCalls++
			_, _ = w.Write([]byte(`{"id":"cron-1","agentId":"picoclaw","prompt":"check launcher","lastResult":"paused","paused":true}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/base-agent/cron/cron-1/resume":
			resumeCalls++
			_, _ = w.Write([]byte(`{"id":"cron-1","agentId":"picoclaw","prompt":"check launcher","lastResult":"resumed","paused":false}`))
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

	runRec := httptest.NewRecorder()
	runReq := httptest.NewRequest(http.MethodPost, "/api/v1/agents/picoclaw/cron/cron-1/run", nil)
	handleWebUIAgent(runRec, runReq, "req-cron-run", client)
	if runRec.Code != http.StatusOK {
		t.Fatalf("run status=%d body=%s", runRec.Code, runRec.Body.String())
	}
	if runCalls != 1 || !strings.Contains(runRec.Body.String(), `"trigger":"manual"`) {
		t.Fatalf("run body=%s runCalls=%d", runRec.Body.String(), runCalls)
	}

	pauseRec := httptest.NewRecorder()
	pauseReq := httptest.NewRequest(http.MethodPost, "/api/v1/agents/picoclaw/cron/cron-1/pause", nil)
	handleWebUIAgent(pauseRec, pauseReq, "req-cron-pause", client)
	if pauseRec.Code != http.StatusOK {
		t.Fatalf("pause status=%d body=%s", pauseRec.Code, pauseRec.Body.String())
	}
	if pauseCalls != 1 || !strings.Contains(pauseRec.Body.String(), `"paused":true`) {
		t.Fatalf("pause body=%s pauseCalls=%d", pauseRec.Body.String(), pauseCalls)
	}

	resumeRec := httptest.NewRecorder()
	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/agents/picoclaw/cron/cron-1/resume", nil)
	handleWebUIAgent(resumeRec, resumeReq, "req-cron-resume", client)
	if resumeRec.Code != http.StatusOK {
		t.Fatalf("resume status=%d body=%s", resumeRec.Code, resumeRec.Body.String())
	}
	if resumeCalls != 1 || !strings.Contains(resumeRec.Body.String(), `"lastResult":"resumed"`) {
		t.Fatalf("resume body=%s resumeCalls=%d", resumeRec.Body.String(), resumeCalls)
	}
}
