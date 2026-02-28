package lifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestUninstallAttemptsDarwinIsolationCleanup(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(defaultLimaInstanceEnvKey, "carrier-dev")

	templatePath := filepath.Join(home, ".carrier", "lima", "carrier-dev.yaml")
	if err := os.MkdirAll(filepath.Dir(templatePath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(templatePath, []byte("mounts: []\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	origGOOS := isolationRuntimeGOOS
	origRun := isolationCleanupRunCommand
	origRemove := isolationCleanupRemoveFile
	t.Cleanup(func() {
		isolationRuntimeGOOS = origGOOS
		isolationCleanupRunCommand = origRun
		isolationCleanupRemoveFile = origRemove
	})

	isolationRuntimeGOOS = "darwin"
	commands := make([][]string, 0, 2)
	isolationCleanupRunCommand = func(name string, args ...string) error {
		commands = append(commands, append([]string{name}, args...))
		return nil
	}
	isolationCleanupRemoveFile = os.Remove

	svc := newServiceForTest(t, &fakeRunner{}, &fakeChecker{})
	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("Install: %v", err)
	}

	if err := svc.Uninstall(context.Background(), "openclaw"); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	want := [][]string{
		{"limactl", "stop", "carrier-dev"},
		{"limactl", "delete", "carrier-dev"},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("cleanup commands mismatch\nwant=%v\ngot=%v", want, commands)
	}
	if _, err := os.Stat(templatePath); !os.IsNotExist(err) {
		t.Fatalf("expected template file to be removed, stat err=%v", err)
	}
}

func TestUninstallCleanupErrorsDoNotBlock(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv(defaultLimaInstanceEnvKey, "carrier-dev")

	origGOOS := isolationRuntimeGOOS
	origRun := isolationCleanupRunCommand
	origRemove := isolationCleanupRemoveFile
	t.Cleanup(func() {
		isolationRuntimeGOOS = origGOOS
		isolationCleanupRunCommand = origRun
		isolationCleanupRemoveFile = origRemove
	})

	isolationRuntimeGOOS = "darwin"
	isolationCleanupRunCommand = func(_ string, _ ...string) error {
		return errors.New("cleanup failure")
	}
	isolationCleanupRemoveFile = func(_ string) error {
		return errors.New("remove failure")
	}

	svc := newServiceForTest(t, &fakeRunner{}, &fakeChecker{})
	if err := svc.Install(context.Background(), "openclaw"); err != nil {
		t.Fatalf("Install: %v", err)
	}

	if err := svc.Uninstall(context.Background(), "openclaw"); err != nil {
		t.Fatalf("Uninstall should succeed despite cleanup errors, got: %v", err)
	}

	_, state, err := svc.getManifestAndState("openclaw")
	if err != nil {
		t.Fatalf("getManifestAndState: %v", err)
	}
	if state.Install != InstallStateNotInstalled {
		t.Fatalf("expected not installed after uninstall, got %q", state.Install)
	}
}
