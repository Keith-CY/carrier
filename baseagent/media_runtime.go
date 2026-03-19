package baseagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"carrier/shared/catalog"
)

type MediaRuntime interface {
	Transcribe(ctx context.Context, attachment AttachmentRef) (string, error)
	SynthesizeSpeech(ctx context.Context, req SpeechSynthesisRequest) (AttachmentRef, error)
}

type SpeechSynthesisRequest struct {
	Text   string `json:"text"`
	Voice  string `json:"voice,omitempty"`
	Format string `json:"format,omitempty"`
}

type OpenAICompatibleMediaRuntimeConfig struct {
	BaseURL  string
	Token    string
	Model    string
	Language string
	Client   *http.Client
}

type OpenAICompatibleChatMediaRuntimeConfig struct {
	BaseURL string
	Token   string
	Model   string
	Client  *http.Client
}

type openAICompatibleMediaRuntime struct {
	baseURL  string
	token    string
	model    string
	language string
	client   *http.Client
}

type openAICompatibleChatMediaRuntime struct {
	baseURL string
	token   string
	model   string
	client  *http.Client
}

type mediaUnsupportedError struct {
	message string
}

func (e *mediaUnsupportedError) Error() string {
	if e == nil {
		return ""
	}
	return strings.TrimSpace(e.message)
}

func isMediaUnsupportedError(err error) bool {
	var unsupported *mediaUnsupportedError
	return errors.As(err, &unsupported)
}

func NewOpenAICompatibleMediaRuntime(cfg OpenAICompatibleMediaRuntimeConfig) MediaRuntime {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	token := strings.TrimSpace(cfg.Token)
	model := strings.TrimSpace(cfg.Model)
	if baseURL == "" || token == "" || model == "" {
		return nil
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: defaultBaseAgentLLMTimeout}
	}
	return &openAICompatibleMediaRuntime{
		baseURL:  baseURL,
		token:    token,
		model:    model,
		language: strings.TrimSpace(cfg.Language),
		client:   client,
	}
}

func NewOpenAICompatibleChatMediaRuntime(cfg OpenAICompatibleChatMediaRuntimeConfig) MediaRuntime {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	token := strings.TrimSpace(cfg.Token)
	model := strings.TrimSpace(cfg.Model)
	if baseURL == "" || token == "" || model == "" {
		return nil
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: defaultBaseAgentLLMTimeout}
	}
	return &openAICompatibleChatMediaRuntime{
		baseURL: baseURL,
		token:   token,
		model:   model,
		client:  client,
	}
}

func NewConfiguredMediaRuntime() MediaRuntime {
	providerID := strings.TrimSpace(os.Getenv("CARRIER_TRANSCRIPTION_PROVIDER"))
	cfg, err := resolveLLMRuntimeConfigForProvider(providerID)
	if err != nil || cfg == nil {
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(cfg.ProviderID), "openrouter") {
		model := strings.TrimSpace(normalizeTranscriptionModel(cfg.ProviderID, os.Getenv("CARRIER_TRANSCRIPTION_MODEL")))
		if model == "" {
			model = "openrouter/healer-alpha"
		}
		return NewOpenAICompatibleChatMediaRuntime(OpenAICompatibleChatMediaRuntimeConfig{
			BaseURL: cfg.BaseURL,
			Token:   cfg.Token,
			Model:   model,
			Client:  nil,
		})
	}
	if !strings.EqualFold(strings.TrimSpace(catalog.ProtocolFamilyForProvider(cfg.ProviderID)), "openai-compatible") {
		return nil
	}
	model := strings.TrimSpace(os.Getenv("CARRIER_TRANSCRIPTION_MODEL"))
	if model == "" {
		model = "whisper-1"
	}
	return NewOpenAICompatibleMediaRuntime(OpenAICompatibleMediaRuntimeConfig{
		BaseURL:  cfg.BaseURL,
		Token:    cfg.Token,
		Model:    model,
		Language: strings.TrimSpace(os.Getenv("CARRIER_TRANSCRIPTION_LANGUAGE")),
	})
}

