package gateway

import (
	"testing"
	"time"
)

func TestGatewayRateLimiter_AllowsUpToLimit(t *testing.T) {
	now := time.Now()
	rl := NewGatewayRateLimiter(3, 100, time.Minute, func() time.Time { return now })

	for i := 0; i < 3; i++ {
		r := rl.Check("session1")
		if !r.Allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// 4th request should be denied
	r := rl.Check("session1")
	if r.Allowed {
		t.Error("4th request should be denied")
	}
	if r.ErrorCode != "E_RATE_LIMITED" {
		t.Errorf("errorCode: %q", r.ErrorCode)
	}
}

func TestGatewayRateLimiter_GlobalLimit(t *testing.T) {
	now := time.Now()
	rl := NewGatewayRateLimiter(100, 2, time.Minute, func() time.Time { return now })

	rl.Check("s1")
	rl.Check("s2")

	// Third request (global limit=2, now exceeded)
	r := rl.Check("s3")
	if r.Allowed {
		t.Error("3rd request should be denied by global limit")
	}
}

func TestGatewayRateLimiter_SlidingWindow(t *testing.T) {
	base := time.Now()
	tick := int64(0)
	mockNow := func() time.Time {
		return base.Add(time.Duration(tick) * time.Second)
	}
	rl := NewGatewayRateLimiter(2, 100, 10*time.Second, mockNow)

	// Fill up
	rl.Check("s1")
	rl.Check("s1")
	r := rl.Check("s1")
	if r.Allowed {
		t.Error("should be denied at limit")
	}

	// Advance past window
	tick = 11
	r2 := rl.Check("s1")
	if !r2.Allowed {
		t.Error("should be allowed after window slides past")
	}
}

func TestGatewayRateLimiter_Reset(t *testing.T) {
	now := time.Now()
	rl := NewGatewayRateLimiter(1, 100, time.Minute, func() time.Time { return now })

	rl.Check("s1")
	r := rl.Check("s1")
	if r.Allowed {
		t.Error("should be denied after limit")
	}

	rl.Reset()
	r2 := rl.Check("s1")
	if !r2.Allowed {
		t.Error("should be allowed after reset")
	}
}

func TestGatewayRateLimiter_Cleanup(t *testing.T) {
	base := time.Now()
	tick := int64(0)
	mockNow := func() time.Time {
		return base.Add(time.Duration(tick) * time.Second)
	}
	rl := NewGatewayRateLimiter(10, 100, 5*time.Second, mockNow)

	rl.Check("s1")
	rl.Check("s2")

	tick = 10
	removed := rl.Cleanup()
	if removed < 2 {
		t.Errorf("expected at least 2 removed, got %d", removed)
	}
}
