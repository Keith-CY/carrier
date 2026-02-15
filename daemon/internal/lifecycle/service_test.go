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
	"reflect"
	"sort"
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
			Start:   manifest.CommandSpec{Command: "tail -f /dev/null"},
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
		Upgrade: manifest.UpgradeSpec{
			Channel:  "stable",
			Strategy: manifest.UpgradeStrategyInPlaceOrReinstall,
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

	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := svc.Start(context.Background(), "openclaw"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := svc.Stop(context.Background(), "openclaw"); err != nil {
		t.Fatalf("stop: %v", err)
	}

	if checker.calls != 2 {
		t.Fatalf("expected checker called twice (install/start), got %d", checker.calls)
	}
	wantCalls := []string{"install-openclaw"}
	if len(runner.calls) != len(wantCalls) {
		t.Fatalf("expected %d runner calls, got %d", len(wantCalls), len(runner.calls))
	}
	for i := range wantCalls {
		if runner.calls[i] != wantCalls[i] {
			t.Fatalf("runner call %d mismatch: want %q got %q", i, wantCalls[i], runner.calls[i])
		}
	}
}

func TestLifecycleUpgradeCreatesBackupAndBumpsVersion(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	runner := &fakeRunner{}
	checker := &fakeChecker{}
	svc := newServiceForTest(t, runner, checker)

	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}
	result, err := svc.Upgrade(context.Background(), "openclaw")
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}

	if result.FromVersion != "1.0.0" {
		t.Fatalf("unexpected from version: %s", result.FromVersion)
	}
	if result.ToVersion != "1.0.1" {
		t.Fatalf("unexpected to version: %s", result.ToVersion)
	}
	if result.BackupPath == "" {
		t.Fatal("expected backup path")
	}
	if _, err := os.Stat(result.BackupPath); err != nil {
		t.Fatalf("expected backup path to exist: %v", err)
	}
	if !strings.Contains(result.BackupPath, "-pre-upgrade-") || !strings.HasSuffix(result.BackupPath, ".json") {
		t.Fatalf("expected json pre-upgrade backup path, got %q", result.BackupPath)
	}

	status, statusErr := svc.Status("openclaw")
	if statusErr != nil {
		t.Fatalf("status: %v", statusErr)
	}
	if status.Version != "1.0.1" {
		t.Fatalf("expected upgraded version 1.0.1, got %q", status.Version)
	}

	wantCalls := []string{"install-openclaw", "upgrade-openclaw"}
	if len(runner.calls) != len(wantCalls) {
		t.Fatalf("expected %d runner calls, got %d", len(wantCalls), len(runner.calls))
	}
	for i := range wantCalls {
		if runner.calls[i] != wantCalls[i] {
			t.Fatalf("runner call %d mismatch: want %q got %q", i, wantCalls[i], runner.calls[i])
		}
	}
}

func TestLifecycleUpgradeRequiresStoppedAgent(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	runner := &fakeRunner{}
	checker := &fakeChecker{}
	svc := newServiceForTest(t, runner, checker)

	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := svc.Start(context.Background(), "openclaw"); err != nil {
		t.Fatalf("start: %v", err)
	}

	if _, err := svc.Upgrade(context.Background(), "openclaw"); !errors.Is(err, ErrAgentRunning) {
		t.Fatalf("expected ErrAgentRunning, got %v", err)
	}
}

func TestLifecycleUpgradeRejectsUnsupportedStrategy(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	runner := &fakeRunner{}
	checker := &fakeChecker{}
	svc := newServiceForTest(t, runner, checker)
	brokenManifest := sampleManifest()
	brokenManifest.Upgrade = manifest.UpgradeSpec{
		Channel:  "stable",
		Strategy: "unsupported",
	}
	svc.mu.Lock()
	svc.manifests["openclaw"] = brokenManifest
	svc.mu.Unlock()
	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}

	_, err := svc.Upgrade(context.Background(), "openclaw")
	if err == nil {
		t.Fatal("expected upgrade strategy error")
	}
	if !errors.Is(err, ErrUpgradeStrategyUnsupported) {
		t.Fatalf("expected ErrUpgradeStrategyUnsupported, got %v", err)
	}
}

