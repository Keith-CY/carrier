package baseagent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

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
