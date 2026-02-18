package gateway

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionStore_CreateAndGet(t *testing.T) {
	s := NewSessionStore("", 0, nil)
	defer s.Stop()

	// Initially no session
	if got := s.GetSession("telegram", "123"); got != nil {
		t.Fatalf("expected nil session, got %+v", got)
	}

	// Create session
	rec := s.CreateSession("telegram", "123")
	if rec == nil {
		t.Fatal("CreateSession returned nil")
	}
	if rec.Provider != "telegram" || rec.ChatID != "123" {
		t.Errorf("bad record: %+v", rec)
	}
	if !startsWith(rec.SessionToken, "session-") {
		t.Errorf("session token %q should start with 'session-'", rec.SessionToken)
	}

	// Get it back
	got := s.GetSession("telegram", "123")
	if got == nil {
		t.Fatal("GetSession returned nil")
	}
	if got.SessionToken != rec.SessionToken {
		t.Errorf("token mismatch: got %q, want %q", got.SessionToken, rec.SessionToken)
	}
}

func TestSessionStore_CreatePreservesToken(t *testing.T) {
	s := NewSessionStore("", 0, nil)
	defer s.Stop()

	// Create once
	first := s.CreateSession("discord", "ch1")

	// Create again (same provider+chatId) → should reuse token
	second := s.CreateSession("discord", "ch1")
	if first.SessionToken != second.SessionToken {
		t.Errorf("expected same token, got %q vs %q", first.SessionToken, second.SessionToken)
	}
}

func TestSessionStore_Touch(t *testing.T) {
	now := time.Now()
	callCount := 0
	mockNow := func() time.Time {
		callCount++
		return now.Add(time.Duration(callCount) * time.Second)
	}
	s := NewSessionStore("", 0, mockNow)
	defer s.Stop()

	s.CreateSession("telegram", "99")
	before := s.GetSession("telegram", "99")
	lastSeen1 := before.LastSeenAt

	s.Touch("telegram", "99")
	after := s.GetSession("telegram", "99")
	if after.LastSeenAt == lastSeen1 {
		t.Error("Touch did not update LastSeenAt")
	}
}

func TestSessionStore_Cleanup(t *testing.T) {
	start := time.Now()
	tick := 0
	mockNow := func() time.Time {
		t := start.Add(time.Duration(tick) * time.Second)
		return t
	}
	s := NewSessionStore("", 10*time.Second, mockNow)
	defer s.Stop()

	s.CreateSession("telegram", "old")
	tick = 20 // advance past TTL
	s.CreateSession("telegram", "new")
	tick = 21
	removed := s.Cleanup()
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}
	if s.GetSession("telegram", "old") != nil {
		t.Error("old session should be removed")
	}
	if s.GetSession("telegram", "new") == nil {
		t.Error("new session should still exist")
	}
}

func TestSessionStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")

	s1 := NewSessionStore(path, 0, nil)
	rec := s1.CreateSession("feishu", "abc")
	token := rec.SessionToken

	// Force save: stop triggers final save flush
	s1.Stop()

	// Check file was created
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("sessions.json not created: %v", err)
	}

	// Load into a new store
	s2 := NewSessionStore(path, 0, nil)
	defer s2.Stop()
	got := s2.GetSession("feishu", "abc")
	if got == nil {
		t.Fatal("session not loaded from disk")
	}
	if got.SessionToken != token {
		t.Errorf("token mismatch: got %q, want %q", got.SessionToken, token)
	}
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
