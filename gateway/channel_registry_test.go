package gateway

import "testing"

func TestChannelRegistryDescriptors(t *testing.T) {
	descriptors := SupportedChannelDescriptors()
	if len(descriptors) != 4 {
		t.Fatalf("expected 4 public channel descriptors, got %d", len(descriptors))
	}

	byID := make(map[ChannelID]ChannelDescriptor, len(descriptors))
	for _, descriptor := range descriptors {
		byID[descriptor.ID] = descriptor
	}

	telegram, ok := byID[ChannelTelegram]
	if !ok {
		t.Fatal("expected telegram descriptor")
	}
	if !telegram.Capabilities.SupportsPairing || !telegram.Capabilities.SupportsPolling || !telegram.Capabilities.SupportsWebhook || !telegram.Capabilities.RequiresBotToken {
		t.Fatalf("unexpected telegram capabilities: %+v", telegram.Capabilities)
	}
	if !telegram.Capabilities.SupportsGatewayCmd || !telegram.Capabilities.SupportsProviderSetup {
		t.Fatalf("telegram should support gateway command and provider setup flows: %+v", telegram.Capabilities)
	}

	discord, ok := byID[ChannelDiscord]
	if !ok {
		t.Fatal("expected discord descriptor")
	}
	if discord.Capabilities.SupportsPairing {
		t.Fatalf("discord should not expose pairing flow: %+v", discord.Capabilities)
	}
	if !discord.Capabilities.SupportsWebhook || !discord.Capabilities.RequiresBotToken || !discord.Capabilities.RequiresWebhookSecret {
		t.Fatalf("unexpected discord capabilities: %+v", discord.Capabilities)
	}
	if !discord.Capabilities.SupportsGatewayCmd || !discord.Capabilities.SupportsProviderSetup {
		t.Fatalf("discord should support gateway command and provider setup flows: %+v", discord.Capabilities)
	}

	feishu, ok := byID[ChannelFeishu]
	if !ok {
		t.Fatal("expected feishu descriptor")
	}
	if feishu.Capabilities.SupportsPairing {
		t.Fatalf("feishu should not expose pairing flow: %+v", feishu.Capabilities)
	}
	if !feishu.Capabilities.SupportsWebhook || !feishu.Capabilities.RequiresBotToken || !feishu.Capabilities.RequiresWebhookSecret {
		t.Fatalf("unexpected feishu capabilities: %+v", feishu.Capabilities)
	}
	if !feishu.Capabilities.SupportsGatewayCmd || !feishu.Capabilities.SupportsProviderSetup {
		t.Fatalf("feishu should support gateway command and provider setup flows: %+v", feishu.Capabilities)
	}

	webui, ok := byID[ChannelWebUI]
	if !ok {
		t.Fatal("expected webui descriptor")
	}
	if webui.Capabilities.RequiresBotToken || webui.Capabilities.SupportsWebhook || webui.Capabilities.SupportsPolling {
		t.Fatalf("webui should be local-only: %+v", webui.Capabilities)
	}
	if !webui.Capabilities.SupportsWebUI {
		t.Fatalf("webui should advertise local webui support: %+v", webui.Capabilities)
	}
	if webui.Capabilities.SupportsGatewayCmd || webui.Capabilities.SupportsProviderSetup {
		t.Fatalf("webui should not expose gateway command/provider setup flows: %+v", webui.Capabilities)
	}
}

func TestChannelRegistryRejectsUnsupportedChannels(t *testing.T) {
	if _, ok := LookupChannelDescriptor("telegram"); !ok {
		t.Fatal("expected telegram lookup to succeed")
	}
	if _, ok := LookupChannelDescriptor("webui"); !ok {
		t.Fatal("expected webui lookup to succeed")
	}

	got, err := NormalizeChannelID("  Discord ")
	if err != nil {
		t.Fatalf("normalize discord: %v", err)
	}
	if got != ChannelDiscord {
		t.Fatalf("NormalizeChannelID(discord) = %q, want %q", got, ChannelDiscord)
	}

	if _, err := NormalizeChannelID("slack"); err == nil {
		t.Fatal("expected unsupported channel to fail normalization")
	}
}
