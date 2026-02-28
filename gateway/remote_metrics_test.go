package gateway

import (
	"testing"
	"time"
)

func TestRemoteMetricsCollectorSnapshot(t *testing.T) {
	c := newRemoteMetricsCollector()
	c.recordOperation(remoteOpHostCheck, true, 120*time.Millisecond)
	c.recordOperation(remoteOpRemoteChatStream, false, 80*time.Millisecond)
	c.recordRepairAttempt(true)

	snap := c.snapshot()
	if snap.Totals.Total != 2 {
		t.Fatalf("expected totals.total=2, got %+v", snap.Totals)
	}
	if snap.Totals.Success != 1 || snap.Totals.Failure != 1 {
		t.Fatalf("unexpected totals success/failure: %+v", snap.Totals)
	}
	if snap.ChatStream.Total != 1 || snap.ChatStream.Failure != 1 {
		t.Fatalf("unexpected chat metrics: %+v", snap.ChatStream)
	}
	if snap.ChatStream.FailureRate <= 0 {
		t.Fatalf("expected positive chat failure rate, got %+v", snap.ChatStream)
	}
	if snap.Repair.Triggered != 1 || snap.Repair.Success != 1 || snap.Repair.Failure != 0 {
		t.Fatalf("unexpected repair metrics: %+v", snap.Repair)
	}
	if snap.Rollout.State != "healthy" {
		t.Fatalf("expected rollout state healthy for low-volume sample, got %+v", snap.Rollout)
	}
	if snap.Alerts.Active {
		t.Fatalf("expected no active alerts for healthy rollout, got %+v", snap.Alerts)
	}
}

func TestRemoteMetricsCollectorReset(t *testing.T) {
	c := newRemoteMetricsCollector()
	c.recordOperation(remoteOpInstancesList, true, 10*time.Millisecond)
	c.recordRepairAttempt(false)
	c.reset()

	snap := c.snapshot()
	if snap.Totals.Total != 0 {
		t.Fatalf("expected totals reset to zero, got %+v", snap.Totals)
	}
	if snap.Repair.Triggered != 0 {
		t.Fatalf("expected repair metrics reset, got %+v", snap.Repair)
	}
	if len(snap.Operations) != 0 {
		t.Fatalf("expected empty operations after reset, got %+v", snap.Operations)
	}
}

func TestEvaluateRemoteRolloutStatusCanary(t *testing.T) {
	status := evaluateRemoteRolloutStatus(
		remoteOperationStats{Total: 6, Success: 5, Failure: 1, SuccessRate: 5.0 / 6.0, AvgLatencyMs: 220, MaxLatencyMs: 600},
		remoteRepairMetrics{Triggered: 1, Success: 1, Failure: 0, SuccessRate: 1},
		remoteChatMetrics{Total: 3, Failure: 1, FailureRate: 1.0 / 3.0},
	)
	if status.State != "canary" || status.CanPromote {
		t.Fatalf("expected canary rollout status, got %+v", status)
	}
	if len(status.Reasons) == 0 {
		t.Fatalf("expected non-empty canary reasons, got %+v", status)
	}
}

func TestEvaluateRemoteRolloutStatusHold(t *testing.T) {
	status := evaluateRemoteRolloutStatus(
		remoteOperationStats{Total: 30, Success: 24, Failure: 6, SuccessRate: 0.8, AvgLatencyMs: 1600, MaxLatencyMs: 6200},
		remoteRepairMetrics{Triggered: 6, Success: 3, Failure: 3, SuccessRate: 0.5},
		remoteChatMetrics{Total: 12, Failure: 4, FailureRate: 4.0 / 12.0},
	)
	if status.State != "hold" || status.CanPromote {
		t.Fatalf("expected hold rollout status, got %+v", status)
	}
	if len(status.Reasons) < 2 {
		t.Fatalf("expected multiple hold reasons, got %+v", status)
	}
}
