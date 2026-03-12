package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	telegramTransportAuto    = "auto"
	telegramTransportWebhook = "webhook"
	telegramTransportPolling = "polling"

	telegramFallbackTokenMissing         = "TOKEN_MISSING"
	telegramFallbackWebhookURLInvalid    = "WEBHOOK_URL_INVALID"
	telegramFallbackWebhookSetupFailed   = "WEBHOOK_SETUP_FAILED"
	telegramFallbackWebhookVerifyFailed  = "WEBHOOK_VERIFY_FAILED"
	telegramFallbackWebhookCleanupFailed = "WEBHOOK_CLEANUP_FAILED"
)

type telegramTransportDecision struct {
	Mode       string
	ReasonCode string
	Reason     string
	Hint       string
}

type telegramAPI interface {
	SetWebhook(ctx context.Context, webhookURL, webhookSecret string) error
	GetWebhookInfo(ctx context.Context) (telegramWebhookInfo, error)
	DeleteWebhook(ctx context.Context) error
	GetUpdates(ctx context.Context, offset int64, timeoutSec int) ([]map[string]interface{}, error)
	SendMessage(ctx context.Context, chatID, text string, disableWebPagePreview bool) error
	SendPhoto(ctx context.Context, chatID, photo, caption string) error
	SendDocument(ctx context.Context, chatID, document, caption string) error
	SetMyCommands(ctx context.Context, commands []telegramBotCommand) error
}

type telegramWebhookInfo struct {
	URL string `json:"url"`
}

type telegramBotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

type telegramBotAPI struct {
	baseURL string
	token   string
	client  *http.Client
}

func newTelegramBotAPI(token, baseURL string, client *http.Client) *telegramBotAPI {
	normalizedBase := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if normalizedBase == "" {
		normalizedBase = "https://api.telegram.org"
	}
	if client == nil {
		client = &http.Client{Timeout: 45 * time.Second}
	}
	return &telegramBotAPI{
		baseURL: normalizedBase,
		token:   strings.TrimSpace(token),
		client:  client,
	}
}

func (a *telegramBotAPI) SetWebhook(ctx context.Context, webhookURL, webhookSecret string) error {
	payload := map[string]interface{}{
		"url": webhookURL,
	}
	if strings.TrimSpace(webhookSecret) != "" {
		payload["secret_token"] = strings.TrimSpace(webhookSecret)
	}
	return a.call(ctx, "setWebhook", payload, nil)
}

func (a *telegramBotAPI) GetWebhookInfo(ctx context.Context) (telegramWebhookInfo, error) {
	var info telegramWebhookInfo
	if err := a.call(ctx, "getWebhookInfo", map[string]interface{}{}, &info); err != nil {
		return telegramWebhookInfo{}, err
	}
	return info, nil
}

func (a *telegramBotAPI) DeleteWebhook(ctx context.Context) error {
	payload := map[string]interface{}{
		"drop_pending_updates": false,
	}
	return a.call(ctx, "deleteWebhook", payload, nil)
}

