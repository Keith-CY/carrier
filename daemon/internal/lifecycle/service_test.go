package lifecycle

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"carrier/daemon/internal/commandexec"
	"carrier/daemon/internal/manifest"
	"carrier/daemon/internal/redact"
)

type fakeRunner struct {
	calls   []string
	results map[string]runResult
}

type runResult struct {
	result commandexec.Result
	err    error
}

func (f *fakeRunner) Run(_ context.Context, command string) (commandexec.Result, error) {
	f.calls = append(f.calls, command)
	if f.results == nil {
		return commandexec.Result{}, nil
	}
	if res, ok := f.results[command]; ok {
		return res.result, res.err
	}
	return commandexec.Result{}, nil
}

type fakeChecker struct {
	err   error
	calls int
}

func (f *fakeChecker) Check(manifest.Manifest) error {
	f.calls++
	return f.err
}

type fakeClock struct {
	current time.Time
}

func (c *fakeClock) Now() time.Time {
	return c.current
}

func (c *fakeClock) Advance(d time.Duration) {
	c.current = c.current.Add(d)
}

func sampleManifest() manifest.Manifest {
	return manifest.Manifest{
		ID:      "openclaw",
		Name:    "OpenClaw",
		Version: "1.0.0",
		Runtime: manifest.RuntimeSpec{
			Type:    manifest.RuntimeTypeLocalBinary,
			Install: manifest.CommandSpec{Command: "install-openclaw"},
			Upgrade: manifest.CommandSpec{Command: "upgrade-openclaw"},
			Start:   manifest.CommandSpec{Command: "start-openclaw"},
			Stop:    manifest.CommandSpec{Command: "stop-openclaw"},
		},
		Network: manifest.NetworkSpec{
			Ports: []manifest.PortSpec{{Name: "http", Port: 0}},
		},
		Env: manifest.EnvSpec{
			Required: []manifest.EnvVar{{Name: "OPENAI_API_KEY", Secret: true}},
		},
		Memory: manifest.MemorySpec{
			Supports:  []manifest.MemoryType{manifest.MemoryTypePerAgent, manifest.MemoryTypeShared, manifest.MemoryTypePublic},
			MountPath: "./memory",
		},
	}
}

func newServiceForTest(t *testing.T, runner *fakeRunner, checker *fakeChecker) *Service {
	t.Helper()
	clock := &fakeClock{current: time.Date(2026, 2, 14, 4, 20, 0, 0, time.UTC)}
	svc := NewService(nil,
		WithRunner(runner),
		WithRuntimeChecker(checker),
		WithDiagnoseDir(t.TempDir()),
		WithNow(clock.Now),
	)
	if err := svc.RegisterManifest(sampleManifest()); err != nil {
		t.Fatalf("register manifest: %v", err)
	}
	return svc
}

func newServiceForTestWithClock(t *testing.T, runner *fakeRunner, checker *fakeChecker) (*Service, *fakeClock) {
	t.Helper()
	clock := &fakeClock{current: time.Date(2026, 2, 14, 4, 20, 0, 0, time.UTC)}
	svc := NewService(nil,
		WithRunner(runner),
		WithRuntimeChecker(checker),
		WithDiagnoseDir(t.TempDir()),
		WithNow(clock.Now),
	)
	if err := svc.RegisterManifest(sampleManifest()); err != nil {
		t.Fatalf("register manifest: %v", err)
	}
	return svc, clock
}

func TestLifecycleInstallStartStop(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	runner := &fakeRunner{}
	checker := &fakeChecker{}
	svc := newServiceForTest(t, runner, checker)

	if err := svc.Install("openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := svc.Start("openclaw"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := svc.Stop("openclaw"); err != nil {
		t.Fatalf("stop: %v", err)
	}

	if checker.calls != 2 {
		t.Fatalf("expected checker called twice (install/start), got %d", checker.calls)
	}
	wantCalls := []string{"install-openclaw", "start-openclaw", "stop-openclaw"}
	if len(runner.calls) != len(wantCalls) {
		t.Fatalf("expected %d runner calls, got %d", len(wantCalls), len(runner.calls))
	}
	for i := range wantCalls {
		if runner.calls[i] != wantCalls[i] {
			t.Fatalf("runner call %d mismatch: want %q got %q", i, wantCalls[i], runner.calls[i])
		}
	}
}

