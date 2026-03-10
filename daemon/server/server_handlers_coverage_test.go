package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"carrier/baseagent"
	"carrier/daemon/internal/lifecycle"
	"carrier/daemon/internal/messaging"
)

func TestHandleMessageSendAndInboxBranches(t *testing.T) {
	bus := messaging.NewMessageBus()

	notAllowedSend := httptest.NewRecorder()
	handleMessageSend(bus, "agent-a", notAllowedSend, httptest.NewRequest(http.MethodGet, "/send", nil))
	if notAllowedSend.Code != http.StatusMethodNotAllowed {
		t.Fatalf("send method check status=%d", notAllowedSend.Code)
	}

	sendReq := httptest.NewRequest(http.MethodPost, "/send", strings.NewReader(`{
		"id":"msg-1",
		"from":"tester",
		"payload":"hello"
	}`))
	sendRR := httptest.NewRecorder()
	handleMessageSend(bus, "agent-a", sendRR, sendReq)
	if sendRR.Code != http.StatusOK {
		t.Fatalf("send status=%d body=%s", sendRR.Code, sendRR.Body.String())
	}

	notAllowedInbox := httptest.NewRecorder()
	handleMessageInbox(bus, "agent-a", notAllowedInbox, httptest.NewRequest(http.MethodPost, "/inbox", nil))
	if notAllowedInbox.Code != http.StatusMethodNotAllowed {
		t.Fatalf("inbox method check status=%d", notAllowedInbox.Code)
	}

	inboxRR := httptest.NewRecorder()
	handleMessageInbox(bus, "agent-a", inboxRR, httptest.NewRequest(http.MethodGet, "/inbox", nil))
	if inboxRR.Code != http.StatusOK || !strings.Contains(inboxRR.Body.String(), "messages") {
		t.Fatalf("inbox status/body unexpected: %d %s", inboxRR.Code, inboxRR.Body.String())
	}

	emptyInboxRR := httptest.NewRecorder()
	handleMessageInbox(bus, "agent-a", emptyInboxRR, httptest.NewRequest(http.MethodGet, "/inbox", nil))
	if emptyInboxRR.Code != http.StatusOK || !strings.Contains(emptyInboxRR.Body.String(), `"messages":[]`) {
		t.Fatalf("empty inbox status/body unexpected: %d %s", emptyInboxRR.Code, emptyInboxRR.Body.String())
	}
}

func TestHandleMetricsAndConfigSetBranches(t *testing.T) {
	svc := lifecycle.NewService(baseagent.NoopTriager{})

	metricsMethodRR := httptest.NewRecorder()
	handleMetrics(svc, "agent-a", metricsMethodRR, httptest.NewRequest(http.MethodPost, "/metrics", nil))
	if metricsMethodRR.Code != http.StatusMethodNotAllowed {
		t.Fatalf("metrics method check status=%d", metricsMethodRR.Code)
	}

	metricsRR := httptest.NewRecorder()
	handleMetrics(svc, "missing-agent", metricsRR, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if metricsRR.Code == http.StatusOK {
		t.Fatalf("metrics should fail for missing agent, body=%s", metricsRR.Body.String())
	}

	configMethodRR := httptest.NewRecorder()
	handleConfigSet(svc, "agent-a", configMethodRR, httptest.NewRequest(http.MethodGet, "/config/set", nil))
	if configMethodRR.Code != http.StatusMethodNotAllowed {
		t.Fatalf("config method check status=%d", configMethodRR.Code)
	}

	configBadBodyRR := httptest.NewRecorder()
	handleConfigSet(svc, "agent-a", configBadBodyRR, httptest.NewRequest(http.MethodPost, "/config/set", strings.NewReader("{")))
	if configBadBodyRR.Code == http.StatusOK {
		t.Fatalf("config set should fail on malformed body")
	}

	configRR := httptest.NewRecorder()
	handleConfigSet(svc, "missing-agent", configRR, httptest.NewRequest(http.MethodPost, "/config/set", strings.NewReader(`{"changes":{"A":"1"}}`)))
	if configRR.Code == http.StatusOK {
		t.Fatalf("config set should fail for missing agent, body=%s", configRR.Body.String())
	}
}

type fakeAgentChatRuntime struct {
	lastReq   baseagent.ChatRequest
	resp      baseagent.ChatResponse
	err       error
	callCount int
}

func (f *fakeAgentChatRuntime) Chat(_ context.Context, req baseagent.ChatRequest) (baseagent.ChatResponse, error) {
	f.lastReq = req
	f.callCount++
	return f.resp, f.err
}

func TestHandleAgentChatBranches(t *testing.T) {
	origHome := userHomeDirFunc
	t.Cleanup(func() {
		userHomeDirFunc = origHome
	})
	userHomeDirFunc = func() (string, error) { return t.TempDir(), nil }

	notAllowedRR := httptest.NewRecorder()
	handleAgentChat(nil, "zeroclaw", notAllowedRR, httptest.NewRequest(http.MethodGet, "/chat", nil))
	if notAllowedRR.Code != http.StatusMethodNotAllowed {
		t.Fatalf("chat method check status=%d", notAllowedRR.Code)
	}

	unavailableRR := httptest.NewRecorder()
	handleAgentChat(nil, "zeroclaw", unavailableRR, httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"message":"hi"}`)))
	if unavailableRR.Code != http.StatusServiceUnavailable {
		t.Fatalf("chat unavailable status=%d body=%s", unavailableRR.Code, unavailableRR.Body.String())
	}

	badBodyRR := httptest.NewRecorder()
	handleAgentChat(&fakeAgentChatRuntime{}, "zeroclaw", badBodyRR, httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"message"`)))
	if badBodyRR.Code != http.StatusBadRequest {
		t.Fatalf("chat bad body status=%d body=%s", badBodyRR.Code, badBodyRR.Body.String())
	}

	missingMessageRR := httptest.NewRecorder()
	handleAgentChat(&fakeAgentChatRuntime{}, "zeroclaw", missingMessageRR, httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"message":"   "}`)))
	if missingMessageRR.Code != http.StatusBadRequest {
		t.Fatalf("chat missing message status=%d body=%s", missingMessageRR.Code, missingMessageRR.Body.String())
	}

	runtime := &fakeAgentChatRuntime{
		resp: baseagent.ChatResponse{Message: "hello from local runtime", Action: "chat"},
	}
	okReq := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"message":"hello","sessionId":"sess-1","provider":"openrouter"}`))
	okRR := httptest.NewRecorder()
	handleAgentChat(runtime, "zeroclaw", okRR, okReq)
	if okRR.Code != http.StatusOK {
		t.Fatalf("chat ok status=%d body=%s", okRR.Code, okRR.Body.String())
	}
	if runtime.callCount != 1 {
		t.Fatalf("expected one runtime call, got %d", runtime.callCount)
	}
	if runtime.lastReq.Message != "hello" || runtime.lastReq.ChatID != "sess-1" || runtime.lastReq.Provider != "openrouter" {
		t.Fatalf("unexpected runtime request: %+v", runtime.lastReq)
	}
	if !strings.Contains(okRR.Body.String(), `"agentId":"zeroclaw"`) || !strings.Contains(okRR.Body.String(), `"sessionId":"sess-1"`) {
		t.Fatalf("unexpected chat response body=%s", okRR.Body.String())
	}
}

