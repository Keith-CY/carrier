package baseagent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func mediaAnyToString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
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

func TestOpenAICompatibleMediaRuntimeTranscribePostsMultipartRequest(t *testing.T) {
	audioDir := t.TempDir()
	audioPath := filepath.Join(audioDir, "voice.ogg")
	if err := os.WriteFile(audioPath, []byte("OggS"), 0o600); err != nil {
		t.Fatalf("write audio fixture: %v", err)
	}

	var seenAuth string
	var seenModel string
	var seenLanguage string
	var seenFilename string
	var seenBytes []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio/transcriptions" {
			t.Fatalf("path=%q want /audio/transcriptions", r.URL.Path)
		}
		seenAuth = r.Header.Get("Authorization")
		reader, err := r.MultipartReader()
		if err != nil {
			t.Fatalf("MultipartReader: %v", err)
		}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("NextPart: %v", err)
			}
			body, err := io.ReadAll(part)
			if err != nil {
				t.Fatalf("ReadAll(%s): %v", part.FormName(), err)
			}
			switch part.FormName() {
			case "model":
				seenModel = string(body)
			case "language":
				seenLanguage = string(body)
			case "file":
				seenFilename = part.FileName()
				seenBytes = append([]byte(nil), body...)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"text": "transcribed speech"})
	}))
	defer srv.Close()

	runtime := NewOpenAICompatibleMediaRuntime(OpenAICompatibleMediaRuntimeConfig{
		BaseURL:  srv.URL,
		Token:    "TOKEN",
		Model:    "whisper-1",
		Language: "en",
		Client:   srv.Client(),
	})

	text, err := runtime.Transcribe(context.Background(), AttachmentRef{
		Kind:       "audio",
		Name:       "voice.ogg",
		Path:       audioPath,
		MediaType:  "audio/ogg",
		ExternalID: "tg-audio-9",
	})
	if err != nil {
		t.Fatalf("Transcribe error: %v", err)
	}
	if text != "transcribed speech" {
		t.Fatalf("text=%q want transcribed speech", text)
	}
	if seenAuth != "Bearer TOKEN" {
		t.Fatalf("Authorization=%q want Bearer TOKEN", seenAuth)
	}
	if seenModel != "whisper-1" {
		t.Fatalf("model=%q want whisper-1", seenModel)
	}
	if seenLanguage != "en" {
		t.Fatalf("language=%q want en", seenLanguage)
	}
	if seenFilename != "voice.ogg" {
		t.Fatalf("filename=%q want voice.ogg", seenFilename)
	}
	if string(seenBytes) != "OggS" {
		t.Fatalf("file bytes=%q want OggS", string(seenBytes))
	}
}

func TestOpenAICompatibleChatMediaRuntimeTranscribePostsInputAudioRequest(t *testing.T) {
	audioDir := t.TempDir()
	audioPath := filepath.Join(audioDir, "voice.wav")
	if err := os.WriteFile(audioPath, []byte("RIFFdemoWAVE"), 0o600); err != nil {
		t.Fatalf("write audio fixture: %v", err)
	}

	var seenAuth string
	var seenModel string
	var seenPrompt string
	var seenAudioData string
	var seenAudioFormat string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path=%q want /chat/completions", r.URL.Path)
		}
		seenAuth = r.Header.Get("Authorization")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		seenModel = strings.TrimSpace(mediaAnyToString(body["model"]))
		messages, ok := body["messages"].([]any)
		if !ok || len(messages) != 1 {
			t.Fatalf("messages=%#v", body["messages"])
		}
		first, ok := messages[0].(map[string]any)
		if !ok {
			t.Fatalf("first message=%#v", messages[0])
		}
		content, ok := first["content"].([]any)
		if !ok || len(content) != 2 {
			t.Fatalf("content=%#v", first["content"])
		}
		textBlock, ok := content[0].(map[string]any)
		if !ok {
			t.Fatalf("text block=%#v", content[0])
		}
		seenPrompt = strings.TrimSpace(mediaAnyToString(textBlock["text"]))
		audioBlock, ok := content[1].(map[string]any)
		if !ok {
			t.Fatalf("audio block=%#v", content[1])
		}
		inputAudio, ok := audioBlock["input_audio"].(map[string]any)
		if !ok {
			t.Fatalf("input_audio=%#v", audioBlock["input_audio"])
		}
		seenAudioData = strings.TrimSpace(mediaAnyToString(inputAudio["data"]))
		seenAudioFormat = strings.TrimSpace(mediaAnyToString(inputAudio["format"]))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{
				map[string]any{
					"message": map[string]any{
						"content": "transcribed via chat completions",
					},
				},
			},
		})
	}))
	defer srv.Close()

	runtime := NewOpenAICompatibleChatMediaRuntime(OpenAICompatibleChatMediaRuntimeConfig{
		BaseURL: srv.URL,
		Token:   "TOKEN",
		Model:   "google/gemini-2.0-flash-001",
		Client:  srv.Client(),
	})

	text, err := runtime.Transcribe(context.Background(), AttachmentRef{
		Kind:      "audio",
		Name:      "voice.wav",
		Path:      audioPath,
		MediaType: "audio/wav",
	})
	if err != nil {
		t.Fatalf("Transcribe error: %v", err)
	}
	if text != "transcribed via chat completions" {
		t.Fatalf("text=%q want transcribed via chat completions", text)
	}
	if seenAuth != "Bearer TOKEN" {
		t.Fatalf("Authorization=%q want Bearer TOKEN", seenAuth)
	}
	if seenModel != "google/gemini-2.0-flash-001" {
		t.Fatalf("model=%q want google/gemini-2.0-flash-001", seenModel)
	}
	if seenPrompt == "" || !strings.Contains(strings.ToLower(seenPrompt), "transcribe") {
		t.Fatalf("prompt=%q want transcription instruction", seenPrompt)
	}
	if seenAudioFormat != "wav" {
		t.Fatalf("format=%q want wav", seenAudioFormat)
	}
	gotBytes, err := base64.StdEncoding.DecodeString(seenAudioData)
	if err != nil {
		t.Fatalf("decode audio data: %v", err)
	}
	if !bytes.Equal(gotBytes, []byte("RIFFdemoWAVE")) {
		t.Fatalf("audio bytes=%q want RIFFdemoWAVE", string(gotBytes))
	}
}

func TestNewConfiguredMediaRuntimeUsesChatCompletionsForOpenRouter(t *testing.T) {
	writeDefaultModelConfigWithBaseURL(t, "openrouter", "openrouter/google/gemini-2.0-flash-001", "OPENROUTER_API_KEY", "https://openrouter.ai/api/v1")
	t.Setenv("OPENROUTER_API_KEY", "sk-openrouter")
	t.Setenv("CARRIER_TRANSCRIPTION_PROVIDER", "openrouter")

	runtime := NewConfiguredMediaRuntime()
	if runtime == nil {
		t.Fatal("expected configured media runtime")
	}
	typed, ok := runtime.(*openAICompatibleChatMediaRuntime)
	if !ok {
		t.Fatalf("runtime type = %T, want *openAICompatibleChatMediaRuntime", runtime)
	}
	if typed.model != "openrouter/healer-alpha" {
		t.Fatalf("runtime model = %q, want %q", typed.model, "openrouter/healer-alpha")
	}
}
