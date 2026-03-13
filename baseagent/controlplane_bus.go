package baseagent

import (
	"context"
	"strings"
	"sync"
	"time"
)

const (
	defaultInboundQueueSize  = 128
	defaultOutboundQueueSize = 128
	defaultEventQueueSize    = 256
)

type AttachmentRef struct {
	ID             string            `json:"id,omitempty"`
	Kind           string            `json:"kind,omitempty"`
	OutputRole     string            `json:"outputRole,omitempty"`
	Path           string            `json:"path,omitempty"`
	Name           string            `json:"name,omitempty"`
	MIMEType       string            `json:"mimeType,omitempty"`
	MediaType      string            `json:"mediaType,omitempty"`
	SizeBytes      int64             `json:"sizeBytes,omitempty"`
	Source         string            `json:"source,omitempty"`
	ExternalID     string            `json:"externalId,omitempty"`
	ArtifactID     string            `json:"artifactId,omitempty"`
	DownloadURL    string            `json:"downloadUrl,omitempty"`
	SourceMetadata map[string]string `json:"sourceMetadata,omitempty"`
}

type ContentBlock struct {
	Type         string `json:"type"`
	OutputRole   string `json:"outputRole,omitempty"`
	Text         string `json:"text,omitempty"`
	Name         string `json:"name,omitempty"`
	Path         string `json:"path,omitempty"`
	MIMEType     string `json:"mimeType,omitempty"`
	MediaType    string `json:"mediaType,omitempty"`
	AttachmentID string `json:"attachmentId,omitempty"`
	URL          string `json:"url,omitempty"`
	SizeBytes    int64  `json:"sizeBytes,omitempty"`
}

type RichOutboundMessage struct {
	Text        string          `json:"text,omitempty"`
	RenderMode  string          `json:"renderMode,omitempty"`
	Blocks      []ContentBlock  `json:"blocks,omitempty"`
	Attachments []AttachmentRef `json:"attachments,omitempty"`
}

