package baseagent

import (
	"context"
	"sync"
	"time"
)

const (
	defaultInboundQueueSize  = 128
	defaultOutboundQueueSize = 128
	defaultEventQueueSize    = 256
)

// InboundEnvelope represents an inbound message into the base-agent loop.
type InboundEnvelope struct {
	Channel    string
	SenderID   string
	ChatID     string
	Content    string
	SessionKey string
	Metadata   map[string]string
}

// OutboundEnvelope represents an outbound message emitted by the base-agent loop.
type OutboundEnvelope struct {
	Channel  string
	ChatID   string
	Content  string
	Metadata map[string]string
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
