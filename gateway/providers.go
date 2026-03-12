package gateway

import (
	"carrier/baseagent"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// NormalizedCommand is a platform-agnostic parsed command.
type NormalizedCommand struct {
	Provider  string
	ChatID    string
	RequestID string
	Command   string
	Args      []string
	RawText   string
}

// NormalizedMessage is a platform-agnostic parsed inbound message.
// Command is nil when the message is plain chat text (non-slash command).
type NormalizedMessage struct {
	Provider    string
	ChatID      string
	RequestID   string
	RawText     string
	Attachments []baseagent.AttachmentRef
	Command     *NormalizedCommand
}

// ToGatewayInput converts a normalized command to the gateway input string.
func ToGatewayInput(nc *NormalizedCommand) string {
	parts := []string{nc.Provider, nc.ChatID, nc.RequestID, nc.Command}
	parts = append(parts, nc.Args...)
	return strings.Join(parts, " ")
}

// --- Telegram ---

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
	rawText := firstString(message["text"], message["caption"])
	attachments := extractTelegramAttachments(message)
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

func extractTelegramAttachments(message map[string]interface{}) []baseagent.AttachmentRef {
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
		appendAttachment(telegramAttachmentRef("document", document))
	}
	if audio, ok := asMap(message["audio"]); ok {
		appendAttachment(telegramAttachmentRef("audio", audio))
	}
	if voice, ok := asMap(message["voice"]); ok {
		appendAttachment(telegramAttachmentRef("voice", voice))
	}
	if video, ok := asMap(message["video"]); ok {
		appendAttachment(telegramAttachmentRef("video", video))
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
		appendAttachment(telegramAttachmentRef("image", best))
	}
	return attachments
}

