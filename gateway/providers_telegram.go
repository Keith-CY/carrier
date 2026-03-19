package gateway

import (
	"carrier/baseagent"
	"fmt"
	"strings"
)

// ParseTelegramMessage parses a Telegram update payload into a normalized message.
func ParseTelegramMessage(payload map[string]interface{}) *NormalizedMessage {
	var message map[string]interface{}
	for _, field := range []string{"message", "edited_message", "channel_post", "edited_channel_post"} {
		if m, ok := asMap(payload[field]); ok {
			message = m
			break
		}
	}
	if message == nil {
		return nil
	}

	chat, _ := asMap(message["chat"])
	chatID := toID(chat["id"])
	messageID := toID(message["message_id"])
	rawText := firstString(message["text"], message["caption"])
	attachments := extractTelegramAttachments(message, chatID, messageID)
	if chatID == "" || (rawText == "" && len(attachments) == 0) {
		return nil
	}

	requestID := buildRequestID("tg", toID(payload["update_id"]), toID(message["message_id"]))
	out := &NormalizedMessage{
		Provider:    "telegram",
		ChatID:      chatID,
		RequestID:   requestID,
		RawText:     rawText,
		Attachments: attachments,
	}
	if parsed := parseCommandText(rawText); parsed != nil {
		out.Command = &NormalizedCommand{
			Provider:  out.Provider,
			ChatID:    out.ChatID,
			RequestID: out.RequestID,
			Command:   parsed.command,
			Args:      parsed.args,
			RawText:   rawText,
		}
	}
	return out
}

// ParseTelegramUpdate parses a Telegram update payload into a normalized command.
func ParseTelegramUpdate(payload map[string]interface{}) *NormalizedCommand {
	msg := ParseTelegramMessage(payload)
	if msg == nil {
		return nil
	}
	return msg.Command
}

// VerifyTelegramSecret does a constant-time comparison of the provided secret.
// If no expected secret is configured, all requests are allowed.
func VerifyTelegramSecret(provided, expected string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return true
	}
	provided = strings.TrimSpace(provided)
	if provided == "" {
		return false
	}
	return constantTimeStringEqual(provided, expected)
}

// RenderTelegramResponse renders a GatewayResponse for Telegram.
func RenderTelegramResponse(resp GatewayResponse) map[string]interface{} {
	var lines []string
	if resp.Result == "ok" {
		message := strings.TrimSpace(resp.Message)
		if message == "" && resp.RichContent != nil {
			message = strings.TrimSpace(resp.RichContent.PlainTextFallback())
		}
		if message == "" {
			message = "completed"
		}
		lines = append(lines, "✅ "+message)
		if resp.SessionToken != "" {
			lines = append(lines, "Session token: "+resp.SessionToken)
		}
		if resp.HandoffID != "" {
			status := resp.HandoffStatus
			if status == "" {
				status = "pending"
			}
			lines = append(lines, fmt.Sprintf("Handoff: %s (%s)", resp.HandoffID, status))
		}
		if resp.DownloadURL != "" {
			lines = append(lines, "Download: "+resp.DownloadURL)
		}
	} else {
		code := resp.ErrorCode
		if code == "" {
			code = "E_UNKNOWN"
		}
		lines = append(lines, fmt.Sprintf("❌ %s: %s", code, resp.Message))
	}
	result := map[string]interface{}{
		"text": strings.Join(lines, "\n"),
	}
	if resp.DownloadURL != "" {
		result["disable_web_page_preview"] = true
	}
	return result
}

func extractTelegramAttachments(message map[string]interface{}, chatID, messageID string) []baseagent.AttachmentRef {
	if len(message) == 0 {
		return nil
	}
	var attachments []baseagent.AttachmentRef
	appendAttachment := func(ref baseagent.AttachmentRef) {
		if strings.TrimSpace(ref.Name) == "" && strings.TrimSpace(ref.ExternalID) == "" && strings.TrimSpace(ref.Path) == "" {
			return
		}
		attachments = append(attachments, ref)
	}

	if document, ok := asMap(message["document"]); ok {
		appendAttachment(telegramAttachmentRef("document", document, chatID, messageID))
	}
	if audio, ok := asMap(message["audio"]); ok {
		appendAttachment(telegramAttachmentRef("audio", audio, chatID, messageID))
	}
	if voice, ok := asMap(message["voice"]); ok {
		appendAttachment(telegramAttachmentRef("voice", voice, chatID, messageID))
	}
	if video, ok := asMap(message["video"]); ok {
		appendAttachment(telegramAttachmentRef("video", video, chatID, messageID))
	}
	if photos, ok := message["photo"].([]interface{}); ok && len(photos) > 0 {
		best := map[string]interface{}{}
		bestSize := float64(-1)
		for _, raw := range photos {
			photo, ok := asMap(raw)
			if !ok {
				continue
			}
			size := toFloat(photo["file_size"])
			if size >= bestSize {
				best = photo
				bestSize = size
			}
		}
		appendAttachment(telegramAttachmentRef("image", best, chatID, messageID))
	}
	return attachments
}

