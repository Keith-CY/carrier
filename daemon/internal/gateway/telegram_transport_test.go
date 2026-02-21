package gateway

import (
	"context"
	"strings"
	"testing"
)

type fakeTelegramAPI struct {
	setWebhookCalls int
	getInfoCalls    int
	deleteCalls     int

	setWebhookErr error
	getInfoErr    error
	deleteErr     error

	webhookInfo telegramWebhookInfo
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
	return nil
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

	mode, reason, err := resolveTelegramTransportMode(context.Background(), cfg, fake)
	if err != nil {
		t.Fatalf("resolveTelegramTransportMode error: %v", err)
	}
	if mode != telegramTransportWebhook {
		t.Fatalf("mode = %q, want %q", mode, telegramTransportWebhook)
	}
	if reason != "" {
		t.Fatalf("reason = %q, want empty", reason)
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

	mode, reason, err := resolveTelegramTransportMode(context.Background(), cfg, fake)
	if err != nil {
		t.Fatalf("resolveTelegramTransportMode error: %v", err)
	}
	if mode != telegramTransportPolling {
		t.Fatalf("mode = %q, want %q", mode, telegramTransportPolling)
	}
	if !strings.Contains(reason, "public") {
		t.Fatalf("reason = %q, want contains %q", reason, "public")
	}
	if fake.setWebhookCalls != 0 {
		t.Fatalf("setWebhookCalls = %d, want 0", fake.setWebhookCalls)
	}
	if fake.deleteCalls != 1 {
		t.Fatalf("deleteCalls = %d, want 1", fake.deleteCalls)
	}
}

func TestResolveTelegramTransportMode_WebhookInvalidURL(t *testing.T) {
	fake := &fakeTelegramAPI{}
	cfg := &GatewayConfig{
		TelegramTransportMode: telegramTransportWebhook,
		TelegramWebhookURL:    "http://example.com/webhook/telegram",
	}

	_, _, err := resolveTelegramTransportMode(context.Background(), cfg, fake)
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