func TestLifecycleUpgradeFailureKeepsStateAndReturnsRollbackGuidance(t *testing.T) {
	runner := &fakeRunner{
		results: map[string]runResult{
			"install-openclaw": {result: commandexec.Result{ExitCode: 0}},
		},
	}
	checker := &fakeChecker{}
	svc := newServiceForTest(t, runner, checker)

	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}
	runner.results["upgrade-openclaw"] = runResult{err: errors.New("upgrade command failed"), result: commandexec.Result{ExitCode: 1}}
	result, err := svc.Upgrade(context.Background(), "openclaw")
	if err == nil {
		t.Fatal("expected upgrade failure")
	}
	if result.FromVersion != "1.0.0" {
		t.Fatalf("unexpected from version: %s", result.FromVersion)
	}
	if result.ToVersion != "1.0.0" {
		t.Fatalf("expected to version unchanged on failure, got %s", result.ToVersion)
	}
	if !strings.Contains(err.Error(), "rollback") {
		t.Fatalf("expected rollback guidance in error, got %q", err.Error())
	}
	if result.BackupPath == "" {
		t.Fatal("expected backup path on upgrade failure")
	}
	if _, err := os.Stat(result.BackupPath); err != nil {
		t.Fatalf("expected backup path to exist: %v", err)
	}

	status, statusErr := svc.Status("openclaw")
	if statusErr != nil {
		t.Fatalf("status: %v", statusErr)
	}
	if status.Version != "1.0.0" {
		t.Fatalf("expected unchanged version after failed upgrade, got %q", status.Version)
	}
	if !strings.Contains(status.LastError, "backup captured at") {
		t.Fatalf("expected state last error with rollback guidance, got %q", status.LastError)
	}
}

func TestInstallFailsWhenPrerequisiteCheckFails(t *testing.T) {
	runner := &fakeRunner{}
	checker := &fakeChecker{err: errors.New("missing wsl")}
	svc := newServiceForTest(t, runner, checker)

	err := svc.Install(context.Background(), "openclaw")
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

	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}

	err := svc.Start(context.Background(), "openclaw")
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

	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}
	startErr := svc.Start(context.Background(), "openclaw")
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
	runner := &fakeRunner{results: map[string]runResult{}}
	checker := &fakeChecker{}
	svc, clock := newServiceForTestWithClock(t, runner, checker)

	m := sampleManifest()
	m.Runtime.Start.Command = "missing-start-command"
	if regErr := svc.RegisterManifest(m); regErr != nil {
		t.Fatalf("re-register manifest: %v", regErr)
	}

	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}

	for i := 0; i < defaultCrashLoopThreshold; i++ {
		err := svc.Start(context.Background(), "openclaw")
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

	blockedErr := svc.Start(context.Background(), "openclaw")
	if !errors.Is(blockedErr, ErrCrashLoop) {
		t.Fatalf("expected ErrCrashLoop while cooling down, got %v", blockedErr)
	}

	clock.Advance(defaultCrashLoopCooldown + time.Second)
	svc.mu.Lock()
	updated := svc.manifests["openclaw"]
	updated.Runtime.Start.Command = "tail -f /dev/null"
	svc.manifests["openclaw"] = updated
	svc.mu.Unlock()
	if err := svc.Start(context.Background(), "openclaw"); err != nil {
		t.Fatalf("expected start success after cooldown, got %v", err)
	}
	if err := svc.Stop(context.Background(), "openclaw"); err != nil {
		t.Fatalf("stop after recovery start: %v", err)
	}
}

