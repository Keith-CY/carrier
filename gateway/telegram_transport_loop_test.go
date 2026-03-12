package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type scriptedTelegramAPI struct {
	getUpdatesFn   func(ctx context.Context, offset int64, timeoutSec int) ([]map[string]interface{}, error)
	sendMessageFn  func(ctx context.Context, chatID, text string, disableWebPagePreview bool) error
	sendPhotoFn    func(ctx context.Context, chatID, photo, caption string) error
	sendDocumentFn func(ctx context.Context, chatID, document, caption string) error
	getFileFn      func(ctx context.Context, fileID string) (telegramFileInfo, error)
	downloadFileFn func(ctx context.Context, filePath string) ([]byte, error)
}

func (s *scriptedTelegramAPI) SetWebhook(_ context.Context, _ string, _ string) error {
	return nil
}

func (s *scriptedTelegramAPI) GetWebhookInfo(_ context.Context) (telegramWebhookInfo, error) {
	return telegramWebhookInfo{}, nil
}

func (s *scriptedTelegramAPI) DeleteWebhook(_ context.Context) error {
	return nil
}

func (s *scriptedTelegramAPI) GetUpdates(ctx context.Context, offset int64, timeoutSec int) ([]map[string]interface{}, error) {
	if s.getUpdatesFn != nil {
		return s.getUpdatesFn(ctx, offset, timeoutSec)
	}
	return nil, nil
}

func (s *scriptedTelegramAPI) GetFile(ctx context.Context, fileID string) (telegramFileInfo, error) {
	if s.getFileFn != nil {
		return s.getFileFn(ctx, fileID)
	}
	return telegramFileInfo{}, errors.New("unexpected GetFile call")
}

func (s *scriptedTelegramAPI) DownloadFile(ctx context.Context, filePath string) ([]byte, error) {
	if s.downloadFileFn != nil {
		return s.downloadFileFn(ctx, filePath)
	}
	return nil, errors.New("unexpected DownloadFile call")
}

func (s *scriptedTelegramAPI) SendMessage(ctx context.Context, chatID, text string, disableWebPagePreview bool) error {
	if s.sendMessageFn != nil {
		return s.sendMessageFn(ctx, chatID, text, disableWebPagePreview)
	}
	return nil
}

func (s *scriptedTelegramAPI) SendPhoto(ctx context.Context, chatID, photo, caption string) error {
	if s.sendPhotoFn != nil {
		return s.sendPhotoFn(ctx, chatID, photo, caption)
	}
	return nil
}

func (s *scriptedTelegramAPI) SendDocument(ctx context.Context, chatID, document, caption string) error {
	if s.sendDocumentFn != nil {
		return s.sendDocumentFn(ctx, chatID, document, caption)
	}
	return nil
}

func (s *scriptedTelegramAPI) SetMyCommands(_ context.Context, _ []telegramBotCommand) error {
	return nil
}

func TestStartTelegramTransport_AutoMissingTokenDisables(t *testing.T) {
	cfg := &GatewayConfig{
		TelegramTransportMode: telegramTransportAuto,
		TelegramBotToken:      "",
	}

	if err := startTelegramTransport(context.Background(), cfg, nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("startTelegramTransport returned error: %v", err)
	}

	status := snapshotTelegramTransportStatus()
	if status.RequestedMode != telegramTransportAuto {
		t.Fatalf("requested mode = %q, want %q", status.RequestedMode, telegramTransportAuto)
	}
	if status.SelectedMode != "disabled" {
		t.Fatalf("selected mode = %q, want disabled", status.SelectedMode)
	}
	if status.ReasonCode != telegramFallbackTokenMissing {
		t.Fatalf("reason code = %q, want %q", status.ReasonCode, telegramFallbackTokenMissing)
	}
}

func TestStartTelegramTransport_InvalidModeSetsErrorStatus(t *testing.T) {
	cfg := &GatewayConfig{
		TelegramTransportMode: "not-a-real-mode",
		TelegramBotToken:      "TOKEN",
	}

	err := startTelegramTransport(context.Background(), cfg, nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected invalid mode error")
	}

	status := snapshotTelegramTransportStatus()
	if status.SelectedMode != "error" {
		t.Fatalf("selected mode = %q, want error", status.SelectedMode)
	}
	if status.ReasonCode != "RESOLUTION_FAILED" {
		t.Fatalf("reason code = %q, want RESOLUTION_FAILED", status.ReasonCode)
	}
}

