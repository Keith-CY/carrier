package baseagent

import (
	"context"
	"fmt"
	"strings"
)

type MediaRuntime interface {
	Transcribe(ctx context.Context, attachment AttachmentRef) (string, error)
}

func isAudioAttachment(ref AttachmentRef) bool {
	switch strings.ToLower(strings.TrimSpace(ref.Kind)) {
	case "audio", "voice":
		return true
	default:
		return false
	}
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