func TestLogsAndDiagnoseArtifact(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	runner := &fakeRunner{results: map[string]runResult{
		"install-openclaw": {result: commandexec.Result{CombinedOutput: "install-ok OPENAI_API_KEY=very-secret", ExitCode: 0}},
	}}
	checker := &fakeChecker{}
	svc := newServiceForTest(t, runner, checker)

	if err := svc.Install(context.Background(), "openclaw"); err != nil {
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
	want := []string{"agent_manifest.json", "env.json", "logs.txt", "manifest.json", "metadata.json", "state.json"}
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

	diagnoseManifestPayload := readZipEntry(t, zr.File, "manifest.json")
	agentManifestPayload := readZipEntry(t, zr.File, "agent_manifest.json")
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
		"manifest.json":       diagnoseManifestPayload,
		"state.json":          statePayload,
		"agent_manifest.json": agentManifestPayload,
		"logs.txt":            []byte(logPayload),
		"env.json":            envPayload,
	})
	if meta.SHA256 != wantChecksum {
		t.Fatalf("metadata sha256 mismatch: got %q want %q", meta.SHA256, wantChecksum)
	}
}

func TestHandleFailureMarksRemoteDiagnosisNeed(t *testing.T) {
	runner := &fakeRunner{}
	checker := &fakeChecker{}
	svc := newServiceForTest(t, runner, checker)
	if err := svc.Install(context.Background(), "openclaw"); err != nil {
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

func TestUpgradeRejectsWhenNoUpgradeCommand(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	runner := &fakeRunner{}
	checker := &fakeChecker{}
	svc := NewService(nil,
		WithRunner(runner),
		WithRuntimeChecker(checker),
		WithDiagnoseDir(t.TempDir()),
	)
	// Register a manifest without upgrade command
	m := sampleManifest()
	m.Runtime.Upgrade = manifest.CommandSpec{}
	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("register: %v", err)
	}

	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}

	_, err := svc.Upgrade(context.Background(), "openclaw")
	if !errors.Is(err, ErrUpgradeNotSupported) {
		t.Fatalf("expected ErrUpgradeNotSupported, got %v", err)
	}
}

func TestUpgradeRejectsWhenAgentRunning(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	runner := &fakeRunner{}
	checker := &fakeChecker{}
	svc := newServiceForTest(t, runner, checker)

	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := svc.Start(context.Background(), "openclaw"); err != nil {
		t.Fatalf("start: %v", err)
	}

	_, err := svc.Upgrade(context.Background(), "openclaw")
	if !errors.Is(err, ErrAgentRunning) {
		t.Fatalf("expected ErrAgentRunning, got %v", err)
	}
}

func TestUpgradeResetsCrashLoopState(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	runner := &fakeRunner{results: map[string]runResult{}}
	checker := &fakeChecker{}
	svc, clock := newServiceForTestWithClock(t, runner, checker)

	m := sampleManifest()
	m.Runtime.Start.Command = "missing-start-command"
	if regErr := svc.RegisterManifest(m); regErr != nil {
		t.Fatalf("re-register manifest: %v", regErr)
	}

	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}

	// Trigger crash-loop
	for i := 0; i < defaultCrashLoopThreshold; i++ {
		if err := svc.Start(context.Background(), "openclaw"); err == nil {
			t.Fatal("expected start to fail while triggering crash loop")
		}
	}

	status, err := svc.Status("openclaw")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Runtime != RuntimeStateCrashing {
		t.Fatalf("expected crashing, got %s", status.Runtime)
	}

	// Verify blocked by cooldown
	blockedErr := svc.Start(context.Background(), "openclaw")
	if !errors.Is(blockedErr, ErrCrashLoop) {
		t.Fatalf("expected ErrCrashLoop, got %v", blockedErr)
	}

	// Stop and upgrade should reset crash-loop state
	clock.Advance(defaultCrashLoopCooldown + time.Second)
	// Force state to stopped for upgrade
	_ = svc.Stop(context.Background(), "openclaw")

	runner.results["upgrade-openclaw"] = runResult{result: commandexec.Result{ExitCode: 0}}
	if _, err := svc.Upgrade(context.Background(), "openclaw"); err != nil {
		t.Fatalf("upgrade: %v", err)
	}

	status, err = svc.Status("openclaw")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Runtime != RuntimeStateStopped {
		t.Fatalf("expected stopped after upgrade, got %s", status.Runtime)
	}
	if status.Health != HealthStateUnknown {
		t.Fatalf("expected unknown health after upgrade, got %s", status.Health)
	}
	if status.LastError != "" {
		t.Fatalf("expected empty LastError after upgrade, got %q", status.LastError)
	}

	// Verify can start without crash-loop blocking.
	// Restore a valid start command so this assertion checks crash-loop reset behavior,
	// not command validity.
	m.Runtime.Start.Command = "tail -f /dev/null"
	svc.mu.Lock()
	svc.manifests["openclaw"] = m
	svc.mu.Unlock()
	if err := svc.Start(context.Background(), "openclaw"); err != nil {
		t.Fatalf("expected start success after upgrade reset, got %v", err)
	}
	if err := svc.Stop(context.Background(), "openclaw"); err != nil {
		t.Fatalf("stop after post-upgrade start: %v", err)
	}
}

