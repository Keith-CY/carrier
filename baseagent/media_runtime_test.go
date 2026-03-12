package baseagent

import (
	"context"
	"strings"
	"testing"
)

type mediaRuntimeFake struct {
	transcribeCalls []AttachmentRef
	transcription   string
	err             error
}

func (f *mediaRuntimeFake) Transcribe(_ context.Context, attachment AttachmentRef) (string, error) {
	f.transcribeCalls = append(f.transcribeCalls, attachment)
	if f.err != nil {
		return "", f.err
	}
	return f.transcription, nil
}

func TestMediaRuntimeUnsupportedWithoutRuntime(t *testing.T) {
	rt := NewRuntime(&runtimeServiceFake{}, nil)

	resp, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "audio-unsupported",
		Attachments: []AttachmentRef{
			{Kind: "audio", Name: "voice.ogg", ExternalID: "tg-audio-1"},
		},
	})
	if err != nil {
		t.Fatalf("runtime chat: %v", err)
	}
	if !strings.Contains(strings.ToLower(resp.Message), "audio attachments are not supported") {
		t.Fatalf("expected bounded unsupported response, got %+v", resp)
	}
}

func TestRuntimeChatTranscribesAudioAttachment(t *testing.T) {
	provider := &scriptedTextProvider{
		name:    "audio-provider",
		replies: []string{"transcription handled"},
	}
	media := &mediaRuntimeFake{transcription: "weather in tokyo"}

	rt := NewRuntime(&runtimeServiceFake{}, nil, WithMediaRuntime(media))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	resp, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "audio-supported",
		Attachments: []AttachmentRef{
			{Kind: "voice", Name: "voice.ogg", ExternalID: "tg-audio-2"},
		},
	})
	if err != nil {
		t.Fatalf("runtime chat: %v", err)
	}
	if resp.Message != "transcription handled" {
		t.Fatalf("unexpected runtime response: %+v", resp)
	}
	if len(media.transcribeCalls) != 1 || media.transcribeCalls[0].ExternalID != "tg-audio-2" {
		t.Fatalf("unexpected media runtime calls: %+v", media.transcribeCalls)
	}
	if len(provider.requests) != 1 || !strings.Contains(provider.requests[0].UserMessage, "weather in tokyo") {
		t.Fatalf("expected provider to receive transcription, got %+v", provider.requests)
	}
}

func TestRuntimeChatTranscribesAudioAttachmentByMediaType(t *testing.T) {
	provider := &scriptedTextProvider{
		name:    "audio-provider",
		replies: []string{"transcription handled"},
	}
	media := &mediaRuntimeFake{transcription: "voice memo transcript"}

	rt := NewRuntime(&runtimeServiceFake{}, nil, WithMediaRuntime(media))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	resp, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "audio-media-type",
		Attachments: []AttachmentRef{
			{Name: "voice.ogg", MediaType: "audio/ogg", ExternalID: "tg-audio-3"},
		},
	})
	if err != nil {
		t.Fatalf("runtime chat: %v", err)
	}
	if resp.Message != "transcription handled" {
		t.Fatalf("unexpected runtime response: %+v", resp)
	}
	if len(media.transcribeCalls) != 1 || media.transcribeCalls[0].ExternalID != "tg-audio-3" {
		t.Fatalf("unexpected media runtime calls: %+v", media.transcribeCalls)
	}
	if len(provider.requests) != 1 || !strings.Contains(provider.requests[0].UserMessage, "voice memo transcript") {
		t.Fatalf("expected provider to receive transcription, got %+v", provider.requests)
	}
}