func (a *telegramBotAPI) GetUpdates(ctx context.Context, offset int64, timeoutSec int) ([]map[string]interface{}, error) {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	payload := map[string]interface{}{
		"offset":          offset,
		"timeout":         timeoutSec,
		"allowed_updates": []string{"message", "edited_message", "channel_post", "edited_channel_post"},
	}
	var updates []map[string]interface{}
	if err := a.call(ctx, "getUpdates", payload, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

func (a *telegramBotAPI) SendMessage(ctx context.Context, chatID, text string, disableWebPagePreview bool) error {
	payload := map[string]interface{}{
		"chat_id": chatID,
		"text":    text,
	}
	if disableWebPagePreview {
		payload["disable_web_page_preview"] = true
	}
	return a.call(ctx, "sendMessage", payload, nil)
}

func (a *telegramBotAPI) SendPhoto(ctx context.Context, chatID, photo, caption string) error {
	payload := map[string]interface{}{
		"chat_id": chatID,
		"photo":   photo,
	}
	if strings.TrimSpace(caption) != "" {
		payload["caption"] = strings.TrimSpace(caption)
	}
	return a.call(ctx, "sendPhoto", payload, nil)
}

func (a *telegramBotAPI) SendDocument(ctx context.Context, chatID, document, caption string) error {
	payload := map[string]interface{}{
		"chat_id":  chatID,
		"document": document,
	}
	if strings.TrimSpace(caption) != "" {
		payload["caption"] = strings.TrimSpace(caption)
	}
	return a.call(ctx, "sendDocument", payload, nil)
}

func (a *telegramBotAPI) SetMyCommands(ctx context.Context, commands []telegramBotCommand) error {
	safe := make([]telegramBotCommand, 0, len(commands))
	for _, cmd := range commands {
		command := strings.TrimSpace(cmd.Command)
		description := strings.TrimSpace(cmd.Description)
		if command == "" || description == "" {
			continue
		}
		safe = append(safe, telegramBotCommand{
			Command:     command,
			Description: description,
		})
	}
	if len(safe) == 0 {
		return nil
	}
	payload := map[string]interface{}{
		"commands": safe,
	}
	return a.call(ctx, "setMyCommands", payload, nil)
}

type telegramAPIEnvelope struct {
	OK          bool            `json:"ok"`
	Description string          `json:"description"`
	Result      json.RawMessage `json:"result"`
}

func (a *telegramBotAPI) call(ctx context.Context, method string, payload interface{}, out interface{}) error {
	if strings.TrimSpace(a.token) == "" {
		return errors.New("telegram bot token is empty")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal %s payload: %w", method, err)
	}

	urlStr := fmt.Sprintf("%s/bot%s/%s", a.baseURL, a.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build %s request: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram %s request failed: %w", method, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read %s response: %w", method, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram %s HTTP %d: %s", method, resp.StatusCode, strings.TrimSpace(string(respBytes)))
	}

	var envelope telegramAPIEnvelope
	if err := json.Unmarshal(respBytes, &envelope); err != nil {
		return fmt.Errorf("decode %s response: %w", method, err)
	}
	if !envelope.OK {
		if envelope.Description == "" {
			envelope.Description = "unknown telegram API error"
		}
		return fmt.Errorf("telegram %s failed: %s", method, envelope.Description)
	}
	if out != nil && len(envelope.Result) > 0 && string(envelope.Result) != "null" {
		if err := json.Unmarshal(envelope.Result, out); err != nil {
			return fmt.Errorf("decode %s result: %w", method, err)
		}
	}
	return nil
}

func startTelegramTransport(
	ctx context.Context,
	cfg *GatewayConfig,
	daemon *DaemonClient,
	sessions *SessionStore,
	downloads *DownloadStore,
	rl *GatewayRateLimiter,
	onboard *OnboardStore,
) error {
	mode := strings.ToLower(strings.TrimSpace(cfg.TelegramTransportMode))
	if mode == "" {
		mode = telegramTransportAuto
	}

	token := strings.TrimSpace(cfg.TelegramBotToken)
	if token == "" {
		if mode == telegramTransportAuto {
			setTelegramTransportStatus(mode, "disabled", telegramFallbackTokenMissing, "telegram bot token is missing", "Set CARRIER_TELEGRAM_BOT_TOKEN to enable telegram transport.")
			log.Printf("[gateway/telegram] transport disabled: missing CARRIER_TELEGRAM_BOT_TOKEN")
			return nil
		}
		setTelegramTransportStatus(mode, "error", telegramFallbackTokenMissing, "telegram bot token is missing", "Set CARRIER_TELEGRAM_BOT_TOKEN for explicit transport mode.")
		return fmt.Errorf("telegram transport mode %q requires CARRIER_TELEGRAM_BOT_TOKEN", mode)
	}

	api := newTelegramBotAPI(token, cfg.TelegramAPIBaseURL, nil)
	if err := api.SetMyCommands(ctx, carrierTelegramDefaultCommands()); err != nil {
		// Command menu is UX-only; transport should continue even if this fails.
		log.Printf("[gateway/telegram] warning: setMyCommands failed: %v", err)
	}
	decision, err := resolveTelegramTransportMode(ctx, cfg, api)
	if err != nil {
		setTelegramTransportStatus(mode, "error", "RESOLUTION_FAILED", err.Error(), "Verify telegram transport configuration.")
		return err
	}
	setTelegramTransportStatus(mode, decision.Mode, decision.ReasonCode, decision.Reason, decision.Hint)

	switch decision.Mode {
	case telegramTransportWebhook:
		log.Printf("[gateway/telegram] transport=webhook url=%s", strings.TrimSpace(cfg.TelegramWebhookURL))
		return nil
	case telegramTransportPolling:
		if strings.TrimSpace(decision.ReasonCode) != "" || strings.TrimSpace(decision.Reason) != "" {
			log.Printf("[gateway/telegram] transport=polling reason_code=%s reason=%s hint=%s", decision.ReasonCode, decision.Reason, decision.Hint)
		} else {
			log.Printf("[gateway/telegram] transport=polling")
		}
		go runTelegramPollingLoop(ctx, cfg, api, daemon, sessions, downloads, rl, onboard)
		return nil
	default:
		return fmt.Errorf("unknown telegram transport mode resolved: %s", decision.Mode)
	}
}

func resolveTelegramTransportMode(ctx context.Context, cfg *GatewayConfig, api telegramAPI) (telegramTransportDecision, error) {
	requestedMode := strings.ToLower(strings.TrimSpace(cfg.TelegramTransportMode))
	if requestedMode == "" {
		requestedMode = telegramTransportAuto
	}

	switch requestedMode {
	case telegramTransportAuto:
		webhookURL, webhookErr := normalizeTelegramWebhookURL(cfg.TelegramWebhookURL)
		if webhookErr == nil {
			if setErr := api.SetWebhook(ctx, webhookURL, cfg.TelegramWebhookSecret); setErr == nil {
				info, infoErr := api.GetWebhookInfo(ctx)
				if infoErr == nil && strings.TrimSpace(info.URL) == webhookURL {
					return telegramTransportDecision{Mode: telegramTransportWebhook}, nil
				}
				decision := telegramTransportDecision{
					Mode:       telegramTransportPolling,
					ReasonCode: telegramFallbackWebhookVerifyFailed,
					Hint:       "Confirm CARRIER_TELEGRAM_WEBHOOK_URL is publicly reachable and matches Telegram webhook info.",
				}
				if infoErr != nil {
					decision.Reason = fmt.Sprintf("webhook verification failed: %v", infoErr)
				} else {
					decision.Reason = fmt.Sprintf("webhook verification failed: expected %q, got %q", webhookURL, strings.TrimSpace(info.URL))
				}
				if delErr := api.DeleteWebhook(ctx); delErr != nil {
					decision.ReasonCode = telegramFallbackWebhookCleanupFailed
					decision.Reason = decision.Reason + "; deleteWebhook failed: " + delErr.Error()
					decision.Hint = "Fix webhook URL/secret mismatch and ensure Telegram API calls succeed."
				}
				return decision, nil
			} else {
				decision := telegramTransportDecision{
					Mode:       telegramTransportPolling,
					ReasonCode: telegramFallbackWebhookSetupFailed,
					Reason:     fmt.Sprintf("setWebhook failed: %v", setErr),
					Hint:       "Ensure webhook URL is reachable over HTTPS and webhook secret matches gateway config.",
				}
				if delErr := api.DeleteWebhook(ctx); delErr != nil {
					decision.ReasonCode = telegramFallbackWebhookCleanupFailed
					decision.Reason = decision.Reason + "; deleteWebhook failed: " + delErr.Error()
					decision.Hint = "Resolve Telegram webhook API failures and retry auto mode."
				}
				return decision, nil
			}
		} else {
			decision := telegramTransportDecision{
				Mode:       telegramTransportPolling,
				ReasonCode: telegramFallbackWebhookURLInvalid,
				Reason:     webhookErr.Error(),
				Hint:       "Set a public HTTPS CARRIER_TELEGRAM_WEBHOOK_URL, or force polling mode explicitly.",
			}
			if delErr := api.DeleteWebhook(ctx); delErr != nil {
				decision.ReasonCode = telegramFallbackWebhookCleanupFailed
				decision.Reason = decision.Reason + "; deleteWebhook failed: " + delErr.Error()
				decision.Hint = "Fix webhook URL and Telegram API connectivity before retrying auto mode."
			}
			return decision, nil
		}

	case telegramTransportWebhook:
		webhookURL, webhookErr := normalizeTelegramWebhookURL(cfg.TelegramWebhookURL)
		if webhookErr != nil {
			return telegramTransportDecision{}, fmt.Errorf("telegram webhook mode requires a valid public HTTPS CARRIER_TELEGRAM_WEBHOOK_URL: %w", webhookErr)
		}
		if err := api.SetWebhook(ctx, webhookURL, cfg.TelegramWebhookSecret); err != nil {
			return telegramTransportDecision{}, fmt.Errorf("telegram webhook setup failed: %w", err)
		}
		info, err := api.GetWebhookInfo(ctx)
		if err != nil {
			return telegramTransportDecision{}, fmt.Errorf("telegram webhook verification failed: %w", err)
		}
		if strings.TrimSpace(info.URL) != webhookURL {
			return telegramTransportDecision{}, fmt.Errorf("telegram webhook verification failed: expected %q, got %q", webhookURL, strings.TrimSpace(info.URL))
		}
		return telegramTransportDecision{Mode: telegramTransportWebhook}, nil

	case telegramTransportPolling:
		if err := api.DeleteWebhook(ctx); err != nil {
			log.Printf("[gateway/telegram] warning: deleteWebhook before polling failed: %v", err)
		}
		return telegramTransportDecision{
			Mode:       telegramTransportPolling,
			ReasonCode: "POLLING_FORCED",
			Reason:     "polling mode forced by configuration",
			Hint:       "Set CARRIER_TELEGRAM_TRANSPORT_MODE=auto to retry webhook automatically.",
		}, nil

	default:
		return telegramTransportDecision{}, fmt.Errorf("invalid CARRIER_TELEGRAM_TRANSPORT_MODE %q (expected auto|webhook|polling)", requestedMode)
	}
}

func runTelegramPollingLoop(
	ctx context.Context,
	cfg *GatewayConfig,
	api telegramAPI,
	daemon *DaemonClient,
	sessions *SessionStore,
	downloads *DownloadStore,
	rl *GatewayRateLimiter,
	onboard *OnboardStore,
) {
	pollTimeout := cfg.TelegramPollingTimeout
	if pollTimeout <= 0 {
		pollTimeout = 30
	}

	var offset int64
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		updates, err := api.GetUpdates(ctx, offset, pollTimeout)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("[gateway/telegram] polling error: %v", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}

		for _, update := range updates {
			if updateID := telegramUpdateID(update); updateID >= offset {
				offset = updateID + 1
			}

			envelope := NormalizeTelegramInboundEnvelope(update, sessions)
			if envelope == nil {
				continue
			}
			resp := RouteInboundChannel(ctx, envelope, daemon, sessions, downloads, rl, onboard)
			if err := sendTelegramGatewayResponse(ctx, api, envelope.ChatID, resp); err != nil {
				log.Printf("[gateway/telegram] sendMessage failed (chat=%s request=%s): %v", envelope.ChatID, envelope.RequestID, err)
			}
		}
	}
}

func sendTelegramGatewayResponse(ctx context.Context, api telegramAPI, chatID string, resp GatewayResponse) error {
	if mediaKind, mediaRef, caption, ok := selectTelegramRichAttachment(resp); ok {
		switch mediaKind {
		case "image":
			return api.SendPhoto(ctx, chatID, mediaRef, caption)
		case "document":
			return api.SendDocument(ctx, chatID, mediaRef, caption)
		}
	}

	rendered := RenderTelegramResponse(resp)
	text, _ := rendered["text"].(string)
	if strings.TrimSpace(text) == "" {
		return nil
	}
	disableWebPagePreview, _ := rendered["disable_web_page_preview"].(bool)
	return api.SendMessage(ctx, chatID, text, disableWebPagePreview)
}

func selectTelegramRichAttachment(resp GatewayResponse) (kind string, ref string, caption string, ok bool) {
	if resp.Result != "ok" || resp.RichContent == nil {
		return "", "", "", false
	}
	caption = strings.TrimSpace(resp.RichContent.PlainTextFallback())
	for _, attachment := range resp.RichContent.Attachments {
		ref = strings.TrimSpace(attachment.ExternalID)
		if ref == "" {
			if u := strings.TrimSpace(attachment.Path); strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") {
				ref = u
			}
		}
		if ref == "" {
			continue
		}
		switch strings.TrimSpace(attachment.Kind) {
		case "image":
			return "image", ref, caption, true
		case "document":
			return "document", ref, caption, true
		}
	}
	return "", "", "", false
}

