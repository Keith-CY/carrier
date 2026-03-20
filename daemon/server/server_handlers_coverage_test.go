package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"carrier/baseagent"
	"carrier/daemon/internal/catalog"
	"carrier/daemon/internal/lifecycle"
	"carrier/daemon/internal/memory"
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
	lastReq    baseagent.ChatRequest
	resp       baseagent.ChatResponse
	err        error
	callCount  int
	chatHook   func(baseagent.ChatRequest)
	lastSpeak  baseagent.SpeechSynthesisRequest
	speakResp  baseagent.ChatResponse
	speakErr   error
	speakCalls int
}

func (f *fakeAgentChatRuntime) Chat(_ context.Context, req baseagent.ChatRequest) (baseagent.ChatResponse, error) {
	f.lastReq = req
	f.callCount++
	if f.chatHook != nil {
		f.chatHook(req)
	}
	return f.resp, f.err
}

func (f *fakeAgentChatRuntime) SpeakMedia(_ context.Context, req baseagent.SpeechSynthesisRequest) (baseagent.ChatResponse, error) {
	f.lastSpeak = req
	f.speakCalls++
	return f.speakResp, f.speakErr
}

func TestHandleAgentChatBranches(t *testing.T) {
	origHome := userHomeDirFunc
	t.Cleanup(func() {
		userHomeDirFunc = origHome
	})
	userHomeDirFunc = func() (string, error) { return t.TempDir(), nil }

	notAllowedRR := httptest.NewRecorder()
	handleAgentChat(nil, nil, "zeroclaw", notAllowedRR, httptest.NewRequest(http.MethodGet, "/chat", nil))
	if notAllowedRR.Code != http.StatusMethodNotAllowed {
		t.Fatalf("chat method check status=%d", notAllowedRR.Code)
	}

	unavailableRR := httptest.NewRecorder()
	handleAgentChat(nil, nil, "zeroclaw", unavailableRR, httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"message":"hi"}`)))
	if unavailableRR.Code != http.StatusServiceUnavailable {
		t.Fatalf("chat unavailable status=%d body=%s", unavailableRR.Code, unavailableRR.Body.String())
	}

	badBodyRR := httptest.NewRecorder()
	handleAgentChat(nil, &fakeAgentChatRuntime{}, "zeroclaw", badBodyRR, httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"message"`)))
	if badBodyRR.Code != http.StatusBadRequest {
		t.Fatalf("chat bad body status=%d body=%s", badBodyRR.Code, badBodyRR.Body.String())
	}

	missingMessageRR := httptest.NewRecorder()
	handleAgentChat(nil, &fakeAgentChatRuntime{}, "zeroclaw", missingMessageRR, httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"message":"   "}`)))
	if missingMessageRR.Code != http.StatusBadRequest {
		t.Fatalf("chat missing message status=%d body=%s", missingMessageRR.Code, missingMessageRR.Body.String())
	}

	runtime := &fakeAgentChatRuntime{
		resp: baseagent.ChatResponse{
			Message: "hello from local runtime",
			Action:  "chat",
			RichContent: &baseagent.RichOutboundMessage{
				Text:       "hello from local runtime",
				RenderMode: "rich_media",
				Blocks: []baseagent.ContentBlock{{
					Type:       "audio",
					OutputRole: "generated",
					Name:       "voice-note.ogg",
					MediaType:  "audio/ogg",
					URL:        "https://downloads.example.com/voice-note.ogg",
				}},
			},
		},
	}
	okReq := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"message":"hello","sessionId":"sess-1","provider":"openrouter","modelAlias":"flash","model":"google/gemini-2.0-flash-001"}`))
	okRR := httptest.NewRecorder()
	handleAgentChat(nil, runtime, "zeroclaw", okRR, okReq)
	if okRR.Code != http.StatusOK {
		t.Fatalf("chat ok status=%d body=%s", okRR.Code, okRR.Body.String())
	}
	if runtime.callCount != 1 {
		t.Fatalf("expected one runtime call, got %d", runtime.callCount)
	}
	if runtime.lastReq.Message != "hello" || runtime.lastReq.ChatID != "sess-1" || runtime.lastReq.Provider != "openrouter" || runtime.lastReq.ModelAlias != "flash" || runtime.lastReq.Model != "google/gemini-2.0-flash-001" {
		t.Fatalf("unexpected runtime request: %+v", runtime.lastReq)
	}
	if !strings.Contains(okRR.Body.String(), `"agentId":"zeroclaw"`) || !strings.Contains(okRR.Body.String(), `"sessionId":"sess-1"`) {
		t.Fatalf("unexpected chat response body=%s", okRR.Body.String())
	}
	if !strings.Contains(okRR.Body.String(), `"richContent"`) || !strings.Contains(okRR.Body.String(), `"renderMode":"rich_media"`) {
		t.Fatalf("expected rich content in chat response body=%s", okRR.Body.String())
	}
}

