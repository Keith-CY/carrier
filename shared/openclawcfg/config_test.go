package openclawcfg

import "testing"

func TestProviderSecretPointerEscapesPathSegments(t *testing.T) {
	got := ProviderSecretPointer("foo~/bar")
	want := "/providers/foo~0~1bar/apiKey"
	if got != want {
		t.Fatalf("ProviderSecretPointer()=%q, want %q", got, want)
	}
}

func TestChannelTokenFieldTelegramUsesBotToken(t *testing.T) {
	if got := ChannelTokenField("telegram"); got != "botToken" {
		t.Fatalf("ChannelTokenField(telegram)=%q, want botToken", got)
	}
	if got := ChannelTokenField("discord"); got != "token" {
		t.Fatalf("ChannelTokenField(discord)=%q, want token", got)
	}
}
