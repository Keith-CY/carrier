package api

import (
	"strings"
	"testing"
	"time"
)

type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.now = c.now.Add(d)
}

func TestPairingCodeStoreIssueGeneratesCode(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 2, 14, 16, 0, 0, 0, time.UTC)}
	store := NewPairingCodeStore(clock.Now)

	record, err := store.Issue(30 * time.Second)
	if err != nil {
		t.Fatalf("Issue() error: %v", err)
	}
	if !strings.HasPrefix(record.Code, "pair-") {
		t.Fatalf("expected pair- prefix, got %q", record.Code)
	}
	if store.Count() != 1 {
		t.Fatalf("expected one active code, got %d", store.Count())
	}
}

func TestPairingCodeStoreVerifyAndConsumeSingleUse(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 2, 14, 16, 0, 0, 0, time.UTC)}
	store := NewPairingCodeStore(clock.Now)

	record, err := store.Register("pair-test", 2*time.Minute)
	if err != nil {
		t.Fatalf("Register() error: %v", err)
	}
	if record.Code != "pair-test" {
		t.Fatalf("unexpected code %q", record.Code)
	}

	if err := store.VerifyAndConsume("pair-test"); err != nil {
		t.Fatalf("VerifyAndConsume() first call error: %v", err)
	}
	if err := store.VerifyAndConsume("pair-test"); err == nil {
		t.Fatal("expected second consume to fail")
	}
}

func TestPairingCodeStoreRejectsExpiredCode(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 2, 14, 16, 0, 0, 0, time.UTC)}
	store := NewPairingCodeStore(clock.Now)

	if _, err := store.Register("pair-expired", 5*time.Second); err != nil {
		t.Fatalf("Register() error: %v", err)
	}
	clock.Advance(6 * time.Second)

	err := store.VerifyAndConsume("pair-expired")
	if err == nil {
		t.Fatal("expected expired code to fail")
	}
	if err != ErrPairCodeInvalid {
		t.Fatalf("expected ErrPairCodeInvalid, got %v", err)
	}
}

func TestPairingCodeStoreCleanupExpired(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 2, 14, 16, 0, 0, 0, time.UTC)}
	store := NewPairingCodeStore(clock.Now)

	if _, err := store.Register("pair-a", 1*time.Second); err != nil {
		t.Fatalf("Register(pair-a): %v", err)
	}
	if _, err := store.Register("pair-b", 10*time.Second); err != nil {
		t.Fatalf("Register(pair-b): %v", err)
	}

	clock.Advance(2 * time.Second)
	removed := store.CleanupExpired()
	if removed != 1 {
		t.Fatalf("expected one code removed, got %d", removed)
	}
	if store.Count() != 1 {
		t.Fatalf("expected one remaining code, got %d", store.Count())
	}
}
