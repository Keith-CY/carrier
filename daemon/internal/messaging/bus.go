package messaging

import (
	"errors"
	"strings"
	"sync"
	"time"
)

type MessageType string

const (
	MessageTypeRequest  MessageType = "request"
	MessageTypeEvent    MessageType = "event"
	MessageTypeResponse MessageType = "response"
)

type Message struct {
	ID        string      `json:"id"`
	From      string      `json:"from"`
	To        string      `json:"to"`
	Type      MessageType `json:"type"`
	Payload   string      `json:"payload"`
	Timestamp time.Time   `json:"timestamp"`
}

type MessageBus struct {
	mu        sync.Mutex
	queues    map[string][]Message
	queueSize int
	ttl       time.Duration
}

func NewMessageBus() *MessageBus {
	return &MessageBus{
		queues:    map[string][]Message{},
		queueSize: 1000,
		ttl:       5 * time.Minute,
	}
}

func (b *MessageBus) Send(msg Message) error {
	if b == nil {
		return errors.New("message bus is nil")
	}
	if strings.TrimSpace(msg.To) == "" {
		return errors.New("message.To is required")
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now().UTC()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pruneLocked(msg.To)
	q := append(b.queues[msg.To], msg)
	if len(q) > b.queueSize {
		q = q[len(q)-b.queueSize:]
	}
	b.queues[msg.To] = q
	return nil
}

func (b *MessageBus) Receive(agentID string, timeout time.Duration) (Message, error) {
	if b == nil {
		return Message{}, errors.New("message bus is nil")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return Message{}, errors.New("agentID is required")
	}
	deadline := time.Now().Add(timeout)
	for {
		b.mu.Lock()
		b.pruneLocked(agentID)
		queue := b.queues[agentID]
		if len(queue) > 0 {
			msg := queue[0]
			b.queues[agentID] = queue[1:]
			b.mu.Unlock()
			return msg, nil
		}
		b.mu.Unlock()
		if timeout <= 0 || time.Now().After(deadline) {
			return Message{}, errors.New("message receive timeout")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (b *MessageBus) pruneLocked(agentID string) {
	queue := b.queues[agentID]
	if len(queue) == 0 {
		return
	}
	cutoff := time.Now().Add(-b.ttl)
	idx := 0
	for idx < len(queue) {
		if queue[idx].Timestamp.After(cutoff) || queue[idx].Timestamp.Equal(cutoff) {
			break
		}
		idx++
	}
	if idx > 0 {
		queue = queue[idx:]
		if len(queue) == 0 {
			delete(b.queues, agentID)
		} else {
			b.queues[agentID] = queue
		}
	}
}