func TestHandleAgentChat_PassesAttachmentMetadataToRuntime(t *testing.T) {
	runtime := &fakeAgentChatRuntime{
		resp: baseagent.ChatResponse{Message: "accepted", Action: "chat"},
	}
	req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{
		"message":"hello with attachment",
		"sessionId":"sess-a",
		"attachments":[
			{"id":"tg-audio-unique-1","kind":"audio","name":"voice.ogg","mimeType":"audio/ogg","mediaType":"audio/ogg","externalId":"tg-audio-1","sourceMetadata":{"chat_id":"123","message_id":"456"}}
		]
	}`))
	rr := httptest.NewRecorder()
	handleAgentChat(nil, runtime, "openclaw", rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("chat status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(runtime.lastReq.Attachments) != 1 {
		t.Fatalf("expected one attachment, got %+v", runtime.lastReq.Attachments)
	}
	if runtime.lastReq.Attachments[0].Kind != "audio" || runtime.lastReq.Attachments[0].ExternalID != "tg-audio-1" {
		t.Fatalf("unexpected runtime attachments: %+v", runtime.lastReq.Attachments)
	}
	if runtime.lastReq.Attachments[0].ID != "tg-audio-unique-1" || runtime.lastReq.Attachments[0].MediaType != "audio/ogg" {
		t.Fatalf("unexpected attachment identity/media: %+v", runtime.lastReq.Attachments[0])
	}
	if runtime.lastReq.Attachments[0].SourceMetadata["chat_id"] != "123" || runtime.lastReq.Attachments[0].SourceMetadata["message_id"] != "456" {
		t.Fatalf("unexpected attachment source metadata: %+v", runtime.lastReq.Attachments[0].SourceMetadata)
	}
}

func TestHandleAgentChat_PassesSharedInstructionsAndRuntimeContextToRuntime(t *testing.T) {
	runtime := &fakeAgentChatRuntime{
		resp: baseagent.ChatResponse{Message: "accepted", Action: "chat"},
	}
	req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{
		"message":"hello with execution context",
		"sessionId":"sess-ctx",
		"sharedInstructions":[{"id":"ops","content":"Prefer deterministic changes."}],
		"runtimeContext":[{"key":"workflow.run_id","value":"run-123","class":"workflow","redactionMode":"hidden"}]
	}`))
	rr := httptest.NewRecorder()
	handleAgentChat(nil, runtime, "openclaw", rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("chat status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(runtime.lastReq.SharedInstructions) != 1 || runtime.lastReq.SharedInstructions[0].ID != "ops" {
		t.Fatalf("unexpected shared instructions: %+v", runtime.lastReq.SharedInstructions)
	}
	if len(runtime.lastReq.RuntimeContext) != 1 || runtime.lastReq.RuntimeContext[0].Key != "workflow.run_id" {
		t.Fatalf("unexpected runtime context: %+v", runtime.lastReq.RuntimeContext)
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
		_, _ = w.Write([]byte(`{"message":"proxied from zeroclaw","richContent":{"text":"proxied from zeroclaw","renderMode":"rich_media","blocks":[{"type":"audio","outputRole":"generated","name":"proxy-note.ogg","mediaType":"audio/ogg","url":"https://downloads.example.com/proxy-note.ogg"}]}}`))
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
	handleAgentChat(nil, runtime, "zeroclaw", rr, req)
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
	if !strings.Contains(rr.Body.String(), `"richContent"`) || !strings.Contains(rr.Body.String(), `"renderMode":"rich_media"`) {
		t.Fatalf("expected structured proxy response body=%s", rr.Body.String())
	}
}

func TestHandleAgentChatRefreshesPersistentMemoryBeforeTurn(t *testing.T) {
	store := memory.NewStore(memory.WithRootDir(t.TempDir()))
	svc := lifecycle.NewService(baseagent.NoopTriager{}, lifecycle.WithMemoryStore(store))
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatalf("register manifest: %v", err)
	}
	if err := store.AttachScope("openclaw", memory.Scope("shared:team")); err != nil {
		t.Fatalf("AttachScope(shared:team): %v", err)
	}
	if _, err := store.GrantScope("openclaw", memory.Scope("shared:team"), "tester", "seed shared scope"); err != nil {
		t.Fatalf("GrantScope(shared:team): %v", err)
	}
	if _, err := store.UpsertRecord(memory.UpsertRecordInput{
		ID:             "shared-team-1",
		Subject:        "openclaw",
		Scope:          memory.Scope("shared:team"),
		Type:           memory.RecordTypeFact,
		ContentSummary: "team timezone is tokyo",
	}); err != nil {
		t.Fatalf("UpsertRecord(shared:team): %v", err)
	}

	runtime := &fakeAgentChatRuntime{
		resp: baseagent.ChatResponse{Message: "refreshed", Action: "chat"},
	}
	req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"message":"hello","sessionId":"sess-refresh"}`))
	rr := httptest.NewRecorder()
	handleAgentChat(svc, runtime, "openclaw", rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("first chat status=%d body=%s", rr.Code, rr.Body.String())
	}

	lines, err := svc.Logs("openclaw", 200)
	if err != nil {
		t.Fatalf("Logs(openclaw): %v", err)
	}
	if got := countLogSubstring(lines, "memory effective view prepared"); got != 1 {
		t.Fatalf("expected one pre-turn refresh log after first turn, got %d logs=%v", got, lines)
	}

	if _, err := store.UpsertRecord(memory.UpsertRecordInput{
		ID:             "shared-team-1",
		Subject:        "openclaw",
		Scope:          memory.Scope("shared:team"),
		Type:           memory.RecordTypeFact,
		ContentSummary: "team timezone is osaka",
	}); err != nil {
		t.Fatalf("mutate shared record: %v", err)
	}

	rr = httptest.NewRecorder()
	handleAgentChat(svc, runtime, "openclaw", rr, httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"message":"hello again","sessionId":"sess-refresh"}`)))
	if rr.Code != http.StatusOK {
		t.Fatalf("second chat status=%d body=%s", rr.Code, rr.Body.String())
	}
	lines, err = svc.Logs("openclaw", 200)
	if err != nil {
		t.Fatalf("Logs(openclaw): %v", err)
	}
	if got := countLogSubstring(lines, "memory effective view prepared"); got != 2 {
		t.Fatalf("expected refresh before second turn, got %d logs=%v", got, lines)
	}
}