func TestUpgradeCreatesBackupAndPreservesMemoryAttachments(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	runner := &fakeRunner{}
	checker := &fakeChecker{}
	svc := newServiceForTest(t, runner, checker)

	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}
	svc.memoryLinks["openclaw"] = []string{"shared/team-style@1.0.0", "per-agent/default@0.1.0"}

	if _, err := svc.Upgrade(context.Background(), "openclaw"); err != nil {
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

	auditLogs := svc.AuditLogs()
	hasStartAudit := false
	hasSuccessAudit := false
	for _, log := range auditLogs {
		if log.Action == "upgrade" && log.Target == "openclaw" && strings.Contains(log.Message, "upgrade_start") {
			hasStartAudit = true
		}
		if log.Action == "upgrade" && log.Target == "openclaw" && strings.Contains(log.Message, "upgrade_success") {
			hasSuccessAudit = true
		}
	}
	if !hasStartAudit || !hasSuccessAudit {
		t.Fatalf("expected upgrade start/success audit events, got %d audit logs", len(auditLogs))
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

	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}
	svc.memoryLinks["openclaw"] = []string{"shared/team-style@1.0.0"}

	_, err := svc.Upgrade(context.Background(), "openclaw")
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

	auditLogs := svc.AuditLogs()
	hasFailureAudit := false
	for _, log := range auditLogs {
		if log.Action == "upgrade" && log.Target == "openclaw" && log.Result == AuditResultFailure && strings.Contains(log.Message, "upgrade_failure") {
			hasFailureAudit = true
			break
		}
	}
	if !hasFailureAudit {
		t.Fatalf("expected upgrade failure audit event, got %d audit logs", len(auditLogs))
	}
}

func TestUpgradeBackupCreationFailureReturnsFocusedError(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	runner := &fakeRunner{}
	checker := &fakeChecker{}
	svc := newServiceForTest(t, runner, checker)

	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}

	blockedPath := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blockedPath, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocked path: %v", err)
	}
	svc.diagnoseDir = blockedPath

	_, err := svc.Upgrade(context.Background(), "openclaw")
	if err == nil {
		t.Fatal("expected upgrade error")
	}
	if !strings.Contains(err.Error(), "create diagnose dir") {
		t.Fatalf("expected diagnose-dir creation error, got %v", err)
	}
	if strings.Contains(err.Error(), "manual rollback guidance") || strings.Contains(err.Error(), "-pre-upgrade-") {
		t.Fatalf("backup guidance should not be shown when backup creation fails, got %v", err)
	}

	for _, log := range svc.AuditLogs() {
		if log.Action == "upgrade" && strings.Contains(log.Message, "upgrade_start") {
			t.Fatalf("unexpected upgrade_start audit when backup creation failed: %+v", log)
		}
	}
}

