package gateway

import (
	"encoding/json"
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

func TestSessionStore_StartPeriodicCleanupReturnsSelf(t *testing.T) {
	s := NewSessionStore("", 0, nil)
	defer s.Stop()

	if got := s.StartPeriodicCleanup(); got != s {
		t.Fatalf("StartPeriodicCleanup() should return store itself")
	}
}

func TestParseSessionTime_Branches(t *testing.T) {
	valid := time.Now().UTC().Format(time.RFC3339Nano)
	if got := parseSessionTime(valid); got.IsZero() {
		t.Fatalf("expected parsed valid time, got zero")
	}
	if got := parseSessionTime("not-a-time"); !got.IsZero() {
		t.Fatalf("expected zero time for invalid input, got %v", got)
	}
}

func TestSessionStore_LoadSessionsBranches(t *testing.T) {
	t.Run("parse error keeps store empty", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "sessions.json")
		if err := os.WriteFile(path, []byte("{bad-json"), 0o600); err != nil {
			t.Fatalf("write malformed sessions file: %v", err)
		}
		s := NewSessionStore(path, 0, nil)
		defer s.Stop()
		if got := s.GetSession("telegram", "123"); got != nil {
			t.Fatalf("expected empty store on parse error, got %+v", got)
		}
	})

	t.Run("malformed records are skipped", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "sessions.json")
		payload := []map[string]string{
			{
				"provider":     "telegram",
				"chatId":       "123",
				"sessionToken": "session-abc",
				"createdAt":    "2026-02-24T00:00:00Z",
				"lastSeenAt":   "2026-02-24T00:00:01Z",
			},
			{
				"provider":     "telegram",
				"chatId":       "",
				"sessionToken": "session-bad",
				"createdAt":    "2026-02-24T00:00:00Z",
				"lastSeenAt":   "2026-02-24T00:00:01Z",
			},
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatalf("write sessions payload: %v", err)
		}
		s := NewSessionStore(path, 0, nil)
		defer s.Stop()

		if got := s.GetSession("telegram", "123"); got == nil {
			t.Fatal("expected valid session to load")
		}
		if got := s.GetSession("telegram", ""); got != nil {
			t.Fatalf("expected malformed session to be skipped, got %+v", got)
		}
	})

	t.Run("read error path still allows runtime sessions", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "sessions")
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatalf("mkdir sessions path: %v", err)
		}
		s := NewSessionStore(path, 0, nil) // path is a directory: readFile returns error
		defer s.Stop()
		if got := s.CreateSession("telegram", "555"); got == nil {
			t.Fatal("expected CreateSession to work after load error")
		}
	})
}

func TestSessionStore_PersistSessionsErrorBranch(t *testing.T) {
	root := t.TempDir()
	blocking := filepath.Join(root, "blocking-file")
	if err := os.WriteFile(blocking, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocking file: %v", err)
	}
	storePath := filepath.Join(blocking, "sessions.json")

	s := NewSessionStore(storePath, 0, nil)
	defer s.Stop()
	s.CreateSession("telegram", "900")

	if err := s.persistSessions(); err == nil {
		t.Fatal("expected persistSessions to fail when parent path is a file")
	}
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
