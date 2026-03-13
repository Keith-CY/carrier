package baseagent

import (
	"bytes"
	"context"
	"encoding/base64"
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
		"model": r.model,
		"input": text,
		"voice": voice,
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

func (r *openAICompatibleChatMediaRuntime) Transcribe(ctx context.Context, attachment AttachmentRef) (string, error) {
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
	reqBody := map[string]any{
		"model": r.model,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "text",
						"text": "Transcribe this audio and return only the spoken text.",
					},
					{
						"type": "input_audio",
						"input_audio": map[string]any{
							"data":   base64.StdEncoding.EncodeToString(data),
							"format": detectAudioInputFormat(attachment),
						},
					},
				},
			},
		},
	}
	rawBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal transcription request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/chat/completions", bytes.NewReader(rawBody))
	if err != nil {
		return "", fmt.Errorf("build transcription request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Content-Type", "application/json")

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
		Choices []struct {
			Message map[string]any `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("decode transcription response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("transcription response returned no choices")
	}
	text := strings.TrimSpace(extractModelContent(parsed.Choices[0].Message))
	if text == "" {
		return "", fmt.Errorf("transcription response returned empty text")
	}
	return text, nil
}

func (r *openAICompatibleChatMediaRuntime) SynthesizeSpeech(ctx context.Context, req SpeechSynthesisRequest) (AttachmentRef, error) {
	_ = ctx
	_ = req
	return AttachmentRef{}, &mediaUnsupportedError{message: "speech output is not supported for the configured media provider"}
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

func detectAudioInputFormat(ref AttachmentRef) string {
	for _, candidate := range []string{
		strings.TrimSpace(ref.MediaType),
		strings.TrimSpace(ref.MIMEType),
		strings.TrimSpace(filepath.Ext(ref.Name)),
		strings.TrimSpace(filepath.Ext(ref.Path)),
	} {
		switch strings.ToLower(strings.Trim(candidate, ".")) {
		case "audio/wav", "audio/x-wav", "wav":
			return "wav"
		case "audio/mpeg", "audio/mp3", "mp3":
			return "mp3"
		case "audio/ogg", "audio/oga", "ogg", "oga":
			return "ogg"
		case "audio/webm", "webm":
			return "webm"
		case "audio/flac", "flac":
			return "flac"
		}
	}
	return "wav"
}

func normalizeTranscriptionModel(providerID, modelID string) string {
	trimmedProvider := strings.ToLower(strings.TrimSpace(providerID))
	trimmedModel := strings.TrimSpace(modelID)
	if trimmedProvider == "openrouter" && strings.HasPrefix(trimmedModel, "openrouter/") && strings.Count(trimmedModel, "/") == 1 {
		return trimmedModel
	}
	return strings.TrimSpace(normalizeModelForProvider(providerID, modelID))
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