func TestCreateRemoteDiagnosisHandoffRequiresNeedFlag(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	runner := &fakeRunner{}
	checker := &fakeChecker{}
	svc := newServiceForTest(t, runner, checker)

	if err := svc.Install(context.Background(), "openclaw"); err != nil {
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

	if err := svc.Install(context.Background(), "openclaw"); err != nil {
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

	if err := svc.Install(context.Background(), "openclaw"); err != nil {
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

	if err := svc.Install(context.Background(), "openclaw"); err != nil {
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

func TestAuditBufferStatus(t *testing.T) {
	svc := NewService(nil, WithAuditLogLimit(50))

	status := svc.AuditBufferStatus()
	if status.BufferSize != 0 {
		t.Fatalf("expected empty buffer, got %d", status.BufferSize)
	}
	if status.Limit != 50 {
		t.Fatalf("expected limit 50, got %d", status.Limit)
	}

	// Generate some audit entries by installing an agent.
	m := manifest.Manifest{
		ID: "a", Name: "a", Version: "1",
		Runtime: manifest.RuntimeSpec{
			Type:    manifest.RuntimeTypeLocalBinary,
			Install: manifest.CommandSpec{Command: "echo ok"},
			Start:   manifest.CommandSpec{Command: "echo start"},
			Stop:    manifest.CommandSpec{Command: "echo stop"},
		},
		Memory: manifest.MemorySpec{
			MountPath: "/tmp/test-audit-mem",
			Supports:  []manifest.MemoryType{manifest.MemoryTypePerAgent},
		},
	}
	if err := svc.RegisterManifest(m); err != nil {
		t.Fatal(err)
	}
	if err := svc.Install(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}

	status = svc.AuditBufferStatus()
	if status.BufferSize == 0 {
		t.Fatal("expected non-zero buffer after install")
	}
}

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

func TestMemoryAttachmentsSetAndGet(t *testing.T) {
	runner := &fakeRunner{}
	checker := &fakeChecker{}
	svc := newServiceForTest(t, runner, checker)

	// Initially empty
	got := svc.getMemoryAttachments("openclaw")
	if len(got) != 0 {
		t.Fatalf("expected empty attachments, got %v", got)
	}

	// Set and get
	svc.setMemoryAttachments("openclaw", []string{"/mem/a.md", "/mem/b.md"})
	got = svc.getMemoryAttachments("openclaw")
	if !reflect.DeepEqual(got, []string{"/mem/a.md", "/mem/b.md"}) {
		t.Fatalf("unexpected attachments: %v", got)
	}

	// Returned slice is a copy (mutation-safe)
	got[0] = "mutated"
	fresh := svc.getMemoryAttachments("openclaw")
	if fresh[0] == "mutated" {
		t.Fatal("getMemoryAttachments must return a copy")
	}
}

func TestMemoryAttachmentsPreservedAcrossStartStop(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	runner := &fakeRunner{}
	checker := &fakeChecker{}
	svc := newServiceForTest(t, runner, checker)

	svc.setMemoryAttachments("openclaw", []string{"/mem/persist.md"})

	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := svc.Start(context.Background(), "openclaw"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := svc.Stop(context.Background(), "openclaw"); err != nil {
		t.Fatalf("stop: %v", err)
	}

	got := svc.getMemoryAttachments("openclaw")
	if !reflect.DeepEqual(got, []string{"/mem/persist.md"}) {
		t.Fatalf("attachments lost after start/stop: %v", got)
	}
}

func TestMemoryAttachmentsPreservedAcrossUpgrade(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	runner := &fakeRunner{}
	checker := &fakeChecker{}
	svc := newServiceForTest(t, runner, checker)

	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}

	svc.setMemoryAttachments("openclaw", []string{"/mem/keep.md"})

	_, err := svc.Upgrade(context.Background(), "openclaw")
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}

	got := svc.getMemoryAttachments("openclaw")
	if !reflect.DeepEqual(got, []string{"/mem/keep.md"}) {
		t.Fatalf("attachments lost after upgrade: %v", got)
	}
}

func TestLoadPersistedState_VerifiesProcessLiveness(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	// Create a service with state file enabled
	runner := &fakeRunner{results: make(map[string]runResult)}
	checker := &fakeChecker{}
	clock := &fakeClock{current: time.Date(2026, 2, 15, 5, 0, 0, 0, time.UTC)}

	svc := NewService(nil,
		WithRunner(runner),
		WithRuntimeChecker(checker),
		WithNow(clock.Now),
		WithStateFile(statePath),
	)

	// Register an agent
	m := sampleManifest()
	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("register manifest: %v", err)
	}

	// Create a persisted state file with RuntimeStateRunning
	// but without actually starting the process
	now := clock.Now()
	persistedStates := map[string]PersistedAgentState{
		"openclaw": {
			ID:             "openclaw",
			Installed:      true,
			RuntimeState:   string(RuntimeStateRunning),
			LastTransition: now,
		},
	}

	data, err := json.MarshalIndent(persistedStates, "", "  ")
	if err != nil {
		t.Fatalf("marshal persisted state: %v", err)
	}

	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatalf("write state file: %v", err)
	}

	// Load the persisted state
	if err := svc.loadPersistedState(); err != nil {
		t.Fatalf("loadPersistedState failed: %v", err)
	}

	// Verify that the state was corrected from "running" to "stopped"
	// because the process is not actually running
	state, err := svc.Status("openclaw")
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	if state.Runtime != RuntimeStateStopped {
		t.Errorf("expected runtime state to be corrected to %q, got %q", RuntimeStateStopped, state.Runtime)
	}

	if state.Install != InstallStateInstalled {
		t.Errorf("expected install state %q, got %q", InstallStateInstalled, state.Install)
	}
}

func TestLoadPersistedState_KeepsRunningIfProcessAlive(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	// Create a service with state file enabled
	runner := &fakeRunner{results: make(map[string]runResult)}
	checker := &fakeChecker{}
	clock := &fakeClock{current: time.Date(2026, 2, 15, 5, 0, 0, 0, time.UTC)}

	svc := NewService(nil,
		WithRunner(runner),
		WithRuntimeChecker(checker),
		WithNow(clock.Now),
		WithStateFile(statePath),
	)

	// Register and install the agent
	m := sampleManifest()
	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("register manifest: %v", err)
	}

	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("install: %v", err)
	}

	// Start the agent so there's an actual process running
	t.Setenv("OPENAI_API_KEY", "test-key")
	if err := svc.Start(context.Background(), "openclaw"); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Save the current state (which includes RuntimeStateRunning)
	svc.saveState()

	// Create a new service instance and load the persisted state
	svc2 := NewService(nil,
		WithRunner(runner),
		WithRuntimeChecker(checker),
		WithNow(clock.Now),
		WithStateFile(statePath),
		WithProcessLogDir(svc.processLogDir),
	)
	svc2.processManager = svc.processManager // Share the process manager so it sees the running process

	if err := svc2.RegisterManifest(m); err != nil {
		t.Fatalf("register manifest: %v", err)
	}

	if err := svc2.loadPersistedState(); err != nil {
		t.Fatalf("loadPersistedState failed: %v", err)
	}

	// Verify that the state remains "running" because the process is actually alive
	state, err := svc2.Status("openclaw")
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	if state.Runtime != RuntimeStateRunning {
		t.Errorf("expected runtime state to remain %q, got %q", RuntimeStateRunning, state.Runtime)
	}

	// Clean up
	if err := svc.Stop(context.Background(), "openclaw"); err != nil {
		t.Logf("cleanup stop: %v", err)
	}
}