func TestInstallFailsWhenPrerequisiteCheckFails(t *testing.T) {
	runner := &fakeRunner{}
	checker := &fakeChecker{err: errors.New("missing wsl")}
	svc := newServiceForTest(t, runner, checker)

	err := svc.Install("openclaw")
	if !errors.Is(err, ErrRuntimePrerequisites) {
		t.Fatalf("expected ErrRuntimePrerequisites, got %v", err)
	}

	status, statusErr := svc.Status("openclaw")
	if statusErr != nil {
		t.Fatalf("status: %v", statusErr)
	}
	if status.Install != InstallStateBroken {
		t.Fatalf("expected install state broken, got %s", status.Install)
	}
}

func TestStartFailsWhenRequiredEnvIsMissing(t *testing.T) {
	runner := &fakeRunner{}
	checker := &fakeChecker{}
	svc := newServiceForTest(t, runner, checker)

	if err := svc.Install("openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}

	err := svc.Start("openclaw")
	if !errors.Is(err, ErrMissingRequiredEnv) {
		t.Fatalf("expected ErrMissingRequiredEnv, got %v", err)
	}
}

func TestStartFailsWhenPortIsInUse(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	runner := &fakeRunner{}
	checker := &fakeChecker{}
	svc := newServiceForTest(t, runner, checker)
	origListen := listenTCP
	origOccupant := portOccupantFor
	listenTCP = func(_, _ string) (net.Listener, error) {
		return nil, errors.New("bind: address already in use")
	}
	portOccupantFor = func(_ int) string { return "pid 4242 (test-holder)" }
	t.Cleanup(func() {
		listenTCP = origListen
		portOccupantFor = origOccupant
	})

	// Override manifest with fixed port for conflict test.
	m := sampleManifest()
	m.Network.Ports = []manifest.PortSpec{{Name: "http", Port: 18080}}
	if regErr := svc.RegisterManifest(m); regErr != nil {
		t.Fatalf("re-register manifest: %v", regErr)
	}

	if err := svc.Install("openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}
	startErr := svc.Start("openclaw")
	if !errors.Is(startErr, ErrPortConflict) {
		t.Fatalf("expected ErrPortConflict, got %v", startErr)
	}
	wantPID := "pid 4242"
	if !strings.Contains(startErr.Error(), wantPID) {
		t.Fatalf("expected port conflict error to include %q, got %q", wantPID, startErr.Error())
	}
}

func TestStartDetectsCrashLoopAndAppliesCooldown(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	runner := &fakeRunner{results: map[string]runResult{
		"start-openclaw": {err: errors.New("boom")},
	}}
	checker := &fakeChecker{}
	svc, clock := newServiceForTestWithClock(t, runner, checker)

	if err := svc.Install("openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}

	for i := 0; i < defaultCrashLoopThreshold; i++ {
		err := svc.Start("openclaw")
		if err == nil {
			t.Fatalf("expected start error on attempt %d", i+1)
		}
	}

	status, statusErr := svc.Status("openclaw")
	if statusErr != nil {
		t.Fatalf("status: %v", statusErr)
	}
	if status.Runtime != RuntimeStateCrashing {
		t.Fatalf("expected runtime state crashing, got %s", status.Runtime)
	}
	if !strings.Contains(status.LastError, "crash-loop detected") {
		t.Fatalf("expected crash-loop reason, got %q", status.LastError)
	}

	blockedErr := svc.Start("openclaw")
	if !errors.Is(blockedErr, ErrCrashLoop) {
		t.Fatalf("expected ErrCrashLoop while cooling down, got %v", blockedErr)
	}

	clock.Advance(defaultCrashLoopCooldown + time.Second)
	runner.results["start-openclaw"] = runResult{result: commandexec.Result{ExitCode: 0}}
	if err := svc.Start("openclaw"); err != nil {
		t.Fatalf("expected start success after cooldown, got %v", err)
	}
}

func TestLogsAndDiagnoseArtifact(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	runner := &fakeRunner{results: map[string]runResult{
		"install-openclaw": {result: commandexec.Result{CombinedOutput: "install-ok OPENAI_API_KEY=very-secret", ExitCode: 0}},
	}}
	checker := &fakeChecker{}
	svc := newServiceForTest(t, runner, checker)

	if err := svc.Install("openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}

	logs, logErr := svc.Logs("openclaw", 10)
	if logErr != nil {
		t.Fatalf("logs: %v", logErr)
	}
	if len(logs) == 0 {
		t.Fatal("expected logs to be present")
	}

	zipPath, diagErr := svc.Diagnose("openclaw")
	if diagErr != nil {
		t.Fatalf("diagnose: %v", diagErr)
	}
	if _, statErr := os.Stat(zipPath); statErr != nil {
		t.Fatalf("expected diagnose file to exist: %v", statErr)
	}

	zr, openErr := zip.OpenReader(zipPath)
	if openErr != nil {
		t.Fatalf("open zip: %v", openErr)
	}
	defer zr.Close()

	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	sort.Strings(names)
	want := []string{"env.json", "logs.txt", "manifest.json", "metadata.json", "state.json"}
	if len(names) != len(want) {
		t.Fatalf("unexpected zip entry count: want %d got %d", len(want), len(names))
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("unexpected zip entry %d: want %q got %q", i, want[i], names[i])
		}
	}

	envPayload := readZipEntry(t, zr.File, "env.json")
	var env map[string]string
	if err := json.Unmarshal(envPayload, &env); err != nil {
		t.Fatalf("unmarshal env.json: %v", err)
	}
	if env["OPENAI_API_KEY"] != redact.RedactedValue {
		t.Fatalf("expected OPENAI_API_KEY redacted, got %q", env["OPENAI_API_KEY"])
	}

	logPayload := string(readZipEntry(t, zr.File, "logs.txt"))
	if strings.Contains(logPayload, "very-secret") {
		t.Fatalf("expected logs to redact secrets, got %q", logPayload)
	}
	if !strings.Contains(logPayload, "OPENAI_API_KEY="+redact.RedactedValue) {
		t.Fatalf("expected logs to contain redacted API key assignment, got %q", logPayload)
	}

	manifestPayload := readZipEntry(t, zr.File, "manifest.json")
	statePayload := readZipEntry(t, zr.File, "state.json")
	metadataPayload := readZipEntry(t, zr.File, "metadata.json")

	var meta redact.ArtifactMetadata
	if err := json.Unmarshal(metadataPayload, &meta); err != nil {
		t.Fatalf("unmarshal metadata.json: %v", err)
	}
	fixedNow := time.Date(2026, 2, 14, 4, 20, 0, 0, time.UTC)
	if !meta.CreatedAt.Equal(fixedNow) {
		t.Fatalf("metadata created_at mismatch: got %s want %s", meta.CreatedAt, fixedNow)
	}
	if !meta.ExpiresAt.Equal(fixedNow.Add(24 * time.Hour)) {
		t.Fatalf("metadata expires_at mismatch: got %s want %s", meta.ExpiresAt, fixedNow.Add(24*time.Hour))
	}

	wantChecksum := redact.ArtifactChecksum(map[string][]byte{
		"state.json":    statePayload,
		"manifest.json": manifestPayload,
		"logs.txt":      []byte(logPayload),
		"env.json":      envPayload,
	})
	if meta.SHA256 != wantChecksum {
		t.Fatalf("metadata sha256 mismatch: got %q want %q", meta.SHA256, wantChecksum)
	}
}

func TestHandleFailureMarksRemoteDiagnosisNeed(t *testing.T) {
	runner := &fakeRunner{}
	checker := &fakeChecker{}
	svc := newServiceForTest(t, runner, checker)
	if err := svc.Install("openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}

	triage, err := svc.HandleFailure(context.Background(), "openclaw", "startup failed")
	if err != nil {
		t.Fatalf("handle failure: %v", err)
	}
	if triage.Resolved {
		t.Fatal("expected unresolved triage in noop mode")
	}

	status, statusErr := svc.Status("openclaw")
	if statusErr != nil {
		t.Fatalf("status: %v", statusErr)
	}
	if !status.NeedsRemoteDiagnosis {
		t.Fatal("expected NeedsRemoteDiagnosis=true")
	}
	if status.LastTriageSummary == "" {
		t.Fatal("expected LastTriageSummary to be populated")
	}
}

func TestUpgradeCreatesBackupAndPreservesMemoryAttachments(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	runner := &fakeRunner{}
	checker := &fakeChecker{}
	svc := newServiceForTest(t, runner, checker)

	if err := svc.Install("openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}
	svc.memoryLinks["openclaw"] = []string{"shared/team-style@1.0.0", "per-agent/default@0.1.0"}

	if err := svc.Upgrade("openclaw"); err != nil {
		t.Fatalf("upgrade: %v", err)
	}

	entries, readDirErr := os.ReadDir(svc.diagnoseDir)
	if readDirErr != nil {
		t.Fatalf("read diagnose dir: %v", readDirErr)
	}

	var backupPath string
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "-pre-upgrade-") && strings.HasSuffix(entry.Name(), ".json") {
			backupPath = filepath.Join(svc.diagnoseDir, entry.Name())
			break
		}
	}
	if backupPath == "" {
		t.Fatal("expected pre-upgrade backup file")
	}

	raw, readErr := os.ReadFile(backupPath)
	if readErr != nil {
		t.Fatalf("read backup: %v", readErr)
	}
	var backup upgradeBackup
	if unmarshalErr := json.Unmarshal(raw, &backup); unmarshalErr != nil {
		t.Fatalf("unmarshal backup: %v", unmarshalErr)
	}

	if backup.CurrentVersion != "1.0.0" {
		t.Fatalf("expected backup version 1.0.0, got %q", backup.CurrentVersion)
	}
	if backup.RuntimeState.Runtime != RuntimeStateStopped {
		t.Fatalf("expected backup runtime state stopped, got %q", backup.RuntimeState.Runtime)
	}
	if len(backup.MemoryAttachments) != 2 {
		t.Fatalf("expected 2 memory attachments, got %d", len(backup.MemoryAttachments))
	}
	if len(backup.EnvVarKeys) != 1 || backup.EnvVarKeys[0] != "OPENAI_API_KEY" {
		t.Fatalf("unexpected env keys: %#v", backup.EnvVarKeys)
	}

	gotAttachments := svc.memoryLinks["openclaw"]
	if len(gotAttachments) != 2 {
		t.Fatalf("expected memory attachments preserved, got %d", len(gotAttachments))
	}

	logs, logErr := svc.Logs("openclaw", 50)
	if logErr != nil {
		t.Fatalf("logs: %v", logErr)
	}
	hasStartAudit := false
	hasSuccessAudit := false
	for _, line := range logs {
		if strings.Contains(line, "[audit] upgrade_start") {
			hasStartAudit = true
		}
		if strings.Contains(line, "[audit] upgrade_success") {
			hasSuccessAudit = true
		}
	}
	if !hasStartAudit || !hasSuccessAudit {
		t.Fatalf("expected upgrade start/success audit events, logs=%v", logs)
	}
}

func TestUpgradeFailureReturnsBackupGuidanceAndAuditFailure(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	runner := &fakeRunner{
		results: map[string]runResult{
			"upgrade-openclaw": {result: commandexec.Result{ExitCode: 1}, err: errors.New("upgrade crashed")},
		},
	}
	checker := &fakeChecker{}
	svc := newServiceForTest(t, runner, checker)

	if err := svc.Install("openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}
	svc.memoryLinks["openclaw"] = []string{"shared/team-style@1.0.0"}

	err := svc.Upgrade("openclaw")
	if err == nil {
		t.Fatal("expected upgrade error")
	}
	if !strings.Contains(err.Error(), "manual rollback guidance") {
		t.Fatalf("expected rollback guidance in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "-pre-upgrade-") {
		t.Fatalf("expected backup path in error, got %v", err)
	}

	gotAttachments := svc.memoryLinks["openclaw"]
	if len(gotAttachments) != 1 || gotAttachments[0] != "shared/team-style@1.0.0" {
		t.Fatalf("expected memory attachments preserved on failure, got %v", gotAttachments)
	}

	logs, logErr := svc.Logs("openclaw", 50)
	if logErr != nil {
		t.Fatalf("logs: %v", logErr)
	}
	hasFailureAudit := false
	for _, line := range logs {
		if strings.Contains(line, "[audit] upgrade_failure") {
			hasFailureAudit = true
			break
		}
	}
	if !hasFailureAudit {
		t.Fatalf("expected upgrade failure audit event, logs=%v", logs)
	}
}

func TestCreateRemoteDiagnosisHandoffRequiresNeedFlag(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	runner := &fakeRunner{}
	checker := &fakeChecker{}
	svc := newServiceForTest(t, runner, checker)

	if err := svc.Install("openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}

	_, err := svc.CreateRemoteDiagnosisHandoff("openclaw", true, "user-1", "req-1")
	if !errors.Is(err, ErrRemoteDiagnosisNotNeeded) {
		t.Fatalf("expected ErrRemoteDiagnosisNotNeeded, got %v", err)
	}
}

func TestCreateRemoteDiagnosisHandoffSuccessAndAudit(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	runner := &fakeRunner{}
	checker := &fakeChecker{}
	svc := newServiceForTest(t, runner, checker)

	if err := svc.Install("openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}
	if _, err := svc.HandleFailure(context.Background(), "openclaw", "cannot bind port"); err != nil {
		t.Fatalf("handle failure: %v", err)
	}

	artifact, err := svc.Diagnose("openclaw")
	if err != nil {
		t.Fatalf("diagnose: %v", err)
	}

	handoff, err := svc.CreateRemoteDiagnosisHandoff("openclaw", true, "chat:123", "req-42")
	if err != nil {
		t.Fatalf("create handoff: %v", err)
	}
	if handoff.Status != HandoffStatusPending {
		t.Fatalf("expected pending handoff, got %s", handoff.Status)
	}
	if handoff.ArtifactRef != artifact {
		t.Fatalf("expected artifact %q, got %q", artifact, handoff.ArtifactRef)
	}

	audits := svc.AuditLogs()
	if len(audits) == 0 {
		t.Fatal("expected audit logs")
	}
	last := audits[len(audits)-1]
	if last.Action != "remote_diagnosis_consent" {
		t.Fatalf("expected last audit action remote_diagnosis_consent, got %s", last.Action)
	}
	if last.RequestID != "req-42" {
		t.Fatalf("expected request id req-42, got %s", last.RequestID)
	}
}

func TestCleanupExpiredDiagnosisHandoffs(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	runner := &fakeRunner{}
	checker := &fakeChecker{}
	svc, clock := newServiceForTestWithClock(t, runner, checker)

	if err := svc.Install("openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}
	if _, err := svc.HandleFailure(context.Background(), "openclaw", "cannot bind port"); err != nil {
		t.Fatalf("handle failure: %v", err)
	}
	if _, err := svc.Diagnose("openclaw"); err != nil {
		t.Fatalf("diagnose: %v", err)
	}
	if _, err := svc.CreateRemoteDiagnosisHandoff("openclaw", true, "chat:1", "req-old"); err != nil {
		t.Fatalf("create old handoff: %v", err)
	}

	// Advance clock and create a fresh handoff.
	clock.Advance(25 * time.Hour)
	if _, err := svc.CreateRemoteDiagnosisHandoff("openclaw", true, "chat:2", "req-new"); err != nil {
		t.Fatalf("create new handoff: %v", err)
	}

	removed := svc.CleanupExpiredDiagnosisHandoffs()
	if removed != 1 {
		t.Fatalf("expected 1 removed handoff, got %d", removed)
	}

	handoffs := svc.DiagnosisHandoffs()
	if len(handoffs) != 1 {
		t.Fatalf("expected 1 retained handoff, got %d", len(handoffs))
	}

	audits := svc.AuditLogs()
	last := audits[len(audits)-1]
	if last.Action != "handoff_cleanup" {
		t.Fatalf("expected last audit action handoff_cleanup, got %s", last.Action)
	}
}

func TestCreateRemoteDiagnosisHandoffDeclinedUsesNeutralAuditResult(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	runner := &fakeRunner{}
	checker := &fakeChecker{}
	svc := newServiceForTest(t, runner, checker)

	if err := svc.Install("openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}
	if _, err := svc.HandleFailure(context.Background(), "openclaw", "cannot bind port"); err != nil {
		t.Fatalf("handle failure: %v", err)
	}
	if _, err := svc.Diagnose("openclaw"); err != nil {
		t.Fatalf("diagnose: %v", err)
	}

	handoff, err := svc.CreateRemoteDiagnosisHandoff("openclaw", false, "chat:123", "req-43")
	if err != nil {
		t.Fatalf("create handoff: %v", err)
	}
	if handoff.Status != HandoffStatusDeclined {
		t.Fatalf("expected declined handoff, got %s", handoff.Status)
	}

	audits := svc.AuditLogs()
	last := audits[len(audits)-1]
	if last.Result != AuditResultNeutral {
		t.Fatalf("expected neutral audit result, got %s", last.Result)
	}
}

func TestAuditLogsBoundedByConfiguredLimit(t *testing.T) {
	svc := NewService(nil,
		WithAuditLogLimit(3),
		WithNow(func() time.Time { return time.Date(2026, 2, 14, 4, 20, 0, 0, time.UTC) }),
	)

	svc.recordAudit("r1", "a", "x1", "t", AuditResultSuccess, "", "m1")
	svc.recordAudit("r2", "a", "x2", "t", AuditResultSuccess, "", "m2")
	svc.recordAudit("r3", "a", "x3", "t", AuditResultSuccess, "", "m3")
	svc.recordAudit("r4", "a", "x4", "t", AuditResultSuccess, "", "m4")

	audits := svc.AuditLogs()
	if len(audits) != 3 {
		t.Fatalf("expected 3 audit logs, got %d", len(audits))
	}
	if audits[0].RequestID != "r2" {
		t.Fatalf("expected oldest retained request id r2, got %s", audits[0].RequestID)
	}
	if audits[2].RequestID != "r4" {
		t.Fatalf("expected latest retained request id r4, got %s", audits[2].RequestID)
	}
}

// Wrappers to keep tests explicit and avoid importing extra packages in each assertion block.
var (
	netListen     = net.Listen
	splitHostPort = net.SplitHostPort
	atoi          = strconv.Atoi
)

func readZipEntry(t *testing.T, files []*zip.File, name string) []byte {
	t.Helper()
	for _, f := range files {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", name, err)
		}
		defer rc.Close()
		payload, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read zip entry %s: %v", name, err)
		}
		return payload
	}
	t.Fatalf("missing zip entry %s", name)
	return nil
}