func TestHandleAgentChatSkipsRefreshForDelegatedSnapshotScope(t *testing.T) {
	store := memory.NewStore()
	svc := lifecycle.NewService(baseagent.NoopTriager{}, lifecycle.WithMemoryStore(store))
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatalf("register manifest: %v", err)
	}
	if _, err := store.GrantScope("parent", memory.Scope("shared:team"), "tester", "seed shared scope"); err != nil {
		t.Fatalf("GrantScope(shared:team): %v", err)
	}
	if _, err := store.UpsertRecord(memory.UpsertRecordInput{
		ID:             "shared-team-1",
		Subject:        "parent",
		Scope:          memory.Scope("shared:team"),
		Type:           memory.RecordTypeFact,
		ContentSummary: "team timezone is tokyo",
	}); err != nil {
		t.Fatalf("UpsertRecord(shared:team): %v", err)
	}
	snapshot, err := store.CreateSnapshotForInstance(context.Background(), memory.SnapshotOptions{
		Actor:            "tester",
		RequestID:        "req-chat-snapshot",
		SourceSubject:    "parent",
		SourceScopes:     []memory.Scope{memory.Scope("shared:team")},
		TargetInstanceID: "openclaw",
		Reason:           "delegated task",
	})
	if err != nil {
		t.Fatalf("CreateSnapshotForInstance: %v", err)
	}
	if err := store.MountSnapshot("openclaw", snapshot.ID); err != nil {
		t.Fatalf("MountSnapshot: %v", err)
	}

	runtime := &fakeAgentChatRuntime{
		resp: baseagent.ChatResponse{Message: "snapshot child", Action: "chat"},
	}
	rr := httptest.NewRecorder()
	handleAgentChat(svc, runtime, "openclaw", rr, httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"message":"hello","sessionId":"sess-snapshot"}`)))
	if rr.Code != http.StatusOK {
		t.Fatalf("chat status=%d body=%s", rr.Code, rr.Body.String())
	}
	if runtime.callCount != 1 {
		t.Fatalf("expected runtime call despite missing root dir, got %d", runtime.callCount)
	}
	if runtime.lastReq.MemorySubject != "openclaw" {
		t.Fatalf("expected delegated chat memory subject openclaw, got %+v", runtime.lastReq)
	}
}

func TestHandleAgentChatDoesNotRefreshMidTurn(t *testing.T) {
	store := memory.NewStore(memory.WithRootDir(t.TempDir()))
	svc := lifecycle.NewService(baseagent.NoopTriager{}, lifecycle.WithMemoryStore(store))
	if err := svc.RegisterManifest(catalog.OpenClawManifest()); err != nil {
		t.Fatalf("register manifest: %v", err)
	}
	if err := store.AttachScope("openclaw", memory.Scope("shared:team")); err != nil {
		t.Fatalf("AttachScope(shared:team): %v", err)
	}
	if _, err := store.GrantScope("openclaw", memory.Scope("shared:team"), "tester", "seed shared scope"); err != nil {
		t.Fatalf("GrantScope(shared:team): %v", err)
	}
	if _, err := store.UpsertRecord(memory.UpsertRecordInput{
		ID:             "shared-team-1",
		Subject:        "openclaw",
		Scope:          memory.Scope("shared:team"),
		Type:           memory.RecordTypeFact,
		ContentSummary: "team timezone is tokyo",
	}); err != nil {
		t.Fatalf("UpsertRecord(shared:team): %v", err)
	}

	runtime := &fakeAgentChatRuntime{
		resp: baseagent.ChatResponse{Message: "single refresh", Action: "chat"},
		chatHook: func(_ baseagent.ChatRequest) {
			_, _ = store.UpsertRecord(memory.UpsertRecordInput{
				ID:             "shared-team-1",
				Subject:        "openclaw",
				Scope:          memory.Scope("shared:team"),
				Type:           memory.RecordTypeFact,
				ContentSummary: "team timezone is osaka",
			})
		},
	}
	rr := httptest.NewRecorder()
	handleAgentChat(svc, runtime, "openclaw", rr, httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"message":"hello","sessionId":"sess-midturn"}`)))
	if rr.Code != http.StatusOK {
		t.Fatalf("chat status=%d body=%s", rr.Code, rr.Body.String())
	}
	lines, err := svc.Logs("openclaw", 200)
	if err != nil {
		t.Fatalf("Logs(openclaw): %v", err)
	}
	if got := countLogSubstring(lines, "memory effective view prepared"); got != 1 {
		t.Fatalf("expected exactly one pre-turn refresh during chat, got %d logs=%v", got, lines)
	}
}

