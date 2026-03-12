package baseagent

import (
	"bytes"
	"context"
	"encoding/json"
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
}

type OpenAICompatibleMediaRuntimeConfig struct {
	BaseURL  string
	Token    string
	Model    string
	Language string
	Client   *http.Client
}

type openAICompatibleMediaRuntime struct {
	baseURL  string
	token    string
	model    string
	language string
	client   *http.Client
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

func NewConfiguredMediaRuntime() MediaRuntime {
	providerID := strings.TrimSpace(os.Getenv("CARRIER_TRANSCRIPTION_PROVIDER"))
	cfg, err := resolveLLMRuntimeConfigForProvider(providerID)
	if err != nil || cfg == nil {
		return nil
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

func isAudioAttachment(ref AttachmentRef) bool {
	switch strings.ToLower(strings.TrimSpace(ref.Kind)) {
	case "audio", "voice":
		return true
	}
	for _, mediaType := range []string{ref.MediaType, ref.MIMEType} {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(mediaType)), "audio/") {
			return true
		}
	}
	return false
}

func (r *Runtime) prepareMediaRequest(ctx context.Context, req ChatRequest) (ChatRequest, *ChatResponse, error) {
	if r == nil || len(req.Attachments) == 0 {
		return req, nil, nil
	}
	audioAttachments := make([]AttachmentRef, 0, len(req.Attachments))
	for _, attachment := range req.Attachments {
		if isAudioAttachment(attachment) {
			audioAttachments = append(audioAttachments, attachment)
		}
	}
	if len(audioAttachments) == 0 {
		return req, nil, nil
	}
	if r.mediaRuntime == nil {
		return req, &ChatResponse{
			Message: "Audio attachments are not supported by this runtime yet.",
			Action:  "unsupported",
		}, nil
	}

	transcripts := make([]string, 0, len(audioAttachments))
	for _, attachment := range audioAttachments {
		text, err := r.mediaRuntime.Transcribe(ctx, attachment)
		if err != nil {
			return req, &ChatResponse{
				Message: fmt.Sprintf("Audio transcription is unavailable right now: %v", err),
				Action:  "unsupported",
			}, nil
		}
		text = strings.TrimSpace(text)
		if text != "" {
			transcripts = append(transcripts, text)
		}
	}
	if len(transcripts) == 0 {
		return req, nil, nil
	}

	existing := strings.TrimSpace(req.Message)
	if existing == "" {
		req.Message = strings.Join(transcripts, "\n")
	} else {
		req.Message = existing + "\n\nTranscribed audio:\n" + strings.Join(transcripts, "\n")
	}
	return req, nil, nil
}
