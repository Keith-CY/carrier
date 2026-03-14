package gateway

import (
	"carrier/baseagent"
	"context"
	"strings"
)

const (
	InboundEnvelopeKindCommand = "command"
	InboundEnvelopeKindChat    = "chat"
)

// InboundChannelEnvelope is the transport-neutral routing shape for inbound channel events.
type InboundChannelEnvelope struct {
	Channel      string
	ChatID       string
	RequestID    string
	SessionToken string
	Command      string
	Args         []string
	Text         string
	Attachments  []baseagent.AttachmentRef
	Kind         string
	Metadata     map[string]string
}

// NormalizeTelegramInboundEnvelope converts a verified Telegram payload into a shared envelope.
func NormalizeTelegramInboundEnvelope(payload map[string]interface{}, sessions *SessionStore) *InboundChannelEnvelope {
	return normalizeInboundEnvelope(ParseTelegramMessage(payload), sessions, "telegram-webhook")
}

// NormalizeDiscordInboundEnvelope converts a verified Discord payload into a shared envelope.
func NormalizeDiscordInboundEnvelope(payload map[string]interface{}, sessions *SessionStore) *InboundChannelEnvelope {
	return normalizeInboundEnvelope(ParseDiscordMessage(payload), sessions, "discord-webhook")
}

// NormalizeFeishuInboundEnvelope converts a verified Feishu payload into a shared envelope.
func NormalizeFeishuInboundEnvelope(payload map[string]interface{}, sessions *SessionStore) *InboundChannelEnvelope {
	return normalizeInboundEnvelope(ParseFeishuMessage(payload), sessions, "feishu-webhook")
}

// RouteInboundChannel dispatches a transport-neutral envelope into the normal gateway flows.
func RouteInboundChannel(
	ctx context.Context,
	envelope *InboundChannelEnvelope,
	daemon *DaemonClient,
	sessions *SessionStore,
	downloads *DownloadStore,
	rl *GatewayRateLimiter,
	onboard *OnboardStore,
) GatewayResponse {
	if envelope == nil {
		return errResp("unknown", "E_USAGE", "unsupported inbound channel payload")
	}

	switch envelope.Kind {
	case InboundEnvelopeKindCommand:
		input := strings.Join(append([]string{
			envelope.Channel,
			envelope.ChatID,
			envelope.RequestID,
			envelope.Command,
		}, envelope.Args...), " ")
		input = InjectSessionToken(input, envelope.SessionToken)
		return SafeHandleCommand(ctx, input, daemon, sessions, downloads, rl, onboard)
	case InboundEnvelopeKindChat:
		return processBaseAgentChat(ctx, envelope.Channel, envelope.ChatID, envelope.RequestID, envelope.Text, envelope.Attachments, daemon, sessions, rl)
	default:
		return errResp(envelope.RequestID, "E_USAGE", "unsupported inbound channel envelope")
	}
}

func normalizeInboundEnvelope(msg *NormalizedMessage, sessions *SessionStore, transport string) *InboundChannelEnvelope {
	if msg == nil {
		return nil
	}

	envelope := &InboundChannelEnvelope{
		Channel:     msg.Provider,
		ChatID:      msg.ChatID,
		RequestID:   msg.RequestID,
		Attachments: append([]baseagent.AttachmentRef(nil), msg.Attachments...),
		Metadata:    map[string]string{"transport": transport},
	}
	if sessions != nil {
		if session := sessions.GetSession(msg.Provider, msg.ChatID); session != nil {
			envelope.SessionToken = session.SessionToken
		}
	}
	if msg.Command != nil {
		envelope.Kind = InboundEnvelopeKindCommand
		envelope.Command = msg.Command.Command
		envelope.Args = append([]string(nil), msg.Command.Args...)
		envelope.Text = msg.Command.RawText
		return envelope
	}
	envelope.Kind = InboundEnvelopeKindChat
	envelope.Text = msg.RawText
	return envelope
}