func countLogSubstring(lines []string, needle string) int {
	count := 0
	for _, line := range lines {
		if strings.Contains(line, needle) {
			count++
		}
	}
	return count
}

func TestHandleAgentChat_ProxiesManagedZeroClawAgentCLIForIsolatedInstance(t *testing.T) {
	origHome := userHomeDirFunc
	origLookPath := managedLookPath
	t.Cleanup(func() {
		userHomeDirFunc = origHome
		managedLookPath = origLookPath
	})

	home := t.TempDir()
	userHomeDirFunc = func() (string, error) { return home, nil }
	if err := os.MkdirAll(filepath.Join(home, ".zeroclaw"), 0o700); err != nil {
		t.Fatalf("mkdir zeroclaw dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".zeroclaw", "config.toml"), []byte(`
default_provider = "openrouter"
default_model = "deepseek/deepseek-chat-v3-0324"

[provider_profiles.openrouter-fast]
model_alias = "flash"
model = "google/gemini-2.0-flash-001"

[provider_profiles.openrouter-safe]
model_alias = "flash"
model = "deepseek/deepseek-chat-v3-0324"

[gateway]
host = "127.0.0.1"
port = 9091
require_pairing = false
`), 0o600); err != nil {
		t.Fatalf("write zeroclaw config: %v", err)
	}

	statePath := filepath.Join(t.TempDir(), "lifecycle-state.json")
	persisted := map[string]lifecycle.PersistedAgentState{
		"zeroclaw": {
			ID:               "zeroclaw",
			Installed:        true,
			RuntimeState:     string(lifecycle.RuntimeStateStopped),
			Isolated:         true,
			LimaInstanceName: "carrier-zeroclaw-a3f2",
			LastTransition:   time.Now().UTC(),
		},
	}
	raw, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		t.Fatalf("marshal persisted state: %v", err)
	}
	if err := os.WriteFile(statePath, raw, 0o600); err != nil {
		t.Fatalf("write persisted state: %v", err)
	}

	svc := lifecycle.NewService(baseagent.NoopTriager{}, lifecycle.WithStateFile(statePath))
	if err := svc.RegisterManifest(catalog.ZeroClawManifest()); err != nil {
		t.Fatalf("register zeroclaw manifest: %v", err)
	}

	limactlDir := t.TempDir()
	limactlPath := filepath.Join(limactlDir, "limactl")
	argsPath := filepath.Join(limactlDir, "args.txt")
	configPath := filepath.Join(limactlDir, "config.toml")
	t.Setenv("TEST_ZEROCLAW_ARGS_PATH", argsPath)
	t.Setenv("TEST_ZEROCLAW_CONFIG_PATH", configPath)
	if err := os.WriteFile(limactlPath, []byte(`#!/bin/sh
printf '%s\n' "$@" >"$TEST_ZEROCLAW_ARGS_PATH"
for arg in "$@"; do
  LAST_ARG="$arg"
done
if [ -n "$LAST_ARG" ]; then
  if ! printf '%s' "$LAST_ARG" | base64 -d >"$TEST_ZEROCLAW_CONFIG_PATH" 2>/dev/null; then
    printf '%s' "$LAST_ARG" | base64 -D >"$TEST_ZEROCLAW_CONFIG_PATH"
  fi
fi
printf '\033[2m2026-03-10T07:00:16.309574Z\033[0m \033[32mINFO\033[0m zeroclaw::config::schema: Config loaded\n'
printf '{"message":"Tokyo weather is mild with a chance of rain.","richContent":{"text":"Tokyo weather is mild with a chance of rain.","renderMode":"rich_media","blocks":[{"type":"audio","outputRole":"generated","name":"weather-note.ogg","mediaType":"audio/ogg","url":"https://downloads.example.com/weather-note.ogg"}]}}\n'
`), 0o755); err != nil {
		t.Fatalf("write fake limactl: %v", err)
	}
	managedLookPath = func(file string) (string, error) {
		if file == "limactl" {
			return limactlPath, nil
		}
		return "", fmt.Errorf("unexpected executable lookup: %s", file)
	}

	runtime := &fakeAgentChatRuntime{
		resp: baseagent.ChatResponse{Message: "should not be used", Action: "chat"},
	}
	req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"message":"weather please","sessionId":"sess-z","provider":"openrouter","modelAlias":"flash","model":"google/gemini-2.0-flash-001"}`))
	rr := httptest.NewRecorder()
	handleAgentChat(svc, runtime, "zeroclaw", rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("chat status=%d body=%s", rr.Code, rr.Body.String())
	}
	if runtime.callCount != 0 {
		t.Fatalf("expected runtime fallback not to be called, got %d", runtime.callCount)
	}
	if !strings.Contains(rr.Body.String(), `Tokyo weather is mild with a chance of rain.`) {
		t.Fatalf("unexpected cli proxy response body=%s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"richContent"`) || !strings.Contains(rr.Body.String(), `"renderMode":"rich_media"`) {
		t.Fatalf("expected structured cli proxy response body=%s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), `Config loaded`) || strings.Contains(rr.Body.String(), `zeroclaw::config::schema`) {
		t.Fatalf("expected cli proxy response to strip zeroclaw log lines, got %s", rr.Body.String())
	}

	argsRaw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read fake limactl args: %v", err)
	}
	argsText := string(argsRaw)
	if !strings.Contains(argsText, "shell") || !strings.Contains(argsText, "carrier-zeroclaw-a3f2") {
		t.Fatalf("unexpected limactl args=%q", argsText)
	}
	if !strings.Contains(argsText, "agent") || !strings.Contains(argsText, "-m") || !strings.Contains(argsText, "--json") || !strings.Contains(argsText, "--no-color") || !strings.Contains(argsText, "weather please") {
		t.Fatalf("expected zeroclaw agent single-shot args, got %q", argsText)
	}
	cfgRaw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read rendered zeroclaw config: %v", err)
	}
	if cfgText := string(cfgRaw); !strings.Contains(cfgText, `default_model = "google/gemini-2.0-flash-001"`) {
		t.Fatalf("expected selected model in override config, got:\n%s", cfgText)
	}
}