func (m *RichOutboundMessage) PlainTextFallback() string {
	if m == nil {
		return ""
	}
	lines := []string{}
	if text := strings.TrimSpace(m.Text); text != "" {
		lines = append(lines, text)
	}
	for _, block := range m.Blocks {
		if line := contentBlockPlainText(block); line != "" {
			if len(lines) == 0 || lines[len(lines)-1] != line {
				lines = append(lines, line)
			}
		}
	}
	for _, attachment := range m.Attachments {
		if line := attachmentRefPlainText(attachment); line != "" {
			if len(lines) == 0 || lines[len(lines)-1] != line {
				lines = append(lines, line)
			}
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func contentBlockPlainText(block ContentBlock) string {
	switch strings.ToLower(strings.TrimSpace(block.Type)) {
	case "text":
		return strings.TrimSpace(block.Text)
	case "file":
		if name := strings.TrimSpace(block.Name); name != "" {
			return "Attachment: " + name
		}
		if path := strings.TrimSpace(block.Path); path != "" {
			return "Attachment: " + path
		}
	case "image":
		if name := strings.TrimSpace(block.Name); name != "" {
			return "Image: " + name
		}
	case "audio", "voice":
		if name := strings.TrimSpace(block.Name); name != "" {
			return "Audio: " + name
		}
	case "video":
		if name := strings.TrimSpace(block.Name); name != "" {
			return "Video: " + name
		}
	}
	if url := strings.TrimSpace(block.URL); url != "" {
		return url
	}
	return ""
}

func attachmentRefPlainText(ref AttachmentRef) string {
	switch strings.ToLower(strings.TrimSpace(ref.Kind)) {
	case "document", "file":
		if name := strings.TrimSpace(ref.Name); name != "" {
			return "Attachment: " + name
		}
	case "image":
		if name := strings.TrimSpace(ref.Name); name != "" {
			return "Image: " + name
		}
	case "audio", "voice":
		if name := strings.TrimSpace(ref.Name); name != "" {
			return "Audio: " + name
		}
	case "video":
		if name := strings.TrimSpace(ref.Name); name != "" {
			return "Video: " + name
		}
	}
	if ref.ExternalID != "" {
		return ref.ExternalID
	}
	if ref.Path != "" {
		return ref.Path
	}
	return ""
}

// InboundEnvelope represents an inbound message into the base-agent loop.
type InboundEnvelope struct {
	Channel     string
	SenderID    string
	ChatID      string
	Content     string
	Attachments []AttachmentRef
	SessionKey  string
	Metadata    map[string]string
}

// OutboundEnvelope represents an outbound message emitted by the base-agent loop.
type OutboundEnvelope struct {
	Channel     string
	ChatID      string
	Content     string
	RichContent *RichOutboundMessage
	Metadata    map[string]string
}

// EventType describes the event category emitted by control-plane components.
type EventType string

const (
	EventInbound  EventType = "inbound"
	EventOutbound EventType = "outbound"
	EventTool     EventType = "tool"
	EventError    EventType = "error"
)

// LoopEvent is an observability event emitted by the base-agent control plane.
type LoopEvent struct {
	Type      EventType
	Name      string
	Message   string
	Timestamp time.Time
	Metadata  map[string]string
}

// MessageBus is a lightweight in-memory bus for inbound/outbound traffic and events.
type MessageBus struct {
	inbound  chan InboundEnvelope
	outbound chan OutboundEnvelope
	events   chan LoopEvent

	mu     sync.RWMutex
	closed bool
}

// NewMessageBus creates a new bus with bounded queues.
func NewMessageBus(inboundCap, outboundCap, eventCap int) *MessageBus {
	if inboundCap <= 0 {
		inboundCap = defaultInboundQueueSize
	}
	if outboundCap <= 0 {
		outboundCap = defaultOutboundQueueSize
	}
	if eventCap <= 0 {
		eventCap = defaultEventQueueSize
	}
	return &MessageBus{
		inbound:  make(chan InboundEnvelope, inboundCap),
		outbound: make(chan OutboundEnvelope, outboundCap),
		events:   make(chan LoopEvent, eventCap),
	}
}

// PublishInbound publishes an inbound envelope. When the queue is full, the message is dropped.
func (b *MessageBus) PublishInbound(msg InboundEnvelope) {
	if b == nil {
		return
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return
	}
	select {
	case b.inbound <- msg:
	default:
	}
}

// ConsumeInbound consumes an inbound envelope until context cancellation.
func (b *MessageBus) ConsumeInbound(ctx context.Context) (InboundEnvelope, bool) {
	if b == nil {
		return InboundEnvelope{}, false
	}
	select {
	case msg, ok := <-b.inbound:
		return msg, ok
	case <-ctx.Done():
		return InboundEnvelope{}, false
	}
}

// PublishOutbound publishes an outbound envelope. When the queue is full, the message is dropped.
func (b *MessageBus) PublishOutbound(msg OutboundEnvelope) {
	if b == nil {
		return
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return
	}
	select {
	case b.outbound <- msg:
	default:
	}
}

// ConsumeOutbound consumes an outbound envelope until context cancellation.
func (b *MessageBus) ConsumeOutbound(ctx context.Context) (OutboundEnvelope, bool) {
	if b == nil {
		return OutboundEnvelope{}, false
	}
	select {
	case msg, ok := <-b.outbound:
		return msg, ok
	case <-ctx.Done():
		return OutboundEnvelope{}, false
	}
}

// PublishEvent publishes an observability event.
func (b *MessageBus) PublishEvent(evt LoopEvent) {
	if b == nil {
		return
	}
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now().UTC()
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return
	}
	select {
	case b.events <- evt:
	default:
	}
}

// ConsumeEvent consumes an event until context cancellation.
func (b *MessageBus) ConsumeEvent(ctx context.Context) (LoopEvent, bool) {
	if b == nil {
		return LoopEvent{}, false
	}
	select {
	case evt, ok := <-b.events:
		return evt, ok
	case <-ctx.Done():
		return LoopEvent{}, false
	}
}

// Close closes all queues. Safe to call multiple times.
func (b *MessageBus) Close() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	close(b.inbound)
	close(b.outbound)
	close(b.events)
}
