package baseagent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Channel is the transport abstraction for sending outbound messages.
type Channel interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Send(ctx context.Context, msg OutboundEnvelope) error
	IsRunning() bool
}

// ChannelSender sends outbound messages for callback-backed channels.
type ChannelSender func(ctx context.Context, msg OutboundEnvelope) error

// CallbackChannel is a concrete channel implementation that forwards messages via callback.
type CallbackChannel struct {
	name   string
	sender ChannelSender

	mu      sync.RWMutex
	running bool
}

func NewCallbackChannel(name string, sender ChannelSender) *CallbackChannel {
	return &CallbackChannel{
		name:   strings.ToLower(strings.TrimSpace(name)),
		sender: sender,
	}
}

func (c *CallbackChannel) Name() string {
	return c.name
}

func (c *CallbackChannel) Start(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.running = true
	return nil
}

func (c *CallbackChannel) Stop(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.running = false
	return nil
}

func (c *CallbackChannel) Send(ctx context.Context, msg OutboundEnvelope) error {
	c.mu.RLock()
	running := c.running
	sender := c.sender
	c.mu.RUnlock()
	if !running {
		return fmt.Errorf("channel %s is not running", c.name)
	}
	if sender == nil {
		return fmt.Errorf("channel %s sender is not configured", c.name)
	}
	return sender(ctx, msg)
}

func (c *CallbackChannel) IsRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.running
}

func NewTelegramChannel(sender ChannelSender) Channel {
	return NewCallbackChannel("telegram", sender)
}

func NewDiscordChannel(sender ChannelSender) Channel {
	return NewCallbackChannel("discord", sender)
}

func NewFeishuChannel(sender ChannelSender) Channel {
	return NewCallbackChannel("feishu", sender)
}

// ChannelManager coordinates channel lifecycle and outbound dispatching.
type ChannelManager struct {
	bus *MessageBus

	mu       sync.RWMutex
	channels map[string]Channel

	dispatchCancel context.CancelFunc
}

func NewChannelManager(bus *MessageBus) *ChannelManager {
	if bus == nil {
		bus = NewMessageBus(0, 0, 0)
	}
	return &ChannelManager{
		bus:      bus,
		channels: map[string]Channel{},
	}
}

func (m *ChannelManager) RegisterChannel(name string, ch Channel) error {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return fmt.Errorf("channel name is required")
	}
	if ch == nil {
		return fmt.Errorf("channel implementation is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channels[name] = ch
	return nil
}

func (m *ChannelManager) UnregisterChannel(name string) {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.channels, name)
}

func (m *ChannelManager) GetChannel(name string) (Channel, bool) {
	name = strings.TrimSpace(strings.ToLower(name))
	m.mu.RLock()
	defer m.mu.RUnlock()
	ch, ok := m.channels[name]
	return ch, ok
}

func (m *ChannelManager) ListChannels() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.channels))
	for name := range m.channels {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (m *ChannelManager) StartAll(ctx context.Context) error {
	m.mu.Lock()
	if m.dispatchCancel == nil {
		dispatchCtx, cancel := context.WithCancel(ctx)
		m.dispatchCancel = cancel
		go m.dispatchOutbound(dispatchCtx)
	}

	channels := make([]Channel, 0, len(m.channels))
	for _, ch := range m.channels {
		channels = append(channels, ch)
	}
	m.mu.Unlock()

	for _, ch := range channels {
		if err := ch.Start(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (m *ChannelManager) StopAll(ctx context.Context) error {
	m.mu.Lock()
	cancel := m.dispatchCancel
	m.dispatchCancel = nil
	channels := make([]Channel, 0, len(m.channels))
	for _, ch := range m.channels {
		channels = append(channels, ch)
	}
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	for _, ch := range channels {
		if err := ch.Stop(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (m *ChannelManager) dispatchOutbound(ctx context.Context) {
	for {
		msg, ok := m.bus.ConsumeOutbound(ctx)
		if !ok {
			return
		}
		channelName := strings.TrimSpace(strings.ToLower(msg.Channel))
		if isInternalChannelName(channelName) {
			continue
		}

		m.mu.RLock()
		ch, exists := m.channels[channelName]
		m.mu.RUnlock()
		if !exists {
			continue
		}
		_ = ch.Send(ctx, msg)
	}
}

func isInternalChannelName(channel string) bool {
	switch strings.ToLower(strings.TrimSpace(channel)) {
	case "", "system", "internal", "cli":
		return true
	default:
		return false
	}
}