func TestHandleAgentMediaSpeak_Branches(t *testing.T) {
	notAllowedRR := httptest.NewRecorder()
	handleAgentMedia(nil, "zeroclaw", "speak", notAllowedRR, httptest.NewRequest(http.MethodGet, "/media/speak", nil))
	if notAllowedRR.Code != http.StatusMethodNotAllowed {
		t.Fatalf("media speak method status=%d body=%s", notAllowedRR.Code, notAllowedRR.Body.String())
	}

	unavailableRR := httptest.NewRecorder()
	handleAgentMedia(nil, "zeroclaw", "speak", unavailableRR, httptest.NewRequest(http.MethodPost, "/media/speak", strings.NewReader(`{"text":"hello"}`)))
	if unavailableRR.Code != http.StatusServiceUnavailable {
		t.Fatalf("media speak unavailable status=%d body=%s", unavailableRR.Code, unavailableRR.Body.String())
	}

	badBodyRR := httptest.NewRecorder()
	handleAgentMedia(&fakeAgentChatRuntime{}, "zeroclaw", "speak", badBodyRR, httptest.NewRequest(http.MethodPost, "/media/speak", strings.NewReader(`{"text"`)))
	if badBodyRR.Code != http.StatusBadRequest {
		t.Fatalf("media speak bad body status=%d body=%s", badBodyRR.Code, badBodyRR.Body.String())
	}

	missingTextRR := httptest.NewRecorder()
	handleAgentMedia(&fakeAgentChatRuntime{}, "zeroclaw", "speak", missingTextRR, httptest.NewRequest(http.MethodPost, "/media/speak", strings.NewReader(`{"text":"   "}`)))
	if missingTextRR.Code != http.StatusBadRequest {
		t.Fatalf("media speak missing text status=%d body=%s", missingTextRR.Code, missingTextRR.Body.String())
	}

	runtime := &fakeAgentChatRuntime{
		speakResp: baseagent.ChatResponse{
			Message: "Generated audio: speech.mp3",
			RichContent: &baseagent.RichOutboundMessage{
				Text:       "Generated audio: speech.mp3",
				RenderMode: "rich_media",
				Attachments: []baseagent.AttachmentRef{{
					ID:         "speech-1",
					Kind:       "audio",
					OutputRole: "generated",
					Name:       "speech.mp3",
					Path:       "/tmp/speech.mp3",
					MediaType:  "audio/mpeg",
				}},
				Blocks: []baseagent.ContentBlock{{
					Type:         "audio",
					OutputRole:   "generated",
					Name:         "speech.mp3",
					AttachmentID: "speech-1",
					MediaType:    "audio/mpeg",
				}},
			},
		},
	}
	okReq := httptest.NewRequest(http.MethodPost, "/media/speak", strings.NewReader(`{"text":"Carrier speech smoke works.","voice":"alloy","format":"mp3"}`))
	okRR := httptest.NewRecorder()
	handleAgentMedia(runtime, "zeroclaw", "speak", okRR, okReq)
	if okRR.Code != http.StatusOK {
		t.Fatalf("media speak status=%d body=%s", okRR.Code, okRR.Body.String())
	}
	if runtime.speakCalls != 1 {
		t.Fatalf("expected one speak call, got %d", runtime.speakCalls)
	}
	if runtime.lastSpeak.Text != "Carrier speech smoke works." || runtime.lastSpeak.Voice != "alloy" || runtime.lastSpeak.Format != "mp3" {
		t.Fatalf("unexpected speak request: %+v", runtime.lastSpeak)
	}
	if !strings.Contains(okRR.Body.String(), `"richContent"`) || !strings.Contains(okRR.Body.String(), `"renderMode":"rich_media"`) {
		t.Fatalf("expected rich content in media speak response body=%s", okRR.Body.String())
	}
}