func (r *openAICompatibleMediaRuntime) Transcribe(ctx context.Context, attachment AttachmentRef) (string, error) {
	if r == nil {
		return "", fmt.Errorf("media runtime is unavailable")
	}
	path := strings.TrimSpace(attachment.Path)
	if path == "" {
		return "", fmt.Errorf("audio attachment path is required for transcription")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read audio attachment: %w", err)
	}
	filename := strings.TrimSpace(attachment.Name)
	if filename == "" {
		filename = filepath.Base(path)
	}
	if filename == "" || filename == "." || filename == string(filepath.Separator) {
		filename = "audio.bin"
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("model", r.model); err != nil {
		return "", fmt.Errorf("write transcription model: %w", err)
	}
	if r.language != "" {
		if err := writer.WriteField("language", r.language); err != nil {
			return "", fmt.Errorf("write transcription language: %w", err)
		}
	}
	filePart, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("create transcription file part: %w", err)
	}
	if _, err := filePart.Write(data); err != nil {
		return "", fmt.Errorf("write transcription file part: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("finalize transcription request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/audio/transcriptions", &body)
	if err != nil {
		return "", fmt.Errorf("build transcription request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("transcription request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, baseAgentMaxModelErrorBytes))
		if parsedErr := parseModelError(resp.StatusCode, raw); parsedErr != nil {
			return "", parsedErr
		}
		return "", fmt.Errorf("transcription request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, baseAgentMaxModelRespBytes))
	if err != nil {
		return "", fmt.Errorf("read transcription response: %w", err)
	}
	var parsed struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("decode transcription response: %w", err)
	}
	text := strings.TrimSpace(parsed.Text)
	if text == "" {
		return "", fmt.Errorf("transcription response returned empty text")
	}
	return text, nil
}

func (r *openAICompatibleMediaRuntime) SynthesizeSpeech(ctx context.Context, req SpeechSynthesisRequest) (AttachmentRef, error) {
	if r == nil {
		return AttachmentRef{}, fmt.Errorf("media runtime is unavailable")
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return AttachmentRef{}, fmt.Errorf("speech text is required")
	}
	voice := normalizeSpeechVoice(req.Voice)
	format := normalizeSpeechFormat(req.Format)
	rawBody, err := json.Marshal(map[string]any{
		"model":  r.model,
		"input":  text,
		"voice":  voice,
		"format": format,
	})
	if err != nil {
		return AttachmentRef{}, fmt.Errorf("marshal speech request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/audio/speech", bytes.NewReader(rawBody))
	if err != nil {
		return AttachmentRef{}, fmt.Errorf("build speech request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+r.token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(httpReq)
	if err != nil {
		return AttachmentRef{}, fmt.Errorf("speech request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, baseAgentMaxModelErrorBytes))
		if parsedErr := parseModelError(resp.StatusCode, raw); parsedErr != nil {
			return AttachmentRef{}, parsedErr
		}
		return AttachmentRef{}, fmt.Errorf("speech request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, baseAgentMaxModelRespBytes))
	if err != nil {
		return AttachmentRef{}, fmt.Errorf("read speech response: %w", err)
	}
	if len(data) == 0 {
		return AttachmentRef{}, fmt.Errorf("speech response returned empty audio")
	}
	return persistSynthesizedAudioAttachment(data, format), nil
}

func normalizeSpeechVoice(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "alloy"
	}
	return trimmed
}

func normalizeSpeechFormat(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "mp3":
		return "mp3"
	case "wav", "opus", "aac", "flac", "pcm":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return "mp3"
	}
}

func speechFormatExtension(format string) string {
	switch normalizeSpeechFormat(format) {
	case "wav":
		return ".wav"
	case "opus":
		return ".opus"
	case "aac":
		return ".aac"
	case "flac":
		return ".flac"
	case "pcm":
		return ".pcm"
	default:
		return ".mp3"
	}
}

func speechFormatMediaType(format string) string {
	switch normalizeSpeechFormat(format) {
	case "wav":
		return "audio/wav"
	case "opus":
		return "audio/ogg"
	case "aac":
		return "audio/aac"
	case "flac":
		return "audio/flac"
	case "pcm":
		return "audio/L16"
	default:
		return "audio/mpeg"
	}
}

func persistSynthesizedAudioAttachment(data []byte, format string) AttachmentRef {
	ext := speechFormatExtension(format)
	tmpFile, err := os.CreateTemp("", "carrier-speech-*"+ext)
	if err != nil {
		panic(fmt.Sprintf("persist synthesized audio attachment: %v", err))
	}
	defer tmpFile.Close()
	if _, err := tmpFile.Write(data); err != nil {
		panic(fmt.Sprintf("write synthesized audio attachment: %v", err))
	}
	_ = tmpFile.Chmod(0o600)
	name := "speech" + ext
	id := strings.TrimSuffix(filepath.Base(tmpFile.Name()), ext)
	if id == "" {
		id = "speech"
	}
	return AttachmentRef{
		ID:         id,
		Kind:       "audio",
		OutputRole: "generated",
		Path:       tmpFile.Name(),
		Name:       name,
		MIMEType:   speechFormatMediaType(format),
		MediaType:  speechFormatMediaType(format),
		SizeBytes:  int64(len(data)),
		Source:     "media_runtime",
	}
}
