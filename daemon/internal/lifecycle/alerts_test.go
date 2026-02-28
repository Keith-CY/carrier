package lifecycle

import (
	"context"
	"testing"
	"time"
)

type alertSinkRecorder struct {
	alerts []Alert
}

func (r *alertSinkRecorder) Send(_ context.Context, alert Alert) error {
	r.alerts = append(r.alerts, alert)
	return nil
}

func TestAlertManagerFiresOnCrashLoop(t *testing.T) {
	sink := &alertSinkRecorder{}
	mgr := NewAlertManager(true, sink)
	mgr.now = func() time.Time { return time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC) }

	if err := mgr.Fire(context.Background(), AlertConditionCrashLoop, "openclaw", "too many restarts"); err != nil {
		t.Fatalf("Fire error: %v", err)
	}
	if len(sink.alerts) != 1 {
		t.Fatalf("alerts count = %d, want 1", len(sink.alerts))
	}
	if sink.alerts[0].Condition != AlertConditionCrashLoop {
		t.Fatalf("condition = %s, want %s", sink.alerts[0].Condition, AlertConditionCrashLoop)
	}
}

func TestAlertManagerDeduplicates(t *testing.T) {
	sink := &alertSinkRecorder{}
	now := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	mgr := NewAlertManager(true, sink)
	mgr.now = func() time.Time { return now }

	if err := mgr.Fire(context.Background(), AlertConditionCrashLoop, "openclaw", "first"); err != nil {
		t.Fatalf("first fire: %v", err)
	}
	if err := mgr.Fire(context.Background(), AlertConditionCrashLoop, "openclaw", "duplicate"); err != nil {
		t.Fatalf("second fire: %v", err)
	}
	now = now.Add(31 * time.Minute)
	if err := mgr.Fire(context.Background(), AlertConditionCrashLoop, "openclaw", "after window"); err != nil {
		t.Fatalf("third fire: %v", err)
	}

	if len(sink.alerts) != 2 {
		t.Fatalf("alerts count = %d, want 2", len(sink.alerts))
	}
}