func TestStartTelegramTransport_AutoWebhookSuccess(t *testing.T) {
	var mu sync.Mutex
	setCalls := 0
	infoCalls := 0

	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.URL.Path {
		case "/botTOKEN/setWebhook":
			setCalls++
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "result": true})
		case "/botTOKEN/getWebhookInfo":
			infoCalls++
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"result": map[string]interface{}{
					"url": "https://public.example.com/hook",
				},
			})
		case "/botTOKEN/deleteWebhook":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "result": true})
		default:
			http.NotFound(w, r)
		}
	}))

	cfg := &GatewayConfig{
		TelegramTransportMode: telegramTransportAuto,
		TelegramBotToken:      "TOKEN",
		TelegramWebhookURL:    "https://public.example.com/hook",
		TelegramAPIBaseURL:    srv.URL,
	}

	if err := startTelegramTransport(context.Background(), cfg, nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("startTelegramTransport returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if setCalls != 1 || infoCalls != 1 {
		t.Fatalf("unexpected telegram api calls: set=%d info=%d", setCalls, infoCalls)
	}
	status := snapshotTelegramTransportStatus()
	if status.SelectedMode != telegramTransportWebhook {
		t.Fatalf("selected mode = %q, want %q", status.SelectedMode, telegramTransportWebhook)
	}
}

func TestRunTelegramPollingLoop_NonCommandMessage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		mu           sync.Mutex
		getCalls     int
		sentChatID   string
		sentText     string
		sentMessages int
	)

	api := &scriptedTelegramAPI{
		getUpdatesFn: func(_ context.Context, _ int64, _ int) ([]map[string]interface{}, error) {
			mu.Lock()
			defer mu.Unlock()
			getCalls++
			return []map[string]interface{}{
				{
					"update_id": float64(1),
					"message": map[string]interface{}{
						"chat": map[string]interface{}{"id": float64(123)},
						"text": "hello from polling",
					},
				},
			}, nil
		},
		sendMessageFn: func(_ context.Context, chatID, text string, _ bool) error {
			mu.Lock()
			defer mu.Unlock()
			sentChatID = chatID
			sentText = text
			sentMessages++
			cancel()
			return nil
		},
	}

	sessions := NewSessionStore("", 0, nil)
	defer sessions.Stop()

	done := make(chan struct{})
	go func() {
		runTelegramPollingLoop(ctx, &GatewayConfig{TelegramPollingTimeout: 1}, api, nil, sessions, nil, nil, nil)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runTelegramPollingLoop did not stop after context cancel")
	}

	mu.Lock()
	defer mu.Unlock()
	if getCalls == 0 {
		t.Fatal("expected at least one getUpdates call")
	}
	if sentMessages != 1 {
		t.Fatalf("expected one outgoing message, got %d", sentMessages)
	}
	if sentChatID != "123" {
		t.Fatalf("sent chat id = %q, want 123", sentChatID)
	}
	if sentText == "" || !strings.Contains(sentText, "E_SESSION_REQUIRED") {
		t.Fatalf("unexpected outgoing text: %q", sentText)
	}
}

func TestRunTelegramPollingLoop_CommandMessage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := newMockDaemon(map[string]http.HandlerFunc{
		"GET /api/v1/agents": func(w http.ResponseWriter, _ *http.Request) {
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

	daemon := NewDaemonClient(srv.URL, "test-token", 5*time.Second)
	sessions := NewSessionStore("", 0, nil)
	defer sessions.Stop()
	sessions.CreateSession("telegram", "456")

	var (
		mu       sync.Mutex
		getCalls int
		sentText string
	)

	api := &scriptedTelegramAPI{
		getUpdatesFn: func(_ context.Context, _ int64, _ int) ([]map[string]interface{}, error) {
			mu.Lock()
			defer mu.Unlock()
			getCalls++
			return []map[string]interface{}{
				{
					"update_id": float64(10),
					"message": map[string]interface{}{
						"chat": map[string]interface{}{"id": float64(456)},
						"text": "/agents",
					},
				},
			}, nil
		},
		sendMessageFn: func(_ context.Context, _ string, text string, _ bool) error {
			mu.Lock()
			sentText = text
			mu.Unlock()
			cancel()
			return nil
		},
	}

	done := make(chan struct{})
	go func() {
		runTelegramPollingLoop(ctx, &GatewayConfig{TelegramPollingTimeout: 1}, api, daemon, sessions, nil, nil, nil)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("runTelegramPollingLoop command path did not stop after context cancel")
	}

	mu.Lock()
	defer mu.Unlock()
	if getCalls == 0 {
		t.Fatal("expected getUpdates to be called")
	}
	if !strings.Contains(sentText, "listed 1 agents") {
		t.Fatalf("unexpected command response text: %q", sentText)
	}
}

func TestRunTelegramPollingLoop_GetUpdatesErrorStopsOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sessions := NewSessionStore("", 0, nil)
	defer sessions.Stop()

	api := &scriptedTelegramAPI{
		getUpdatesFn: func(_ context.Context, _ int64, _ int) ([]map[string]interface{}, error) {
			cancel()
			return nil, errors.New("telegram temporary failure")
		},
	}

	done := make(chan struct{})
	go func() {
		runTelegramPollingLoop(ctx, &GatewayConfig{TelegramPollingTimeout: 1}, api, nil, sessions, nil, nil, nil)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runTelegramPollingLoop did not stop after getUpdates error + canceled context")
	}
}
