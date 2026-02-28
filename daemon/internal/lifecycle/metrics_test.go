package lifecycle

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCollectAgentMetricsReturnsValidData(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("collectAgentMetrics currently only supports Linux")
	}
	metrics, err := collectAgentMetrics(os.Getpid())
	if err != nil {
		t.Fatalf("collectAgentMetrics error: %v", err)
	}
	if metrics.CPUPercent < 0 {
		t.Fatalf("CPUPercent = %f, want >= 0", metrics.CPUPercent)
	}
	if metrics.MemoryRSS <= 0 {
		t.Fatalf("MemoryRSS = %d, want > 0", metrics.MemoryRSS)
	}
	if metrics.Uptime <= 0 {
		t.Fatalf("Uptime = %d, want > 0", metrics.Uptime)
	}
}

func TestCollectAgentMetricsInputValidationAndHostBehavior(t *testing.T) {
	if _, err := collectAgentMetrics(0); err == nil {
		t.Fatal("expected invalid pid error")
	}

	_, err := collectAgentMetrics(os.Getpid())
	if runtime.GOOS == "linux" {
		if err != nil {
			t.Fatalf("collectAgentMetrics(linux) error: %v", err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), "only supports Linux") {
		t.Fatalf("expected non-linux support error, got %v", err)
	}
}

func TestParseProcStatFields(t *testing.T) {
	raw := "1234 (agent worker) S 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 24"
	fields := parseProcStatFields(raw)
	if len(fields) < 24 {
		t.Fatalf("unexpected fields length: %d", len(fields))
	}
	if fields[0] != "1234" {
		t.Fatalf("pid field = %q, want 1234", fields[0])
	}
	if fields[1] != "worker)" {
		t.Fatalf("comm field = %q, want %q", fields[1], "worker)")
	}
}

func TestServiceMetricsBranches(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")

	runner := &fakeRunner{}
	checker := &fakeChecker{}
	clock := &fakeClock{current: time.Date(2026, 2, 14, 4, 20, 0, 0, time.UTC)}
	svc := NewService(nil,
		WithRunner(runner),
		WithRuntimeChecker(checker),
		WithNow(clock.Now),
		WithProcessManager(&fakeProcessManager{
			isRunning:          map[string]bool{},
			pids:               map[string]int{},
			waitChs:            map[string]chan struct{}{},
			nextPID:            100,
			shouldStartSucceed: true,
		}),
	)
	if err := svc.RegisterManifest(sampleManifest()); err != nil {
		t.Fatalf("RegisterManifest: %v", err)
	}

	if _, err := svc.Metrics("missing-agent"); err == nil {
		t.Fatal("expected metrics error for missing agent")
	}

	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	state, ok := svc.states["openclaw"]
	if !ok {
		t.Fatal("expected installed state")
	}
	state.LastError = "runtime failed"
	state.UpdatedAt = clock.Now()
	svc.states["openclaw"] = state

	metrics, err := svc.Metrics("openclaw")
	if err != nil {
		t.Fatalf("Metrics(openclaw): %v", err)
	}
	if metrics.RestartCount != state.RestartCount {
		t.Fatalf("RestartCount = %d, want %d", metrics.RestartCount, state.RestartCount)
	}
	if metrics.LastErrorAt == nil {
		t.Fatal("expected LastErrorAt to be populated when state.LastError is set")
	}
}

func TestServiceMetricsKeepsLastErrorTimestampNilWithoutError(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")

	runner := &fakeRunner{}
	checker := &fakeChecker{}
	svc := newServiceForTest(t, runner, checker)
	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("Install: %v", err)
	}

	metrics, err := svc.Metrics("openclaw")
	if err != nil {
		t.Fatalf("Metrics(openclaw): %v", err)
	}
	if metrics.LastErrorAt != nil {
		t.Fatalf("expected LastErrorAt to be nil, got %v", metrics.LastErrorAt)
	}
}

func TestServiceMetricsStatusErrorPropagates(t *testing.T) {
	svc := NewService(nil)
	_, err := svc.Metrics("missing-agent")
	if err == nil {
		t.Fatal("expected status error")
	}
	if !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("expected ErrAgentNotFound, got %v", err)
	}
}