func telegramAttachmentRef(kind string, payload map[string]interface{}) baseagent.AttachmentRef {
	ref := baseagent.AttachmentRef{
		Kind:       strings.TrimSpace(kind),
		Name:       firstString(payload["file_name"]),
		MIMEType:   firstString(payload["mime_type"]),
		SizeBytes:  int64(toFloat(payload["file_size"])),
		ExternalID: firstString(payload["file_id"], payload["file_unique_id"]),
		Source:     "telegram",
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
	return ref
}

// RenderTelegramWebhookResponse wraps a gateway response as a Telegram Bot API
// webhook response payload (sendMessage method call).
func RenderTelegramWebhookResponse(resp GatewayResponse, chatID string) map[string]interface{} {
	base := RenderTelegramResponse(resp)
	text, _ := base["text"].(string)
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

// --- Discord ---

// VerifyDiscordSignature verifies the Ed25519 signature on a Discord webhook request.
func VerifyDiscordSignature(r *http.Request, body []byte, publicKeyHex string, maxAgeSec int) bool {
	publicKeyHex = strings.TrimSpace(publicKeyHex)
	if publicKeyHex == "" {
		return false
	}
	sigHex := strings.TrimSpace(r.Header.Get("X-Signature-Ed25519"))
	timestamp := strings.TrimSpace(r.Header.Get("X-Signature-Timestamp"))
	if sigHex == "" || timestamp == "" {
		return false
	}

	if maxAgeSec > 0 {
		ts, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil {
			return false
		}
		age := math.Abs(float64(time.Now().Unix() - ts))
		if age > float64(maxAgeSec) {
			return false
		}
	}

	sig, err := hex.DecodeString(sigHex)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return false
	}
	pubKeyBytes, err := hex.DecodeString(publicKeyHex)
	if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize {
		return false
	}

	// Go's ed25519.Verify takes raw 32-byte keys directly (no SPKI wrapping needed)
	message := append([]byte(timestamp), body...)
	return ed25519.Verify(ed25519.PublicKey(pubKeyBytes), message, sig)
}

// ParseDiscordMessage parses a Discord payload into a normalized message.
func ParseDiscordMessage(payload map[string]interface{}) *NormalizedMessage {
	// Slash-command interaction (type=2)
	if t := toFloat(payload["type"]); t == 2 {
		data, _ := asMap(payload["data"])
		cmdName := firstString(data["name"])
		if cmdName == "" {
			return nil
		}
		options := flattenDiscordOptions(data["options"])
		chatID := firstString(payload["channel_id"], payload["guild_id"])
		if chatID == "" {
			return nil
		}
		requestID := buildRequestID("dc", toID(payload["id"]))
		cmd := &NormalizedCommand{
			Provider:  "discord",
			ChatID:    chatID,
			RequestID: requestID,
			Command:   normalizeCommandName("/" + cmdName),
			Args:      options,
			RawText:   "/" + cmdName + " " + strings.Join(options, " "),
		}
		return &NormalizedMessage{
			Provider:  cmd.Provider,
			ChatID:    cmd.ChatID,
			RequestID: cmd.RequestID,
			RawText:   cmd.RawText,
			Command:   cmd,
		}
	}

	// Message-create style
	content := firstString(payload["content"])
	chatID := firstString(payload["channel_id"], payload["guild_id"])
	if content == "" || chatID == "" {
		return nil
	}
	var interactionID string
	if interaction, ok := asMap(payload["interaction"]); ok {
		interactionID = toID(interaction["id"])
	}
	requestID := buildRequestID("dc", toID(payload["id"]), interactionID)
	out := &NormalizedMessage{
		Provider:  "discord",
		ChatID:    chatID,
		RequestID: requestID,
		RawText:   content,
	}
	if parsed := parseCommandText(content); parsed != nil {
		out.Command = &NormalizedCommand{
			Provider:  out.Provider,
			ChatID:    out.ChatID,
			RequestID: out.RequestID,
			Command:   parsed.command,
			Args:      parsed.args,
			RawText:   content,
		}
	}
	return out
}

// ParseDiscordPayload parses a Discord webhook payload into a normalized command.
func ParseDiscordPayload(payload map[string]interface{}) *NormalizedCommand {
	msg := ParseDiscordMessage(payload)
	if msg == nil {
		return nil
	}
	return msg.Command
}

// RenderDiscordResponse renders a GatewayResponse for Discord.
func RenderDiscordResponse(resp GatewayResponse) map[string]interface{} {
	var lines []string
	if resp.Result == "ok" {
		lines = append(lines, "✅ "+resp.Message)
		if resp.DownloadURL != "" {
			lines = append(lines, "Download: "+resp.DownloadURL)
		}
		if resp.HandoffID != "" {
			status := resp.HandoffStatus
			if status == "" {
				status = "pending"
			}
			lines = append(lines, fmt.Sprintf("Handoff: %s (%s)", resp.HandoffID, status))
		}
	} else {
		code := resp.ErrorCode
		if code == "" {
			code = "E_UNKNOWN"
		}
		lines = append(lines, fmt.Sprintf("❌ %s: %s", code, resp.Message))
	}
	return map[string]interface{}{
		"content": strings.Join(lines, "\n"),
	}
}

// --- Feishu ---

// VerifyFeishuToken checks the Feishu event token. Empty expected means accept all.
func VerifyFeishuToken(payload map[string]interface{}, expected string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return true
	}
	header, _ := asMap(payload["header"])
	token := firstString(header["token"], payload["token"])
	if token == "" {
		return false
	}
	return constantTimeStringEqual(strings.TrimSpace(token), expected)
}

// ExtractFeishuChallenge returns the URL-verification challenge, or "" if not present.
func ExtractFeishuChallenge(payload map[string]interface{}) (string, bool) {
	t, _ := payload["type"].(string)
	if t != "url_verification" {
		return "", false
	}
	challenge, _ := payload["challenge"].(string)
	return challenge, true
}

