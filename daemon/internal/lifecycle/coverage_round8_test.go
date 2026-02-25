package lifecycle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"carrier/baseagent"
	"carrier/daemon/internal/commandexec"
	"carrier/daemon/internal/manifest"
)

type sequenceChecker struct {
	results []error
	calls   int
}

func (s *sequenceChecker) Check(manifest.Manifest) error {
	if len(s.results) == 0 {
		s.calls++
		return nil
	}
	idx := s.calls
	s.calls++
	if idx >= len(s.results) {
		return s.results[len(s.results)-1]
	}
	return s.results[idx]
}

func TestRepairRuntimePrerequisitesLoopRecovers(t *testing.T) {
	runner := &fakeRunner{
		results: map[string]runResult{
			"cd '/tmp/openclaw-workspace' && pnpm install": {result: commandexecResultWithExitCode(0), err: nil},
		},
	}
	svc := newServiceForTest(t, runner, &fakeChecker{})
	svc.checker = &sequenceChecker{results: []error{nil}}
	svc.triager = &fakeTriager{
		onAnalyze: func(context.Context, baseagent.Evidence) (baseagent.TriageResult, error) {
			return baseagent.TriageResult{
				Summary: "repair runtime deps",
				RepairAction: &baseagent.RepairAction{
					Command:    "pnpm install",
					TargetPath: "/tmp/openclaw-workspace",
				},
			}, nil
		},
	}

	err := svc.repairRuntimePrerequisitesLoop(
		context.Background(),
		context.Background(),
		"openclaw",
		sampleManifest(),
		errors.New("runtime prerequisite missing"),
	)
	if err != nil {
		t.Fatalf("expected recovery, got %v", err)
	}
	if len(runner.calls) != 1 || runner.calls[0] != "cd '/tmp/openclaw-workspace' && pnpm install" {
		t.Fatalf("unexpected repair runner calls: %#v", runner.calls)
	}
}

func TestRepairRuntimePrerequisitesLoopExhausted(t *testing.T) {
	runner := &fakeRunner{
		results: map[string]runResult{
			"cd '/tmp/openclaw-workspace' && pnpm install": {result: commandexecResultWithExitCode(0), err: nil},
		},
	}
	svc := newServiceForTest(t, runner, &fakeChecker{})
	checker := &sequenceChecker{results: []error{
		errors.New("still missing"),
		errors.New("still missing"),
		errors.New("still missing"),
	}}
	svc.checker = checker
	svc.triager = &fakeTriager{
		onAnalyze: func(context.Context, baseagent.Evidence) (baseagent.TriageResult, error) {
			return baseagent.TriageResult{
				Summary: "repair runtime deps",
				RepairAction: &baseagent.RepairAction{
					Command:    "pnpm install",
					TargetPath: "/tmp/openclaw-workspace",
				},
			}, nil
		},
	}

	err := svc.repairRuntimePrerequisitesLoop(
		context.Background(),
		context.Background(),
		"openclaw",
		sampleManifest(),
		errors.New("runtime prerequisite missing"),
	)
	if err == nil {
		t.Fatal("expected exhausted auto-repair error")
	}
	if !strings.Contains(err.Error(), "unresolved after 3 auto-repair rounds") {
		t.Fatalf("unexpected error: %v", err)
	}
	if checker.calls != 3 {
		t.Fatalf("checker calls = %d, want 3", checker.calls)
	}
	if len(runner.calls) != 3 {
		t.Fatalf("runner calls = %d, want 3", len(runner.calls))
	}
}

func TestFinalizeInstallFailureReturnsOriginalWhenDiagnoseFails(t *testing.T) {
	svc := newServiceForTest(t, &fakeRunner{}, &fakeChecker{})
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	svc.diagnoseDir = blocker

	installErr := errors.New("install command failed")
	gotErr := svc.finalizeInstallFailure("openclaw", installErr, "E_INSTALL_FAILED")
	if !errors.Is(gotErr, installErr) {
		t.Fatalf("expected original install error, got %v", gotErr)
	}
	status, err := svc.Status("openclaw")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Install != InstallStateBroken {
		t.Fatalf("install state = %s, want broken", status.Install)
	}
}

func TestShellSingleQuoteAdditionalBranch(t *testing.T) {
	got := shellSingleQuote("a'b")
	if got != `'a'"'"'b'` {
		t.Fatalf("shellSingleQuote escape mismatch: %q", got)
	}
}

func TestRecordRestartSetsCooldownAtThreshold(t *testing.T) {
	clock := &fakeClock{current: time.Date(2026, 2, 25, 9, 0, 0, 0, time.UTC)}
	svc := NewService(
		nil,
		WithNow(clock.Now),
		WithCrashLoopConfig(2, 5*time.Minute, 10*time.Minute),
	)
	if err := svc.RegisterManifest(sampleManifest()); err != nil {
		t.Fatalf("register manifest: %v", err)
	}
	svc.restarts["openclaw"] = []time.Time{clock.Now().Add(-time.Minute)}

	svc.recordRestart("openclaw")
	if len(svc.restarts["openclaw"]) != 2 {
		t.Fatalf("restart history length = %d, want 2", len(svc.restarts["openclaw"]))
	}
	if until, ok := svc.cooldowns["openclaw"]; !ok || until.IsZero() {
		t.Fatal("expected cooldown to be set")
	}
}

func TestAuditPersistenceAndRotation(t *testing.T) {
	auditDir := t.TempDir()
	svc := NewService(
		nil,
		WithAuditLogDir(auditDir),
		WithNow(func() time.Time { return time.Date(2026, 2, 25, 9, 0, 0, 0, time.UTC) }),
	)

	svc.recordAudit("req-1", "tester", "install", "openclaw", AuditResultSuccess, "", "ok")

	auditFile := filepath.Join(auditDir, "audit.jsonl")
	data, err := os.ReadFile(auditFile)
	if err != nil {
		t.Fatalf("read audit file: %v", err)
	}
	var row map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(data), &row); err != nil {
		t.Fatalf("decode audit jsonl row: %v", err)
	}
	if row["request_id"] != "req-1" {
		t.Fatalf("unexpected request_id: %v", row["request_id"])
	}

	if err := os.Truncate(auditFile, maxAuditLogBytes+1); err != nil {
		t.Fatalf("truncate audit file: %v", err)
	}
	svc.rotateAuditLogIfNeeded(auditFile)
	if _, err := os.Stat(auditFile + ".1"); err != nil {
		t.Fatalf("expected rotated audit file: %v", err)
	}
}

func commandexecResultWithExitCode(exit int) commandexec.Result {
	return commandexec.Result{ExitCode: exit, CombinedOutput: "ok"}
}