func TestHandleAgentChat_ProxiesManagedZeroClawGateway(t *testing.T) {
	origHome := userHomeDirFunc
	t.Cleanup(func() {
		userHomeDirFunc = origHome
	})

	var seenPath string
	var seenBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		seenBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"proxied from zeroclaw"}`))
	}))
	defer srv.Close()

	host, portText, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}

	home := t.TempDir()
	userHomeDirFunc = func() (string, error) { return home, nil }
	if err := os.MkdirAll(filepath.Join(home, ".zeroclaw"), 0o700); err != nil {
		t.Fatalf("mkdir zeroclaw dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".zeroclaw", "config.toml"), []byte(fmt.Sprintf(`
default_provider = "openrouter"
[gateway]
host = %q
port = %s
require_pairing = false
`, host, portText)), 0o600); err != nil {
		t.Fatalf("write zeroclaw config: %v", err)
	}

	runtime := &fakeAgentChatRuntime{
		resp: baseagent.ChatResponse{Message: "should not be used", Action: "chat"},
	}
	req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"message":"hello from proxy","sessionId":"sess-z"}`))
	rr := httptest.NewRecorder()
	handleAgentChat(runtime, "zeroclaw", rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("chat status=%d body=%s", rr.Code, rr.Body.String())
	}
	if runtime.callCount != 0 {
		t.Fatalf("expected runtime fallback not to be called, got %d", runtime.callCount)
	}
	if seenPath != "/webhook" {
		t.Fatalf("proxy path=%q, want /webhook", seenPath)
	}
	if !strings.Contains(seenBody, `"message":"hello from proxy"`) {
		t.Fatalf("proxy body=%q", seenBody)
	}
	if !strings.Contains(rr.Body.String(), `"message":"proxied from zeroclaw"`) {
		t.Fatalf("unexpected proxy response body=%s", rr.Body.String())
	}
}

func TestParseAgentMessagingPath(t *testing.T) {
	agentID, action, ok := parseAgentMessagingPath("/api/v1/agents/agent-a/messages/send")
	if !ok || agentID != "agent-a" || action != "send" {
		t.Fatalf("expected valid send path parse, got ok=%v agent=%q action=%q", ok, agentID, action)
	}

	if _, _, ok := parseAgentMessagingPath("/api/v1/agents/agent-a/messages/unknown"); ok {
		t.Fatal("expected invalid action to fail")
	}
	if _, _, ok := parseAgentMessagingPath("/api/v1/agents//messages/send"); ok {
		t.Fatal("expected empty agent id to fail")
	}
	if _, _, ok := parseAgentMessagingPath("/wrong/prefix"); ok {
		t.Fatal("expected wrong prefix to fail")
	}
}