func TestHandleAgentChat_PersistsManagedZeroClawModelRuntimeTelemetry(t *testing.T) {
	origHome := userHomeDirFunc
	origLookPath := managedLookPath
	t.Cleanup(func() {
		userHomeDirFunc = origHome
		managedLookPath = origLookPath
	})

	home := t.TempDir()
	userHomeDirFunc = func() (string, error) { return home, nil }
	storePath := filepath.Join(home, ".carrier", "instances.json")
	t.Setenv("CARRIER_INSTANCE_STORE", storePath)
	if err := os.MkdirAll(filepath.Dir(storePath), 0o700); err != nil {
		t.Fatalf("mkdir carrier dir: %v", err)
	}
	if err := os.WriteFile(storePath, []byte(`{"instances":[{"id":"zeroclaw-local","agent_id":"zeroclaw","updated_at":"2026-03-13T00:00:00Z"}]}`), 0o600); err != nil {
		t.Fatalf("write managed instance store: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".zeroclaw"), 0o700); err != nil {
		t.Fatalf("mkdir zeroclaw dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".zeroclaw", "config.toml"), []byte(`
default_provider = "openrouter"
default_model = "google/gemini-2.0-flash-001"

[provider_profiles.openrouter-fast]
model_alias = "flash"
model = "google/gemini-2.0-flash-001"
provider = "openrouter"
provider_id = "openrouter"

[provider_profiles.openrouter-safe]
model_alias = "flash"
model = "deepseek/deepseek-chat-v3-0324"
provider = "openrouter"
provider_id = "openrouter"

[gateway]
host = "127.0.0.1"
port = 9091
require_pairing = false
`), 0o600); err != nil {
		t.Fatalf("write zeroclaw config: %v", err)
	}

	statePath := filepath.Join(t.TempDir(), "lifecycle-state.json")
	persisted := map[string]lifecycle.PersistedAgentState{
		"zeroclaw": {
			ID:               "zeroclaw",
			Installed:        true,
			RuntimeState:     string(lifecycle.RuntimeStateStopped),
			Isolated:         true,
			LimaInstanceName: "carrier-zeroclaw-a3f2",
			LastTransition:   time.Now().UTC(),
		},
	}
	raw, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		t.Fatalf("marshal persisted state: %v", err)
	}
	if err := os.WriteFile(statePath, raw, 0o600); err != nil {
		t.Fatalf("write persisted state: %v", err)
	}

	svc := lifecycle.NewService(baseagent.NoopTriager{}, lifecycle.WithStateFile(statePath))
	if err := svc.RegisterManifest(catalog.ZeroClawManifest()); err != nil {
		t.Fatalf("register zeroclaw manifest: %v", err)
	}

	limactlDir := t.TempDir()
	limactlPath := filepath.Join(limactlDir, "limactl")
	if err := os.WriteFile(limactlPath, []byte(`#!/bin/sh
printf 'managed reply\n'
`), 0o755); err != nil {
		t.Fatalf("write fake limactl: %v", err)
	}
	managedLookPath = func(file string) (string, error) {
		if file == "limactl" {
			return limactlPath, nil
		}
		return "", fmt.Errorf("unexpected executable lookup: %s", file)
	}

	req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"message":"summarize","sessionId":"sess-z","provider":"openrouter","modelAlias":"flash","model":"deepseek/deepseek-chat-v3-0324"}`))
	rr := httptest.NewRecorder()
	handleAgentChat(svc, &fakeAgentChatRuntime{}, "zeroclaw", rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("chat status=%d body=%s", rr.Code, rr.Body.String())
	}

	storeRaw, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("read managed instance store: %v", err)
	}
	var store struct {
		Instances []struct {
			AgentID               string         `json:"agent_id"`
			ModelSelectionCursors map[string]int `json:"model_selection_cursors"`
			ModelRuntime          struct {
				RequestedAlias    string `json:"requested_alias"`
				RequestedModel    string `json:"requested_model"`
				ResolvedModel     string `json:"resolved_model"`
				ResolvedProfile   string `json:"resolved_profile"`
				FallbackGroup     string `json:"fallback_group"`
				SelectionStrategy string `json:"selection_strategy"`
				SelectionOrdinal  int    `json:"selection_ordinal"`
				OverrideHit       bool   `json:"override_hit"`
				FallbackHit       bool   `json:"fallback_hit"`
				LastRunAt         string `json:"last_run_at"`
			} `json:"model_runtime"`
		} `json:"instances"`
	}
	if err := json.Unmarshal(storeRaw, &store); err != nil {
		t.Fatalf("unmarshal managed instance store: %v raw=%s", err, string(storeRaw))
	}
	if len(store.Instances) != 1 {
		t.Fatalf("expected one managed instance, got %+v", store.Instances)
	}
	runtimeMeta := store.Instances[0].ModelRuntime
	if runtimeMeta.RequestedAlias != "flash" || runtimeMeta.RequestedModel != "deepseek/deepseek-chat-v3-0324" || runtimeMeta.ResolvedModel != "deepseek/deepseek-chat-v3-0324" {
		t.Fatalf("unexpected model runtime metadata: %+v", runtimeMeta)
	}
	if runtimeMeta.ResolvedProfile != "openrouter-safe" || runtimeMeta.SelectionStrategy != "explicit_model" || runtimeMeta.SelectionOrdinal != 1 {
		t.Fatalf("unexpected model runtime selection trace: %+v", runtimeMeta)
	}
	if runtimeMeta.FallbackGroup != "openrouter:flash" || !runtimeMeta.OverrideHit || !runtimeMeta.FallbackHit || strings.TrimSpace(runtimeMeta.LastRunAt) == "" {
		t.Fatalf("unexpected model runtime flags: %+v", runtimeMeta)
	}
	if got := store.Instances[0].ModelSelectionCursors["openrouter:flash"]; got != 0 {
		t.Fatalf("unexpected stored cursor after explicit model selection: %d", got)
	}
}