// ParseFeishuMessage parses a Feishu event payload into a normalized message.
func ParseFeishuMessage(payload map[string]interface{}) *NormalizedMessage {
	t, _ := payload["type"].(string)
	if t == "url_verification" {
		return nil
	}

	header, _ := asMap(payload["header"])
	event, _ := asMap(payload["event"])
	message, _ := asMap(event["message"])
	if message == nil {
		return nil
	}

	chatID := firstString(message["chat_id"], message["open_chat_id"])
	if chatID == "" {
		return nil
	}

	rawText := parseFeishuText(message["content"])
	if rawText == "" {
		return nil
	}

	requestID := buildRequestID("fs",
		toID(header["event_id"]),
		toID(message["message_id"]),
		toID(payload["uuid"]),
	)
	out := &NormalizedMessage{
		Provider:  "feishu",
		ChatID:    chatID,
		RequestID: requestID,
		RawText:   rawText,
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

// ParseFeishuEvent parses a Feishu event payload into a normalized command.
func ParseFeishuEvent(payload map[string]interface{}) *NormalizedCommand {
	msg := ParseFeishuMessage(payload)
	if msg == nil {
		return nil
	}
	return msg.Command
}

// RenderFeishuResponse renders a GatewayResponse for Feishu.
func RenderFeishuResponse(resp GatewayResponse) map[string]interface{} {
	var lines []string
	if resp.Result == "ok" {
		lines = append(lines, "✅ "+resp.Message)
		if resp.DownloadURL != "" {
			lines = append(lines, "Download: "+resp.DownloadURL)
		}
		if resp.HandoffID != "" {
			status := resp.HandoffStatus
			if status == "" {
				status = "pending"
			}
			lines = append(lines, fmt.Sprintf("Handoff: %s (%s)", resp.HandoffID, status))
		}
	} else {
		code := resp.ErrorCode
		if code == "" {
			code = "E_UNKNOWN"
		}
		lines = append(lines, fmt.Sprintf("❌ %s: %s", code, resp.Message))
	}
	return map[string]interface{}{
		"msg_type": "text",
		"content": map[string]interface{}{
			"text": strings.Join(lines, "\n"),
		},
	}
}

// --- Internal helpers ---

type parsedCommandText struct {
	command string
	args    []string
}

var leadingMentionRe = regexp.MustCompile(`^(?:(?:<@!?\d+>|@\S+)\s*)+`)

func parseCommandText(raw string) *parsedCommandText {
	normalized := strings.TrimSpace(leadingMentionRe.ReplaceAllString(raw, ""))
	if normalized == "" {
		return nil
	}
	parts := strings.Fields(normalized)
	if len(parts) == 0 {
		return nil
	}
	cmd := normalizeCommandName(parts[0])
	if !strings.HasPrefix(cmd, "/") || cmd == "/" {
		return nil
	}
	return &parsedCommandText{command: cmd, args: parts[1:]}
}

var atSuffixRe = regexp.MustCompile(`@[\w.-]+$`)

func normalizeCommandName(s string) string {
	lower := strings.ToLower(strings.TrimSpace(s))
	return atSuffixRe.ReplaceAllString(lower, "")
}

func flattenDiscordOptions(value interface{}) []string {
	arr, ok := value.([]interface{})
	if !ok {
		return nil
	}
	var out []string
	for _, item := range arr {
		m, ok := asMap(item)
		if !ok {
			continue
		}
		if v, ok := m["value"]; ok {
			out = append(out, fmt.Sprintf("%v", v))
			continue
		}
		out = append(out, flattenDiscordOptions(m["options"])...)
	}
	return out
}

func parseFeishuText(raw interface{}) string {
	if s, ok := raw.(string); ok {
		var decoded map[string]interface{}
		if err := json.Unmarshal([]byte(s), &decoded); err == nil {
			if text, ok := decoded["text"].(string); ok {
				return text
			}
			return ""
		}
		return s
	}
	if m, ok := asMap(raw); ok {
		if text, ok := m["text"].(string); ok {
			return text
		}
	}
	return ""
}

func buildRequestID(prefix string, parts ...string) string {
	var compact []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			compact = append(compact, p)
		}
	}
	if len(compact) == 0 {
		return prefix + "-" + uuid.New().String()
	}
	return prefix + "-" + strings.Join(compact, "-")
}

func firstString(values ...interface{}) string {
	for _, v := range values {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func toID(v interface{}) string {
	switch val := v.(type) {
	case string:
		if strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
	case float64:
		if !math.IsInf(val, 0) && !math.IsNaN(val) {
			return strconv.FormatInt(int64(val), 10)
		}
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	}
	return ""
}

func toFloat(v interface{}) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	if n, ok := v.(int); ok {
		return float64(n)
	}
	return 0
}

func asMap(v interface{}) (map[string]interface{}, bool) {
	if v == nil {
		return nil, false
	}
	m, ok := v.(map[string]interface{})
	return m, ok
}

func constantTimeStringEqual(a, b string) bool {
	ha := sha256.Sum256([]byte(a))
	hb := sha256.Sum256([]byte(b))
	return hmac.Equal(ha[:], hb[:])
}