func telegramUpdateID(update map[string]interface{}) int64 {
	raw := toID(update["update_id"])
	if raw == "" {
		return 0
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0
	}
	return id
}

func normalizeTelegramWebhookURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", errors.New("missing CARRIER_TELEGRAM_WEBHOOK_URL")
	}

	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("invalid webhook URL: %w", err)
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return "", errors.New("webhook URL must use https")
	}
	if strings.TrimSpace(u.Hostname()) == "" {
		return "", errors.New("webhook URL host is empty")
	}
	if !isPublicWebhookHost(u.Hostname()) {
		return "", errors.New("webhook URL host must be public and reachable by Telegram")
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/webhook/telegram"
	}
	return u.String(), nil
}

func isPublicWebhookHost(host string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(host))
	if trimmed == "" {
		return false
	}
	if trimmed == "localhost" || strings.HasSuffix(trimmed, ".local") {
		return false
	}
	if ip := net.ParseIP(trimmed); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() ||
			ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return false
		}
	}
	return true
}

func carrierTelegramDefaultCommands() []telegramBotCommand {
	return []telegramBotCommand{
		{Command: "pair", Description: "Link this chat with Carrier (/pair <code>)"},
		{Command: "chat", Description: "Chat with base agent (/chat <message>)"},
		{Command: "delegate", Description: "Create an execution from a goal (/delegate <goal>)"},
		{Command: "agents", Description: "List managed agents"},
		{Command: "status", Description: "Show agent status (/status [agent_id])"},
		{Command: "logs", Description: "Show logs (/logs [agent_id] [tail])"},
		{Command: "tools", Description: "List base-agent tools"},
		{Command: "providers", Description: "Show provider backends"},
		{Command: "sessions", Description: "List base-agent sessions"},
		{Command: "boundaries", Description: "Explain base-agent boundaries"},
	}
}
