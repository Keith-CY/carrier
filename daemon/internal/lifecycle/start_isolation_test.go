package lifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
	origEnv := isolationEnvLookup
	t.Cleanup(func() {
		isolationRuntimeGOOS = origGOOS
		isolationBackendLookup = origLookup
		isolationEnvLookup = origEnv
	})

	isolationRuntimeGOOS = "freebsd"
	isolationBackendLookup = func(_ string) (string, error) {
		t.Fatalf("isolation backend lookup should not be called on unsupported OS")
		return "", nil
	}
	isolationEnvLookup = func(string) string { return "" }

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

func TestStartWithIsolationFailsWhenLinuxBackendMissing(t *testing.T) {
	origGOOS := isolationRuntimeGOOS
	origLookup := isolationBackendLookup
	origEnv := isolationEnvLookup
	t.Cleanup(func() {
		isolationRuntimeGOOS = origGOOS
		isolationBackendLookup = origLookup
		isolationEnvLookup = origEnv
	})

	isolationRuntimeGOOS = "linux"
	isolationBackendLookup = func(_ string) (string, error) {
		return "", errors.New("not found")
	}
	isolationEnvLookup = func(string) string { return "" }

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

func TestStartWithIsolationFailsWhenDarwinBackendMissing(t *testing.T) {
	origGOOS := isolationRuntimeGOOS
	origLookup := isolationBackendLookup
	origEnv := isolationEnvLookup
	origCandidates := isolationLimaPathCandidates
	origPathStat := isolationPathStat
	t.Cleanup(func() {
		isolationRuntimeGOOS = origGOOS
		isolationBackendLookup = origLookup
		isolationEnvLookup = origEnv
		isolationLimaPathCandidates = origCandidates
		isolationPathStat = origPathStat
	})

	isolationRuntimeGOOS = "darwin"
	isolationBackendLookup = func(_ string) (string, error) {
		return "", errors.New("not found")
	}
	isolationEnvLookup = func(string) string { return "" }
	isolationLimaPathCandidates = []string{"/tmp/definitely-missing-limactl"}
	isolationPathStat = func(string) (os.FileInfo, error) {
		return nil, errors.New("not found")
	}

	pm := &fakeProcessManager{isRunning: make(map[string]bool), pids: make(map[string]int), shouldStartSucceed: true, nextPID: 100}
	svc := newIsolationTestService(t, pm)
	svc.mu.Lock()
	state := svc.states["openclaw"]
	state.LimaInstanceName = "carrier-openclaw-a3f2"
	svc.states["openclaw"] = state
	svc.mu.Unlock()

	err := svc.StartWithOptions(context.Background(), "openclaw", StartOptions{Isolation: true})
	if err == nil {
		t.Fatal("expected isolation start to fail when limactl is unavailable")
	}
	if !errors.Is(err, ErrIsolationUnavailable) {
		t.Fatalf("expected ErrIsolationUnavailable, got %v", err)
	}
}

func TestStartWithIsolationFailsWhenWindowsBackendMissing(t *testing.T) {
	origGOOS := isolationRuntimeGOOS
	origLookup := isolationBackendLookup
	origEnv := isolationEnvLookup
	t.Cleanup(func() {
		isolationRuntimeGOOS = origGOOS
		isolationBackendLookup = origLookup
		isolationEnvLookup = origEnv
	})

	isolationRuntimeGOOS = "windows"
	isolationBackendLookup = func(_ string) (string, error) {
		return "", errors.New("not found")
	}
	isolationEnvLookup = func(string) string { return "" }

	pm := &fakeProcessManager{isRunning: make(map[string]bool), pids: make(map[string]int), shouldStartSucceed: true, nextPID: 100}
	svc := newIsolationTestService(t, pm)

	err := svc.StartWithOptions(context.Background(), "openclaw", StartOptions{Isolation: true})
	if err == nil {
		t.Fatal("expected isolation start to fail when wsl is unavailable")
	}
	if !errors.Is(err, ErrIsolationUnavailable) {
		t.Fatalf("expected ErrIsolationUnavailable, got %v", err)
	}
}

func TestStartWithIsolationWrapsStartCommandWithBwrap(t *testing.T) {
	origGOOS := isolationRuntimeGOOS
	origLookup := isolationBackendLookup
	origEnv := isolationEnvLookup
	t.Cleanup(func() {
		isolationRuntimeGOOS = origGOOS
		isolationBackendLookup = origLookup
		isolationEnvLookup = origEnv
	})

	isolationRuntimeGOOS = "linux"
	isolationBackendLookup = func(name string) (string, error) {
		if name != "bwrap" {
			t.Fatalf("lookup name = %q, want bwrap", name)
		}
		return "/usr/bin/bwrap", nil
	}
	isolationEnvLookup = func(string) string { return "" }

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

func TestStartWithIsolationWrapsStartCommandWithLimaBwrap(t *testing.T) {
	origGOOS := isolationRuntimeGOOS
	origLookup := isolationBackendLookup
	origEnv := isolationEnvLookup
	t.Cleanup(func() {
		isolationRuntimeGOOS = origGOOS
		isolationBackendLookup = origLookup
		isolationEnvLookup = origEnv
	})

	isolationRuntimeGOOS = "darwin"
	isolationBackendLookup = func(name string) (string, error) {
		if name != "limactl" {
			t.Fatalf("lookup name = %q, want limactl", name)
		}
		return "/opt/homebrew/bin/limactl", nil
	}
	isolationEnvLookup = func(string) string { return "" }

	pm := &fakeProcessManager{isRunning: make(map[string]bool), pids: make(map[string]int), shouldStartSucceed: true, nextPID: 220}
	svc := newIsolationTestService(t, pm)
	svc.mu.Lock()
	state := svc.states["openclaw"]
	state.LimaInstanceName = "carrier-dev-a3f2"
	svc.states["openclaw"] = state
	svc.mu.Unlock()

	if err := svc.StartWithOptions(context.Background(), "openclaw", StartOptions{Isolation: true}); err != nil {
		t.Fatalf("StartWithOptions: %v", err)
	}
	if len(pm.lastArgs) < 2 || pm.lastArgs[0] != "-c" {
		t.Fatalf("unexpected process args: %#v", pm.lastArgs)
	}
	wrapped := pm.lastArgs[1]
	for _, want := range []string{
		"/opt/homebrew/bin/limactl",
		"shell 'carrier-dev-a3f2'",
		"bwrap",
		"--tmpfs /tmp",
		"tail -f /dev/null",
	} {
		if !strings.Contains(wrapped, want) {
			t.Fatalf("expected wrapped command to contain %q, got %q", want, wrapped)
		}
	}
	if err := svc.Stop(context.Background(), "openclaw"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestStartWithIsolationWrapsStartCommandWithWSLBwrap(t *testing.T) {
	origGOOS := isolationRuntimeGOOS
	origLookup := isolationBackendLookup
	origEnv := isolationEnvLookup
	t.Cleanup(func() {
		isolationRuntimeGOOS = origGOOS
		isolationBackendLookup = origLookup
		isolationEnvLookup = origEnv
	})

	isolationRuntimeGOOS = "windows"
	isolationBackendLookup = func(name string) (string, error) {
		if name != "wsl" {
			t.Fatalf("lookup name = %q, want wsl", name)
		}
		return "/usr/bin/wsl", nil
	}
	isolationEnvLookup = func(key string) string {
		if key == defaultWSLDistroEnvKey {
			return "Ubuntu-22.04"
		}
		return ""
	}

	pm := &fakeProcessManager{isRunning: make(map[string]bool), pids: make(map[string]int), shouldStartSucceed: true, nextPID: 240}
	svc := newIsolationTestService(t, pm)

	if err := svc.StartWithOptions(context.Background(), "openclaw", StartOptions{Isolation: true}); err != nil {
		t.Fatalf("StartWithOptions: %v", err)
	}
	if len(pm.lastArgs) < 2 || pm.lastArgs[0] != "-c" {
		t.Fatalf("unexpected process args: %#v", pm.lastArgs)
	}
	wrapped := pm.lastArgs[1]
	for _, want := range []string{
		"/usr/bin/wsl",
		"-d 'Ubuntu-22.04'",
		"bwrap",
		"--tmpfs /tmp",
		"tail -f /dev/null",
	} {
		if !strings.Contains(wrapped, want) {
			t.Fatalf("expected wrapped command to contain %q, got %q", want, wrapped)
		}
	}
	if err := svc.Stop(context.Background(), "openclaw"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestStartWithIsolationWrapsProcessStartFailure(t *testing.T) {
	origGOOS := isolationRuntimeGOOS
	origLookup := isolationBackendLookup
	origEnv := isolationEnvLookup
	t.Cleanup(func() {
		isolationRuntimeGOOS = origGOOS
		isolationBackendLookup = origLookup
		isolationEnvLookup = origEnv
	})

	isolationRuntimeGOOS = "linux"
	isolationBackendLookup = func(_ string) (string, error) {
		return "/usr/bin/bwrap", nil
	}
	isolationEnvLookup = func(string) string { return "" }

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

func TestInstallWithIsolationFailsFastWhenBackendUnavailable(t *testing.T) {
	origGOOS := isolationRuntimeGOOS
	origLookup := isolationBackendLookup
	origEnv := isolationEnvLookup
	origCandidates := isolationLimaPathCandidates
	origPathStat := isolationPathStat
	origHome := isolationUserHomeDir
	t.Cleanup(func() {
		isolationRuntimeGOOS = origGOOS
		isolationBackendLookup = origLookup
		isolationEnvLookup = origEnv
		isolationLimaPathCandidates = origCandidates
		isolationPathStat = origPathStat
		isolationUserHomeDir = origHome
	})

	isolationRuntimeGOOS = "darwin"
	isolationBackendLookup = func(_ string) (string, error) {
		return "", errors.New("not found")
	}
	isolationEnvLookup = func(string) string { return "" }
	isolationLimaPathCandidates = []string{"/tmp/definitely-missing-limactl"}
	isolationPathStat = func(string) (os.FileInfo, error) {
		return nil, errors.New("not found")
	}
	home := t.TempDir()
	isolationUserHomeDir = func() (string, error) { return home, nil }
	t.Setenv(instanceStoreEnvKey, filepath.Join(home, "instances.json"))

	t.Setenv("OPENAI_API_KEY", "test-key")
	runner := &fakeRunner{}
	checker := &fakeChecker{}
	clock := &fakeClock{current: time.Date(2026, 2, 14, 4, 20, 0, 0, time.UTC)}
	svc := NewService(nil,
		WithRunner(runner),
		WithRuntimeChecker(checker),
		WithNow(clock.Now),
		WithDiagnoseDir(t.TempDir()),
	)
	if err := svc.RegisterManifest(sampleManifest()); err != nil {
		t.Fatalf("RegisterManifest: %v", err)
	}

	err := svc.InstallWithOptions(context.Background(), "openclaw", InstallOptions{Isolation: true})
	if err == nil {
		t.Fatal("expected install isolation preflight to fail")
	}
	if !errors.Is(err, ErrIsolationUnavailable) {
		t.Fatalf("expected ErrIsolationUnavailable, got %v", err)
	}
	joined := strings.Join(runner.calls, "\n")
	if strings.Contains(joined, "install-openclaw") {
		t.Fatalf("expected install command not to run when backend is unavailable, got calls=%v", runner.calls)
	}
	if !strings.Contains(joined, "limactl") {
		t.Fatalf("expected host isolation preflight command to run before backend resolution, got calls=%v", runner.calls)
	}
}
