package gateway

import (
	"carrier/baseagent"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeTelegramAPI struct {
	setWebhookCalls   int
	getInfoCalls      int
	deleteCalls       int
	setCmdCalls       int
	sendMessageCalls  int
	sendPhotoCalls    int
	sendDocumentCalls int

	lastSendMessageText     string
	lastSendPhotoRef        string
	lastSendPhotoCaption    string
	lastSendDocumentRef     string
	lastSendDocumentCaption string

	setWebhookErr error
	getInfoErr    error
	deleteErr     error

	webhookInfo telegramWebhookInfo
	fileInfo    telegramFileInfo
	fileData    []byte
	getFileErr  error
	downloadErr error
}

func (f *fakeTelegramAPI) SetWebhook(_ context.Context, _ string, _ string) error {
	f.setWebhookCalls++
	return f.setWebhookErr
}

func (f *fakeTelegramAPI) GetWebhookInfo(_ context.Context) (telegramWebhookInfo, error) {
	f.getInfoCalls++
	if f.getInfoErr != nil {
		return telegramWebhookInfo{}, f.getInfoErr
	}
	return f.webhookInfo, nil
}

func (f *fakeTelegramAPI) DeleteWebhook(_ context.Context) error {
	f.deleteCalls++
	return f.deleteErr
}

func (f *fakeTelegramAPI) GetUpdates(_ context.Context, _ int64, _ int) ([]map[string]interface{}, error) {
	return nil, nil
}

func (f *fakeTelegramAPI) SendMessage(_ context.Context, _ string, _ string, _ bool) error {
	f.sendMessageCalls++
	return nil
}

func (f *fakeTelegramAPI) SendPhoto(_ context.Context, _ string, photo string, caption string) error {
	f.sendPhotoCalls++
	f.lastSendPhotoRef = photo
	f.lastSendPhotoCaption = caption
	return nil
}

func (f *fakeTelegramAPI) SendDocument(_ context.Context, _ string, document string, caption string) error {
	f.sendDocumentCalls++
	f.lastSendDocumentRef = document
	f.lastSendDocumentCaption = caption
	return nil
}

func (f *fakeTelegramAPI) SetMyCommands(_ context.Context, _ []telegramBotCommand) error {
	f.setCmdCalls++
	return nil
}

func (f *fakeTelegramAPI) GetFile(_ context.Context, _ string) (telegramFileInfo, error) {
	if f.getFileErr != nil {
		return telegramFileInfo{}, f.getFileErr
	}
	return f.fileInfo, nil
}

func (f *fakeTelegramAPI) DownloadFile(_ context.Context, _ string) ([]byte, error) {
	if f.downloadErr != nil {
		return nil, f.downloadErr
	}
	return append([]byte(nil), f.fileData...), nil
}

func TestNormalizeTelegramWebhookURL(t *testing.T) {
	t.Run("normalizes path", func(t *testing.T) {
		got, err := normalizeTelegramWebhookURL("https://example.com")
		if err != nil {
			t.Fatalf("normalizeTelegramWebhookURL error: %v", err)
		}
		if got != "https://example.com/webhook/telegram" {
			t.Fatalf("normalized URL = %q, want %q", got, "https://example.com/webhook/telegram")
		}
	})

	t.Run("rejects non-https", func(t *testing.T) {
		if _, err := normalizeTelegramWebhookURL("http://example.com/webhook/telegram"); err == nil {
			t.Fatal("expected error for non-https URL")
		}
	})

	t.Run("rejects localhost", func(t *testing.T) {
		if _, err := normalizeTelegramWebhookURL("https://127.0.0.1/webhook/telegram"); err == nil {
			t.Fatal("expected error for localhost URL")
		}
	})
}

func TestResolveTelegramTransportMode_AutoWebhookSuccess(t *testing.T) {
	fake := &fakeTelegramAPI{
		webhookInfo: telegramWebhookInfo{
			URL: "https://public.example.com/webhook/telegram",
		},
	}
	cfg := &GatewayConfig{
		TelegramTransportMode: telegramTransportAuto,
		TelegramWebhookURL:    "https://public.example.com/webhook/telegram",
	}

	decision, err := resolveTelegramTransportMode(context.Background(), cfg, fake)
	if err != nil {
		t.Fatalf("resolveTelegramTransportMode error: %v", err)
	}
	if decision.Mode != telegramTransportWebhook {
		t.Fatalf("mode = %q, want %q", decision.Mode, telegramTransportWebhook)
	}
	if decision.ReasonCode != "" {
		t.Fatalf("reasonCode = %q, want empty", decision.ReasonCode)
	}
	if fake.setWebhookCalls != 1 {
		t.Fatalf("setWebhookCalls = %d, want 1", fake.setWebhookCalls)
	}
	if fake.getInfoCalls != 1 {
		t.Fatalf("getInfoCalls = %d, want 1", fake.getInfoCalls)
	}
	if fake.deleteCalls != 0 {
		t.Fatalf("deleteCalls = %d, want 0", fake.deleteCalls)
	}
}

func TestResolveTelegramTransportMode_AutoFallbackPolling(t *testing.T) {
	fake := &fakeTelegramAPI{}
	cfg := &GatewayConfig{
		TelegramTransportMode: telegramTransportAuto,
		TelegramWebhookURL:    "https://127.0.0.1/webhook/telegram",
	}

	decision, err := resolveTelegramTransportMode(context.Background(), cfg, fake)
	if err != nil {
		t.Fatalf("resolveTelegramTransportMode error: %v", err)
	}
	if decision.Mode != telegramTransportPolling {
		t.Fatalf("mode = %q, want %q", decision.Mode, telegramTransportPolling)
	}
	if decision.ReasonCode != telegramFallbackWebhookURLInvalid {
		t.Fatalf("reasonCode = %q, want %q", decision.ReasonCode, telegramFallbackWebhookURLInvalid)
	}
	if !strings.Contains(decision.Reason, "public") {
		t.Fatalf("reason = %q, want contains %q", decision.Reason, "public")
	}
	if fake.setWebhookCalls != 0 {
		t.Fatalf("setWebhookCalls = %d, want 0", fake.setWebhookCalls)
	}
	if fake.deleteCalls != 1 {
		t.Fatalf("deleteCalls = %d, want 1", fake.deleteCalls)
	}
}

func TestResolveTelegramTransportMode_AutoFallbackWebhookSetupFailure(t *testing.T) {
	fake := &fakeTelegramAPI{setWebhookErr: context.DeadlineExceeded}
	cfg := &GatewayConfig{
		TelegramTransportMode: telegramTransportAuto,
		TelegramWebhookURL:    "https://public.example.com/webhook/telegram",
	}

	decision, err := resolveTelegramTransportMode(context.Background(), cfg, fake)
	if err != nil {
		t.Fatalf("resolveTelegramTransportMode error: %v", err)
	}
	if decision.Mode != telegramTransportPolling {
		t.Fatalf("mode = %q, want %q", decision.Mode, telegramTransportPolling)
	}
	if decision.ReasonCode != telegramFallbackWebhookSetupFailed {
		t.Fatalf("reasonCode = %q, want %q", decision.ReasonCode, telegramFallbackWebhookSetupFailed)
	}
	if !strings.Contains(decision.Reason, "setWebhook failed") {
		t.Fatalf("reason = %q, want setup failure detail", decision.Reason)
	}
}

func TestResolveTelegramTransportMode_AutoFallbackMissingWebhookURL(t *testing.T) {
	fake := &fakeTelegramAPI{}
	cfg := &GatewayConfig{TelegramTransportMode: telegramTransportAuto}

	decision, err := resolveTelegramTransportMode(context.Background(), cfg, fake)
	if err != nil {
		t.Fatalf("resolveTelegramTransportMode error: %v", err)
	}
	if decision.Mode != telegramTransportPolling {
		t.Fatalf("mode = %q, want %q", decision.Mode, telegramTransportPolling)
	}
	if decision.ReasonCode != telegramFallbackWebhookURLInvalid {
		t.Fatalf("reasonCode = %q, want %q", decision.ReasonCode, telegramFallbackWebhookURLInvalid)
	}
	if !strings.Contains(decision.Reason, "missing CARRIER_TELEGRAM_WEBHOOK_URL") {
		t.Fatalf("reason = %q, want missing webhook URL detail", decision.Reason)
	}
}

func TestResolveTelegramTransportMode_WebhookInvalidURL(t *testing.T) {
	fake := &fakeTelegramAPI{}
	cfg := &GatewayConfig{
		TelegramTransportMode: telegramTransportWebhook,
		TelegramWebhookURL:    "http://example.com/webhook/telegram",
	}

	_, err := resolveTelegramTransportMode(context.Background(), cfg, fake)
	if err == nil {
		t.Fatal("expected error for invalid webhook URL in strict webhook mode")
	}
}

func TestStartTelegramTransport_RequiresTokenForExplicitMode(t *testing.T) {
	cfg := &GatewayConfig{
		TelegramTransportMode: telegramTransportWebhook,
	}
	err := startTelegramTransport(context.Background(), cfg, nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error when token is missing in explicit webhook mode")
	}
}

func TestResolveTelegramTransportMode_AutoFallbackVerifyFailure(t *testing.T) {
	fake := &fakeTelegramAPI{
		webhookInfo: telegramWebhookInfo{URL: "https://public.example.com/another-path"},
	}
	cfg := &GatewayConfig{
		TelegramTransportMode: telegramTransportAuto,
		TelegramWebhookURL:    "https://public.example.com/webhook/telegram",
	}

	decision, err := resolveTelegramTransportMode(context.Background(), cfg, fake)
	if err != nil {
		t.Fatalf("resolveTelegramTransportMode error: %v", err)
	}
	if decision.Mode != telegramTransportPolling {
		t.Fatalf("mode = %q, want %q", decision.Mode, telegramTransportPolling)
	}
	if decision.ReasonCode != telegramFallbackWebhookVerifyFailed {
		t.Fatalf("reasonCode = %q, want %q", decision.ReasonCode, telegramFallbackWebhookVerifyFailed)
	}
	if !strings.Contains(decision.Reason, "expected") {
		t.Fatalf("reason = %q, want verification mismatch detail", decision.Reason)
	}
}

func TestResolveTelegramTransportMode_AutoFallbackVerifyCleanupFailure(t *testing.T) {
	fake := &fakeTelegramAPI{
		webhookInfo: telegramWebhookInfo{URL: "https://public.example.com/another-path"},
		deleteErr:   context.DeadlineExceeded,
	}
	cfg := &GatewayConfig{
		TelegramTransportMode: telegramTransportAuto,
		TelegramWebhookURL:    "https://public.example.com/webhook/telegram",
	}

	decision, err := resolveTelegramTransportMode(context.Background(), cfg, fake)
	if err != nil {
		t.Fatalf("resolveTelegramTransportMode error: %v", err)
	}
	if decision.ReasonCode != telegramFallbackWebhookCleanupFailed {
		t.Fatalf("reasonCode = %q, want %q", decision.ReasonCode, telegramFallbackWebhookCleanupFailed)
	}
	if !strings.Contains(decision.Reason, "deleteWebhook failed") {
		t.Fatalf("reason = %q, want cleanup failure detail", decision.Reason)
	}
}

func TestResolveTelegramTransportMode_AutoFallbackSetupCleanupFailure(t *testing.T) {
	fake := &fakeTelegramAPI{
		setWebhookErr: context.DeadlineExceeded,
		deleteErr:     context.DeadlineExceeded,
	}
	cfg := &GatewayConfig{
		TelegramTransportMode: telegramTransportAuto,
		TelegramWebhookURL:    "https://public.example.com/webhook/telegram",
	}

	decision, err := resolveTelegramTransportMode(context.Background(), cfg, fake)
	if err != nil {
		t.Fatalf("resolveTelegramTransportMode error: %v", err)
	}
	if decision.ReasonCode != telegramFallbackWebhookCleanupFailed {
		t.Fatalf("reasonCode = %q, want %q", decision.ReasonCode, telegramFallbackWebhookCleanupFailed)
	}
	if !strings.Contains(decision.Reason, "setWebhook failed") || !strings.Contains(decision.Reason, "deleteWebhook failed") {
		t.Fatalf("reason = %q, want setup+cleanup failure detail", decision.Reason)
	}
}

func TestResolveTelegramTransportMode_PollingForced(t *testing.T) {
	fake := &fakeTelegramAPI{}
	cfg := &GatewayConfig{
		TelegramTransportMode: telegramTransportPolling,
	}

	decision, err := resolveTelegramTransportMode(context.Background(), cfg, fake)
	if err != nil {
		t.Fatalf("resolveTelegramTransportMode error: %v", err)
	}
	if decision.Mode != telegramTransportPolling {
		t.Fatalf("mode = %q, want %q", decision.Mode, telegramTransportPolling)
	}
	if decision.ReasonCode != "POLLING_FORCED" {
		t.Fatalf("reasonCode = %q, want POLLING_FORCED", decision.ReasonCode)
	}
	if fake.deleteCalls != 1 {
		t.Fatalf("deleteCalls = %d, want 1", fake.deleteCalls)
	}
}

func TestResolveTelegramTransportMode_WebhookSetupAndVerifyErrors(t *testing.T) {
	t.Run("set webhook fails", func(t *testing.T) {
		fake := &fakeTelegramAPI{setWebhookErr: context.DeadlineExceeded}
		cfg := &GatewayConfig{
			TelegramTransportMode: telegramTransportWebhook,
			TelegramWebhookURL:    "https://public.example.com/webhook/telegram",
		}
		if _, err := resolveTelegramTransportMode(context.Background(), cfg, fake); err == nil || !strings.Contains(err.Error(), "webhook setup failed") {
			t.Fatalf("expected setup error, got %v", err)
		}
	})

	t.Run("get webhook info fails", func(t *testing.T) {
		fake := &fakeTelegramAPI{getInfoErr: context.DeadlineExceeded}
		cfg := &GatewayConfig{
			TelegramTransportMode: telegramTransportWebhook,
			TelegramWebhookURL:    "https://public.example.com/webhook/telegram",
		}
		if _, err := resolveTelegramTransportMode(context.Background(), cfg, fake); err == nil || !strings.Contains(err.Error(), "verification failed") {
			t.Fatalf("expected verification error, got %v", err)
		}
	})

	t.Run("get webhook info mismatch", func(t *testing.T) {
		fake := &fakeTelegramAPI{webhookInfo: telegramWebhookInfo{URL: "https://public.example.com/wrong"}}
		cfg := &GatewayConfig{
			TelegramTransportMode: telegramTransportWebhook,
			TelegramWebhookURL:    "https://public.example.com/webhook/telegram",
		}
		if _, err := resolveTelegramTransportMode(context.Background(), cfg, fake); err == nil || !strings.Contains(err.Error(), "expected") {
			t.Fatalf("expected mismatch error, got %v", err)
		}
	})
}

func TestTelegramSendRenderedAttachment_PrefersDocument(t *testing.T) {
	api := &fakeTelegramAPI{}
	resp := GatewayResponse{
		Result:  "ok",
		Message: "download complete",
		RichContent: &baseagent.RichOutboundMessage{
			Text: "download complete",
			Attachments: []baseagent.AttachmentRef{
				{Kind: "document", ExternalID: "tg-file-1", Name: "report.pdf"},
			},
		},
	}

	if err := sendTelegramGatewayResponse(context.Background(), api, "123", resp); err != nil {
		t.Fatalf("sendTelegramGatewayResponse error: %v", err)
	}
	if api.sendDocumentCalls != 1 || api.lastSendDocumentRef != "tg-file-1" {
		t.Fatalf("expected sendDocument with tg-file-1, got calls=%d ref=%q", api.sendDocumentCalls, api.lastSendDocumentRef)
	}
	if api.sendMessageCalls != 0 {
		t.Fatalf("expected no text fallback send, got %d", api.sendMessageCalls)
	}
}

func TestTelegramSendRenderedAttachment_FallsBackToTextForUnsupportedAttachment(t *testing.T) {
	api := &fakeTelegramAPI{}
	resp := GatewayResponse{
		Result: "ok",
		RichContent: &baseagent.RichOutboundMessage{
			Blocks: []baseagent.ContentBlock{
				{Type: "audio", Text: "voice note"},
			},
			Attachments: []baseagent.AttachmentRef{
				{Kind: "audio", Name: "voice.ogg"},
			},
		},
	}

	if err := sendTelegramGatewayResponse(context.Background(), api, "123", resp); err != nil {
		t.Fatalf("sendTelegramGatewayResponse error: %v", err)
	}
	if api.sendMessageCalls != 1 {
		t.Fatalf("expected text fallback send, got %d", api.sendMessageCalls)
	}
	if api.sendPhotoCalls != 0 || api.sendDocumentCalls != 0 {
		t.Fatalf("expected unsupported attachment to avoid rich media send, got photo=%d document=%d", api.sendPhotoCalls, api.sendDocumentCalls)
	}
}

func TestTelegramSendRenderedAttachment_PrefersImageBlockURL(t *testing.T) {
	api := &fakeTelegramAPI{}
	resp := GatewayResponse{
		Result: "ok",
		RichContent: &baseagent.RichOutboundMessage{
			Text: "generated image",
			Blocks: []baseagent.ContentBlock{
				{Type: "image", URL: "https://files.example.com/generated.png", Name: "generated.png"},
			},
		},
	}

	if err := sendTelegramGatewayResponse(context.Background(), api, "123", resp); err != nil {
		t.Fatalf("sendTelegramGatewayResponse error: %v", err)
	}
	if api.sendPhotoCalls != 1 || api.lastSendPhotoRef != "https://files.example.com/generated.png" {
		t.Fatalf("expected sendPhoto with block url, got calls=%d ref=%q", api.sendPhotoCalls, api.lastSendPhotoRef)
	}
	if api.sendMessageCalls != 0 {
		t.Fatalf("expected no text fallback send, got %d", api.sendMessageCalls)
	}
}

func TestTelegramSendRenderedAttachment_UsesAttachmentDownloadURLForDocument(t *testing.T) {
	api := &fakeTelegramAPI{}
	resp := GatewayResponse{
		Result: "ok",
		RichContent: &baseagent.RichOutboundMessage{
			Text: "report ready",
			Attachments: []baseagent.AttachmentRef{
				{Kind: "document", Name: "report.pdf", DownloadURL: "https://downloads.example.com/report.pdf"},
			},
		},
	}

	if err := sendTelegramGatewayResponse(context.Background(), api, "123", resp); err != nil {
		t.Fatalf("sendTelegramGatewayResponse error: %v", err)
	}
	if api.sendDocumentCalls != 1 || api.lastSendDocumentRef != "https://downloads.example.com/report.pdf" {
		t.Fatalf("expected sendDocument with download url, got calls=%d ref=%q", api.sendDocumentCalls, api.lastSendDocumentRef)
	}
	if api.sendMessageCalls != 0 {
		t.Fatalf("expected no text fallback send, got %d", api.sendMessageCalls)
	}
}

func TestHydrateTelegramInboundAttachments_DownloadsToArtifactRoot(t *testing.T) {
	artifactRoot, err := os.MkdirTemp(".", "telegram-inbound-artifacts-*")
	if err != nil {
		t.Fatalf("mkdir artifact root: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(artifactRoot) })
	downloads := NewDownloadStore(artifactRoot, nil)
	api := &fakeTelegramAPI{
		fileInfo: telegramFileInfo{
			FileID:       "tg-file-1",
			FileUniqueID: "tg-uniq-1",
			FilePath:     "documents/file_1/report.pdf",
		},
		fileData: []byte("%PDF-1.7"),
	}
	envelope := &InboundChannelEnvelope{
		RequestID: "req-telegram-1",
		Attachments: []baseagent.AttachmentRef{{
			ID:         "tg-uniq-1",
			Kind:       "document",
			Name:       "report.pdf",
			MediaType:  "application/pdf",
			ExternalID: "tg-file-1",
			Source:     "telegram",
			SourceMetadata: map[string]string{
				"telegram_file_id": "tg-file-1",
				"chat_id":          "123",
				"message_id":       "456",
			},
		}},
	}
	cfg := &GatewayConfig{
		ArtifactRoot:       artifactRoot,
		TelegramBotToken:   "TOKEN",
		TelegramAPIBaseURL: "https://api.telegram.org",
	}

	if err := hydrateTelegramInboundAttachments(context.Background(), envelope, cfg, downloads, api); err != nil {
		t.Fatalf("hydrateTelegramInboundAttachments error: %v", err)
	}
	if len(envelope.Attachments) != 1 {
		t.Fatalf("attachments len=%d want 1", len(envelope.Attachments))
	}
	attachment := envelope.Attachments[0]
	if attachment.Path == "" {
		t.Fatalf("expected persisted attachment path, got %+v", attachment)
	}
	if got := filepath.Base(attachment.Path); got != "report.pdf" {
		t.Fatalf("attachment path base=%q want report.pdf path=%q", got, attachment.Path)
	}
	if attachment.ArtifactID == "" {
		t.Fatalf("expected artifact id, got %+v", attachment)
	}
	if attachment.DownloadURL == "" || !strings.HasPrefix(attachment.DownloadURL, "/downloads/") {
		t.Fatalf("expected download URL, got %+v", attachment)
	}
	data, err := os.ReadFile(attachment.Path)
	if err != nil {
		t.Fatalf("read persisted attachment: %v", err)
	}
	if string(data) != "%PDF-1.7" {
		t.Fatalf("unexpected persisted data %q", string(data))
	}
}

func TestTelegramBotAPI_MethodWrappers(t *testing.T) {
	type callRecord struct {
		Method  string
		Payload map[string]interface{}
	}

	var calls []callRecord
	srv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiMethod := strings.TrimPrefix(r.URL.Path, "/botTOKEN/")
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		calls = append(calls, callRecord{Method: apiMethod, Payload: payload})

		w.Header().Set("Content-Type", "application/json")
		switch apiMethod {
		case "getWebhookInfo":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"url":"https://public.example.com/webhook/telegram"}}`))
		case "getUpdates":
			_, _ = w.Write([]byte(`{"ok":true,"result":[{"update_id":123,"message":{"text":"/agents","chat":{"id":42}}}]}`))
		default:
			_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
		}
	}))
	defer srv.Close()

	api := newTelegramBotAPI("TOKEN", srv.URL, srv.Client())

	if err := api.SetWebhook(context.Background(), "https://public.example.com/webhook/telegram", "sec123"); err != nil {
		t.Fatalf("SetWebhook: %v", err)
	}
	if err := api.SetMyCommands(context.Background(), []telegramBotCommand{
		{Command: "pair", Description: "Pair chat"},
		{Command: "agents", Description: "List agents"},
	}); err != nil {
		t.Fatalf("SetMyCommands: %v", err)
	}
	info, err := api.GetWebhookInfo(context.Background())
	if err != nil {
		t.Fatalf("GetWebhookInfo: %v", err)
	}
	if info.URL != "https://public.example.com/webhook/telegram" {
		t.Fatalf("unexpected webhook url: %q", info.URL)
	}
	if err := api.DeleteWebhook(context.Background()); err != nil {
		t.Fatalf("DeleteWebhook: %v", err)
	}
	if err := api.SendMessage(context.Background(), "42", "hello", true); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	updates, err := api.GetUpdates(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("GetUpdates: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("updates length = %d, want 1", len(updates))
	}

	if len(calls) != 6 {
		t.Fatalf("expected 6 calls, got %d", len(calls))
	}
	if calls[0].Method != "setWebhook" || calls[0].Payload["secret_token"] != "sec123" {
		t.Fatalf("unexpected setWebhook payload: %+v", calls[0])
	}
	if calls[1].Method != "setMyCommands" {
		t.Fatalf("unexpected setMyCommands call: %+v", calls[1])
	}
	commands, ok := calls[1].Payload["commands"].([]interface{})
	if !ok || len(commands) != 2 {
		t.Fatalf("unexpected setMyCommands payload: %+v", calls[1].Payload)
	}
	if calls[3].Method != "deleteWebhook" || calls[3].Payload["drop_pending_updates"] != false {
		t.Fatalf("unexpected deleteWebhook payload: %+v", calls[3])
	}
	if calls[4].Method != "sendMessage" || calls[4].Payload["disable_web_page_preview"] != true {
		t.Fatalf("unexpected sendMessage payload: %+v", calls[4])
	}
	if calls[5].Method != "getUpdates" || calls[5].Payload["timeout"] != float64(30) {
		t.Fatalf("unexpected getUpdates payload: %+v", calls[5])
	}
}

func TestTelegramBotAPI_CallErrorBranches(t *testing.T) {
	api := newTelegramBotAPI("", "https://api.telegram.org", nil)
	if err := api.SendMessage(context.Background(), "42", "hello", false); err == nil {
		t.Fatal("expected error when token is empty")
	}

	httpErrSrv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("bad gateway"))
	}))
	defer httpErrSrv.Close()

	api = newTelegramBotAPI("TOKEN", httpErrSrv.URL, httpErrSrv.Client())
	if err := api.SetWebhook(context.Background(), "https://public.example.com/webhook/telegram", ""); err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("expected HTTP error, got %v", err)
	}

	decodeErrSrv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"result":{invalid`))
	}))
	defer decodeErrSrv.Close()

	api = newTelegramBotAPI("TOKEN", decodeErrSrv.URL, decodeErrSrv.Client())
	var out struct{ X string }
	if err := api.call(context.Background(), "customMethod", map[string]string{"x": "y"}, &out); err == nil || !strings.Contains(err.Error(), "decode customMethod response") {
		t.Fatalf("expected decode response error, got %v", err)
	}

	notOkSrv := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":false,"description":"bad request"}`))
	}))
	defer notOkSrv.Close()

	api = newTelegramBotAPI("TOKEN", notOkSrv.URL, notOkSrv.Client())
	if err := api.DeleteWebhook(context.Background()); err == nil || !strings.Contains(err.Error(), "failed: bad request") {
		t.Fatalf("expected api-level failure, got %v", err)
	}
}
