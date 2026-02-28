package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func newIsolationTestService(t *testing.T, pm *fakeProcessManager) *Service {
	t.Helper()
	t.Setenv("OPENAI_API_KEY", "test-key")
	runner := &fakeRunner{}
	checker := &fakeChecker{}
	clock := &fakeClock{current: time.Date(2026, 2, 14, 4, 20, 0, 0, time.UTC)}

	svc := NewService(nil,
		WithRunner(runner),
		WithRuntimeChecker(checker),
		WithNow(clock.Now),
		WithProcessLogDir(t.TempDir()),
		WithProcessManager(pm),
	)
	if err := svc.RegisterManifest(sampleManifest()); err != nil {
		t.Fatalf("RegisterManifest: %v", err)
	}
	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	return svc
}

func TestStartWithIsolationFailsOnUnsupportedHostOS(t *testing.T) {
	origGOOS := isolationRuntimeGOOS
	origLookup := isolationBackendLookup
	t.Cleanup(func() {
		isolationRuntimeGOOS = origGOOS
		isolationBackendLookup = origLookup
	})

	isolationRuntimeGOOS = "darwin"
	isolationBackendLookup = func(_ string) (string, error) {
		t.Fatalf("isolation backend lookup should not be called on unsupported OS")
		return "", nil
	}

	pm := &fakeProcessManager{isRunning: make(map[string]bool), pids: make(map[string]int), shouldStartSucceed: true, nextPID: 100}
	svc := newIsolationTestService(t, pm)

	err := svc.StartWithOptions(context.Background(), "openclaw", StartOptions{Isolation: true})
	if err == nil {
		t.Fatal("expected isolation start to fail on unsupported OS")
	}
	if !errors.Is(err, ErrIsolationUnavailable) {
		t.Fatalf("expected ErrIsolationUnavailable, got %v", err)
	}
}

func TestStartWithIsolationFailsWhenBackendMissing(t *testing.T) {
	origGOOS := isolationRuntimeGOOS
	origLookup := isolationBackendLookup
	t.Cleanup(func() {
		isolationRuntimeGOOS = origGOOS
		isolationBackendLookup = origLookup
	})

	isolationRuntimeGOOS = "linux"
	isolationBackendLookup = func(_ string) (string, error) {
		return "", errors.New("not found")
	}

	pm := &fakeProcessManager{isRunning: make(map[string]bool), pids: make(map[string]int), shouldStartSucceed: true, nextPID: 100}
	svc := newIsolationTestService(t, pm)

	err := svc.StartWithOptions(context.Background(), "openclaw", StartOptions{Isolation: true})
	if err == nil {
		t.Fatal("expected isolation start to fail when bwrap is unavailable")
	}
	if !errors.Is(err, ErrIsolationUnavailable) {
		t.Fatalf("expected ErrIsolationUnavailable, got %v", err)
	}
}

func TestStartWithIsolationWrapsStartCommandWithBwrap(t *testing.T) {
	origGOOS := isolationRuntimeGOOS
	origLookup := isolationBackendLookup
	t.Cleanup(func() {
		isolationRuntimeGOOS = origGOOS
		isolationBackendLookup = origLookup
	})

	isolationRuntimeGOOS = "linux"
	isolationBackendLookup = func(name string) (string, error) {
		if name != "bwrap" {
			t.Fatalf("lookup name = %q, want bwrap", name)
		}
		return "/usr/bin/bwrap", nil
	}

	pm := &fakeProcessManager{isRunning: make(map[string]bool), pids: make(map[string]int), shouldStartSucceed: true, nextPID: 200}
	svc := newIsolationTestService(t, pm)

	if err := svc.StartWithOptions(context.Background(), "openclaw", StartOptions{Isolation: true}); err != nil {
		t.Fatalf("StartWithOptions: %v", err)
	}
	if pm.lastCommand != "sh" {
		t.Fatalf("process command = %q, want sh", pm.lastCommand)
	}
	if len(pm.lastArgs) < 2 || pm.lastArgs[0] != "-c" {
		t.Fatalf("unexpected process args: %#v", pm.lastArgs)
	}
	wrapped := pm.lastArgs[1]
	if !strings.Contains(wrapped, "/usr/bin/bwrap") || !strings.Contains(wrapped, "--tmpfs /tmp") || !strings.Contains(wrapped, "tail -f /dev/null") {
		t.Fatalf("expected wrapped bwrap command, got %q", wrapped)
	}
	if err := svc.Stop(context.Background(), "openclaw"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestStartWithIsolationWrapsProcessStartFailure(t *testing.T) {
	origGOOS := isolationRuntimeGOOS
	origLookup := isolationBackendLookup
	t.Cleanup(func() {
		isolationRuntimeGOOS = origGOOS
		isolationBackendLookup = origLookup
	})

	isolationRuntimeGOOS = "linux"
	isolationBackendLookup = func(_ string) (string, error) {
		return "/usr/bin/bwrap", nil
	}

	pm := &fakeProcessManager{isRunning: make(map[string]bool), pids: make(map[string]int), shouldStartSucceed: false, nextPID: 300}
	svc := newIsolationTestService(t, pm)

	err := svc.StartWithOptions(context.Background(), "openclaw", StartOptions{Isolation: true})
	if err == nil {
		t.Fatal("expected start failure")
	}
	if !errors.Is(err, ErrIsolationStartFailed) {
		t.Fatalf("expected ErrIsolationStartFailed, got %v", err)
	}
}
