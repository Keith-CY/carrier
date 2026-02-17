package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestBoundaryExactlyAtLimit(t *testing.T) {
	l := New(WithMax(3), WithWindow(1*time.Minute))
	// Exactly 3 should be allowed
	for i := 0; i < 3; i++ {
		if !l.Allow("ip") {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}
	// 4th denied
	if l.Allow("ip") {
		t.Fatal("should be denied at limit")
	}
}

func TestExpiryCleanup(t *testing.T) {
	now := time.Now()
	current := now
	l := New(WithMax(2), WithWindow(30*time.Second), WithNow(func() time.Time { return current }))

	l.Allow("ip")
	l.Allow("ip")
	if l.Allow("ip") {
		t.Fatal("should be denied")
	}

	// Advance just past window so old entries expire
	current = now.Add(31 * time.Second)
	if !l.Allow("ip") {
		t.Fatal("should be allowed after expiry")
	}
	if !l.Allow("ip") {
		t.Fatal("should still be allowed (only 2 in new window)")
	}
	if l.Allow("ip") {
		t.Fatal("should be denied again")
	}
}

func TestConcurrentAccess(t *testing.T) {
	l := New(WithMax(100), WithWindow(1*time.Minute))
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			l.Allow(key)
			l.Allow(key)
			l.Record(key)
			l.Reset(key)
			l.Allow(key)
		}("ip" + string(rune('0'+i%10)))
	}
	wg.Wait()
}

func TestRecordCountsWithoutAllowing(t *testing.T) {
	l := New(WithMax(2), WithWindow(1*time.Minute))
	l.Record("ip")
	l.Record("ip")
	// Should be at limit now
	if l.Allow("ip") {
		t.Fatal("should be denied after 2 records")
	}
}

func TestResetNonexistentKey(t *testing.T) {
	l := New(WithMax(1), WithWindow(1*time.Minute))
	// Should not panic
	l.Reset("nonexistent")
}

func TestWindowEdgeBoundary(t *testing.T) {
	now := time.Now()
	current := now
	l := New(WithMax(1), WithWindow(10*time.Second), WithNow(func() time.Time { return current }))

	l.Allow("ip")

	// Exactly at window boundary
	current = now.Add(10 * time.Second)
	if l.Allow("ip") {
		t.Fatal("should be denied at exact boundary")
	}

	// Just past boundary
	current = now.Add(10*time.Second + time.Millisecond)
	if !l.Allow("ip") {
		t.Fatal("should be allowed just past boundary")
	}
}