func TestResolveManagedZeroClawSelectedModel_UsesAliasProfile(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "instances.json")
	t.Setenv("CARRIER_INSTANCE_STORE", storePath)
	if err := os.WriteFile(storePath, []byte(`{"instances":[{"agent_id":"zeroclaw"}]}`), 0o600); err != nil {
		t.Fatalf("write managed instance store: %v", err)
	}
	cfg := zeroclawLocalConfig{
		DefaultProvider: "openrouter",
		Profiles: []zeroclawProviderProfile{
			{
				SectionName: "openrouter-fast",
				ModelAlias:  "flash",
				Model:       "google/gemini-2.0-flash-001",
				Provider:    "openrouter",
				ProviderID:  "openrouter",
			},
			{
				SectionName: "openrouter-safe",
				ModelAlias:  "flash",
				Model:       "deepseek/deepseek-chat-v3-0324",
				Provider:    "openrouter",
				ProviderID:  "openrouter",
			},
		},
	}

	first, err := resolveManagedZeroClawModelSelection("zeroclaw", cfg, "openrouter", "flash", "")
	if err != nil {
		t.Fatalf("resolveManagedZeroClawModelSelection first returned error: %v", err)
	}
	if first.ResolvedModel != "google/gemini-2.0-flash-001" || first.ResolvedProfile != "openrouter-fast" {
		t.Fatalf("first selection = %+v", first)
	}
	if first.SelectionStrategy != "round_robin" || first.SelectionOrdinal != 0 || first.FallbackHit {
		t.Fatalf("unexpected first selection trace: %+v", first)
	}
	if err := persistManagedAgentModelRuntime("zeroclaw", first); err != nil {
		t.Fatalf("persistManagedAgentModelRuntime first: %v", err)
	}

	second, err := resolveManagedZeroClawModelSelection("zeroclaw", cfg, "openrouter", "flash", "")
	if err != nil {
		t.Fatalf("resolveManagedZeroClawModelSelection second returned error: %v", err)
	}
	if second.ResolvedModel != "deepseek/deepseek-chat-v3-0324" || second.ResolvedProfile != "openrouter-safe" {
		t.Fatalf("second selection = %+v", second)
	}
	if second.SelectionStrategy != "round_robin" || second.SelectionOrdinal != 1 || !second.FallbackHit {
		t.Fatalf("unexpected second selection trace: %+v", second)
	}
	if err := persistManagedAgentModelRuntime("zeroclaw", second); err != nil {
		t.Fatalf("persistManagedAgentModelRuntime second: %v", err)
	}

	storeRaw, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("read managed instance store: %v", err)
	}
	if !strings.Contains(string(storeRaw), `"selection_strategy": "round_robin"`) || !strings.Contains(string(storeRaw), `"selection_ordinal": 1`) {
		t.Fatalf("managed instance store missing selection trace: %s", string(storeRaw))
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
