package gateway

import (
	"testing"
	"time"
)

func TestRemoteAlertWatchdogShouldSendStateChanges(t *testing.T) {
	watchdog := &remoteAlertWatchdog{
		cooldown: 5 * time.Minute,
	}
	base := time.Unix(1700000000, 0)

	healthy := remoteAlertDigest{Active: false, Level: "none", Count: 0}
	send, _, _, _ := watchdog.shouldSend(healthy, base)
	if send {
		t.Fatal("expected no send for initial healthy state")
	}

	active := remoteAlertDigest{Active: true, Level: "canary", Count: 1, Reasons: "operation success rate below 98%"}
	send, reason, prev, hasPrev := watchdog.shouldSend(active, base.Add(30*time.Second))
	if !send {
		t.Fatal("expected send on healthy->active transition")
	}
	if reason != "state-change" {
		t.Fatalf("reason=%q, want state-change", reason)
	}
	if !hasPrev || prev.Active {
		t.Fatalf("expected previous healthy state, got %+v hasPrev=%v", prev, hasPrev)
	}

	watchdog.markSent(active, base.Add(30*time.Second))

	send, _, _, _ = watchdog.shouldSend(active, base.Add(2*time.Minute))
	if send {
		t.Fatal("expected no send while alert state unchanged and cooldown not reached")
	}

	resolved := remoteAlertDigest{Active: false, Level: "none", Count: 0}
	send, reason, prev, hasPrev = watchdog.shouldSend(resolved, base.Add(3*time.Minute))
	if !send {
		t.Fatal("expected send on active->resolved transition")
	}
	if reason != "state-change" {
		t.Fatalf("reason=%q, want state-change", reason)
	}
	if !hasPrev || !prev.Active {
		t.Fatalf("expected previous active state, got %+v hasPrev=%v", prev, hasPrev)
	}
}

func TestRemoteAlertWatchdogShouldSendCooldownHeartbeat(t *testing.T) {
	watchdog := &remoteAlertWatchdog{
		cooldown: 2 * time.Minute,
	}
	base := time.Unix(1700000000, 0)

	active := remoteAlertDigest{Active: true, Level: "hold", Count: 2, Reasons: "chat stream failure rate at or above 25%"}
	send, reason, _, _ := watchdog.shouldSend(active, base)
	if !send || reason != "initial-active" {
		t.Fatalf("expected initial active send, got send=%v reason=%q", send, reason)
	}
	watchdog.markSent(active, base)

	send, _, _, _ = watchdog.shouldSend(active, base.Add(90*time.Second))
	if send {
		t.Fatal("expected no send before cooldown")
	}

	send, reason, _, _ = watchdog.shouldSend(active, base.Add(3*time.Minute))
	if !send {
		t.Fatal("expected cooldown resend for active alert")
	}
	if reason != "cooldown" {
		t.Fatalf("reason=%q, want cooldown", reason)
	}
}

func TestDigestRemoteAlertSnapshot(t *testing.T) {
	snapshot := remoteMetricsSnapshot{
		Alerts: remoteAlertSummary{
			Active: true,
			Level:  "hold",
			Count:  2,
		},
		Rollout: remoteRolloutStatus{
			State:   "hold",
			Reasons: []string{"a", "b"},
		},
	}
	digest := digestRemoteAlertSnapshot(snapshot)
	if !digest.Active || digest.Level != "hold" || digest.Count != 2 {
		t.Fatalf("unexpected digest: %+v", digest)
	}
	if digest.Reasons != "a\nb" {
		t.Fatalf("unexpected digest reasons: %q", digest.Reasons)
	}
}
