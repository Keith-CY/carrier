package ratelimit

import (
	"testing"
	"time"
)

func TestAllowWithinLimit(t *testing.T) {
	l := New(WithMax(3), WithWindow(1*time.Minute))
	for i := 0; i < 3; i++ {
		if !l.Allow("ip1") {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}
}

func TestDenyAfterLimit(t *testing.T) {
	l := New(WithMax(3), WithWindow(1*time.Minute))
	for i := 0; i < 3; i++ {
		l.Allow("ip1")
	}
	if l.Allow("ip1") {
		t.Fatal("4th attempt should be denied")
	}
}

func TestWindowSlides(t *testing.T) {
	now := time.Now()
	current := now
	l := New(WithMax(2), WithWindow(1*time.Minute), WithNow(func() time.Time { return current }))

	l.Allow("ip1")
	l.Allow("ip1")
	if l.Allow("ip1") {
		t.Fatal("should be denied")
	}

	// Advance past window
	current = now.Add(61 * time.Second)
	if !l.Allow("ip1") {
		t.Fatal("should be allowed after window slides")
	}
}

func TestDifferentKeysIndependent(t *testing.T) {
	l := New(WithMax(1), WithWindow(1*time.Minute))
	if !l.Allow("ip1") {
		t.Fatal("ip1 should be allowed")
	}
	if l.Allow("ip1") {
		t.Fatal("ip1 should be denied")
	}
	if !l.Allow("ip2") {
		t.Fatal("ip2 should be allowed independently")
	}
}

func TestReset(t *testing.T) {
	l := New(WithMax(1), WithWindow(1*time.Minute))
	l.Allow("ip1")
	if l.Allow("ip1") {
		t.Fatal("should be denied")
	}
	l.Reset("ip1")
	if !l.Allow("ip1") {
		t.Fatal("should be allowed after reset")
	}
}
