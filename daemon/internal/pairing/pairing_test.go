package pairing

import (
	"testing"
	"time"
)

func TestNewStore(t *testing.T) {
	s, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	code, err := s.Code()
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != CodeLength*2 {
		t.Fatalf("expected %d chars, got %d", CodeLength*2, len(code))
	}
}

func TestCodeStableBeforeExpiry(t *testing.T) {
	now := time.Now()
	s, err := NewStore(WithTTL(10*time.Minute), WithNow(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	c1, _ := s.Code()
	c2, _ := s.Code()
	if c1 != c2 {
		t.Fatal("code should be stable before expiry")
	}
}

func TestCodeRegeneratesAfterExpiry(t *testing.T) {
	now := time.Now()
	current := now
	s, err := NewStore(WithTTL(1*time.Minute), WithNow(func() time.Time { return current }))
	if err != nil {
		t.Fatal(err)
	}
	c1, _ := s.Code()
	current = now.Add(2 * time.Minute)
	c2, _ := s.Code()
	// Codes should differ (extremely unlikely to collide)
	if c1 == c2 {
		t.Fatal("code should regenerate after expiry")
	}
}