func telegramAttachmentRef(kind string, payload map[string]interface{}, chatID, messageID string) baseagent.AttachmentRef {
	fileID := firstString(payload["file_id"])
	fileUniqueID := firstString(payload["file_unique_id"])
	ref := baseagent.AttachmentRef{
		ID:         strings.TrimSpace(firstString(fileUniqueID, fileID)),
		Kind:       strings.TrimSpace(kind),
		Name:       firstString(payload["file_name"]),
		MIMEType:   firstString(payload["mime_type"]),
		MediaType:  firstString(payload["mime_type"]),
		SizeBytes:  int64(toFloat(payload["file_size"])),
		ExternalID: firstString(fileID, fileUniqueID),
		Source:     "telegram",
		SourceMetadata: map[string]string{
			"transport":          "telegram",
			"chat_id":            strings.TrimSpace(chatID),
			"message_id":         strings.TrimSpace(messageID),
			"telegram_file_id":   strings.TrimSpace(fileID),
			"telegram_unique_id": strings.TrimSpace(fileUniqueID),
		},
	}
	if ref.Name == "" {
		switch ref.Kind {
		case "voice":
			ref.Name = "voice-message.ogg"
		case "image":
			ref.Name = "telegram-photo.jpg"
		case "audio":
			ref.Name = "telegram-audio"
		case "video":
			ref.Name = "telegram-video"
		case "document":
			ref.Name = "telegram-document"
		}
	}
	if ref.MediaType == "" {
		ref.MediaType = defaultTelegramMediaType(ref.Kind)
	}
	if ref.MIMEType == "" {
		ref.MIMEType = ref.MediaType
	}
	if ref.ID == "" {
		ref.ID = strings.TrimSpace(firstString(ref.ExternalID, ref.Name))
	}
	return ref
}

func defaultTelegramMediaType(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "image":
		return "image/jpeg"
	case "voice":
		return "audio/ogg"
	case "audio":
		return "audio/mpeg"
	case "video":
		return "video/mp4"
	case "document", "file":
		return "application/octet-stream"
	default:
		return ""
	}
}

// RenderTelegramWebhookResponse wraps a gateway response as a Telegram Bot API
// webhook response payload (sendMessage method call).
func RenderTelegramWebhookResponse(resp GatewayResponse, chatID string) map[string]interface{} {
	base := RenderTelegramResponse(resp)
	text, _ := base["text"].(string)
	caption := strings.TrimSpace(text)
	if mediaKind, mediaRef, _, ok := selectTelegramRichAttachment(resp); ok {
		if rendered := buildTelegramWebhookMediaResponse(mediaKind, chatID, mediaRef, caption); rendered != nil {
			return rendered
		}
	}
	if strings.TrimSpace(resp.DownloadURL) != "" {
		return buildTelegramWebhookMediaResponse("document", chatID, strings.TrimSpace(resp.DownloadURL), caption)
	}
	out := map[string]interface{}{
		"method":  "sendMessage",
		"chat_id": chatID,
		"text":    text,
	}
	if v, ok := base["disable_web_page_preview"]; ok {
		out["disable_web_page_preview"] = v
	}
	return out
}

func buildTelegramWebhookMediaResponse(kind string, chatID string, ref string, caption string) map[string]interface{} {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	method, field := telegramMediaMethodAndField(kind)
	if method == "" || field == "" {
		return nil
	}
	out := map[string]interface{}{
		"method":  method,
		"chat_id": chatID,
		field:     ref,
	}
	if caption != "" {
		out["caption"] = caption
	}
	return out
}

func telegramMediaMethodAndField(kind string) (method string, field string) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "image":
		return "sendPhoto", "photo"
	case "document":
		return "sendDocument", "document"
	case "audio":
		return "sendAudio", "audio"
	case "voice":
		return "sendVoice", "voice"
	case "video":
		return "sendVideo", "video"
	default:
		return "", ""
	}
}
