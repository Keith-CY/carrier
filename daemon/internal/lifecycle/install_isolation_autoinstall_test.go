package lifecycle

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestInstallWithIsolationAutoInstallsBwrapOnLinux(t *testing.T) {
	origGOOS := isolationRuntimeGOOS
	origLookup := isolationBackendLookup
	origEnv := isolationEnvLookup
	origAutoLookPath := autoInstallLookPath
	origAutoExec := autoInstallExecCmd
	t.Cleanup(func() {
		isolationRuntimeGOOS = origGOOS
		isolationBackendLookup = origLookup
		isolationEnvLookup = origEnv
		autoInstallLookPath = origAutoLookPath
		autoInstallExecCmd = origAutoExec
	})

	isolationRuntimeGOOS = "linux"
	isolationEnvLookup = func(string) string { return "" }
	lookupCalls := 0
	bwrapInstalled := false
	isolationBackendLookup = func(name string) (string, error) {
		if name != "bwrap" {
			return "", errors.New("unsupported lookup")
		}
		lookupCalls++
		if lookupCalls == 1 {
			return "", errors.New("missing bwrap")
		}
		if bwrapInstalled {
			return "/usr/bin/bwrap", nil
		}
		return "", errors.New("missing bwrap")
	}

	autoInstallLookPath = func(name string) (string, error) {
		switch name {
		case "apt-get", "sudo":
			return "/usr/bin/" + name, nil
		case "bwrap":
			if bwrapInstalled {
				return "/usr/bin/bwrap", nil
			}
			return "", errors.New("missing")
		default:
			return "", errors.New("missing")
		}
	}
	autoInstallExecCmd = func(name string, args ...string) *exec.Cmd {
		command := strings.TrimSpace(name + " " + strings.Join(args, " "))
		if command == "sudo -n apt-get install -y bubblewrap" {
			bwrapInstalled = true
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	runner := &fakeRunner{}
	checker := &fakeChecker{}
	clock := &fakeClock{current: time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)}
	svc := NewService(nil,
		WithRunner(runner),
		WithRuntimeChecker(checker),
		WithNow(clock.Now),
		WithDiagnoseDir(t.TempDir()),
	)
	m := sampleManifest()
	if err := svc.RegisterManifest(m); err != nil {
		t.Fatalf("RegisterManifest: %v", err)
	}
	if err := svc.InstallWithOptions(context.Background(), m.ID, InstallOptions{Isolation: true}); err != nil {
		t.Fatalf("InstallWithOptions: %v", err)
	}
	if lookupCalls < 2 {
		t.Fatalf("expected backend lookup to be retried after auto-install, calls=%d", lookupCalls)
	}
}
