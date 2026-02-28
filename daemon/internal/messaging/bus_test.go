package messaging

import (
	"testing"
	"time"
)

func TestMessageBusSendReceive(t *testing.T) {
	bus := NewMessageBus()
	err := bus.Send(Message{ID: "1", From: "a", To: "b", Type: MessageTypeRequest, Payload: "hello"})
	if err != nil {
		t.Fatalf("Send error: %v", err)
	}
	msg, err := bus.Receive("b", time.Second)
	if err != nil {
		t.Fatalf("Receive error: %v", err)
	}
	if msg.Payload != "hello" {
		t.Fatalf("payload = %q, want %q", msg.Payload, "hello")
	}
}

func TestMessageBusTTLExpiry(t *testing.T) {
	bus := NewMessageBus()
	bus.ttl = 50 * time.Millisecond
	if err := bus.Send(Message{ID: "1", To: "b", Payload: "old", Timestamp: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatalf("Send error: %v", err)
	}
	if _, err := bus.Receive("b", 80*time.Millisecond); err == nil {
		t.Fatal("expected timeout because message is expired")
	}
}
