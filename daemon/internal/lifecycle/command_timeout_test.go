package lifecycle

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"carrier/daemon/internal/commandexec"
	"carrier/daemon/internal/manifest"
)

type timeoutAwareRunner struct {
	mu        sync.Mutex
	blockOn   map[string]bool
	startedAt map[string]time.Time
	deadlines map[string]time.Time
}

func newTimeoutAwareRunner() *timeoutAwareRunner {
	return &timeoutAwareRunner{
		blockOn:   map[string]bool{},
		startedAt: map[string]time.Time{},
		deadlines: map[string]time.Time{},
	}
}

func (r *timeoutAwareRunner) Run(ctx context.Context, command string) (commandexec.Result, error) {
	now := time.Now()
	r.mu.Lock()
	r.startedAt[command] = now
	if deadline, ok := ctx.Deadline(); ok {
		r.deadlines[command] = deadline
	}
	shouldBlock := r.blockOn[command]
	r.mu.Unlock()

	if shouldBlock {
		<-ctx.Done()
		return commandexec.Result{ExitCode: -1}, ctx.Err()
	}

	return commandexec.Result{ExitCode: 0}, nil
}

func (r *timeoutAwareRunner) deadlineFor(command string) (time.Time, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	deadline, ok := r.deadlines[command]
	return deadline, ok
}

func (r *timeoutAwareRunner) startFor(command string) (time.Time, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	start, ok := r.startedAt[command]
	return start, ok
}

type alwaysPassChecker struct{}

func (alwaysPassChecker) Check(manifest.Manifest) error { return nil }

func TestInstallUsesConfiguredCommandTimeout(t *testing.T) {
	runner := newTimeoutAwareRunner()
	runner.blockOn["install-openclaw"] = true

	svc := NewService(nil,
		WithRunner(runner),
		WithRuntimeChecker(alwaysPassChecker{}),
		WithCommandTimeout(25*time.Millisecond),
		WithDiagnoseDir(t.TempDir()),
	)
	if err := svc.RegisterManifest(sampleManifest()); err != nil {
		t.Fatalf("register manifest: %v", err)
	}

	err := svc.Install(context.Background(), "openclaw")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("install error = %v, want context deadline exceeded", err)
	}

	startedAt, ok := runner.startFor("install-openclaw")
	if !ok {
		t.Fatal("expected install command to run")
	}
	deadline, ok := runner.deadlineFor("install-openclaw")
	if !ok {
		t.Fatal("expected install command context to have a deadline")
	}
	timeoutWindow := deadline.Sub(startedAt)
	if timeoutWindow <= 0 || timeoutWindow > time.Second {
		t.Fatalf("unexpected install timeout window: %v", timeoutWindow)
	}
}

func TestUpgradeUsesConfiguredCommandTimeout(t *testing.T) {
	runner := newTimeoutAwareRunner()
	runner.blockOn["upgrade-openclaw"] = true

	svc := NewService(nil,
		WithRunner(runner),
		WithRuntimeChecker(alwaysPassChecker{}),
		WithCommandTimeout(25*time.Millisecond),
		WithDiagnoseDir(t.TempDir()),
	)
	if err := svc.RegisterManifest(sampleManifest()); err != nil {
		t.Fatalf("register manifest: %v", err)
	}
	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}

	_, err := svc.Upgrade(context.Background(), "openclaw")
	if err == nil {
		t.Fatal("expected upgrade error")
	}
	if !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
		t.Fatalf("upgrade error = %v, want timeout signal", err)
	}

	startedAt, ok := runner.startFor("upgrade-openclaw")
	if !ok {
		t.Fatal("expected upgrade command to run")
	}
	deadline, ok := runner.deadlineFor("upgrade-openclaw")
	if !ok {
		t.Fatal("expected upgrade command context to have a deadline")
	}
	timeoutWindow := deadline.Sub(startedAt)
	if timeoutWindow <= 0 || timeoutWindow > time.Second {
		t.Fatalf("unexpected upgrade timeout window: %v", timeoutWindow)
	}
}
