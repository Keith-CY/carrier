package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPickMinimalProviderWithReasonUsesEnvDefault(t *testing.T) {
	t.Setenv("CARRIER_DEFAULT_PROVIDER_ID", "openai")

	provider, reason, err := pickMinimalProviderWithReason()
	if err != nil {
		t.Fatalf("pickMinimalProviderWithReason error: %v", err)
	}
	if provider.ID != "openai" {
		t.Fatalf("provider.ID = %q, want %q", provider.ID, "openai")
	}
	if !strings.Contains(reason, "CARRIER_DEFAULT_PROVIDER_ID") {
		t.Fatalf("reason should include env-default context, got %q", reason)
	}
}

func TestPickMinimalProviderWithReasonFallsBackToOpenAICodex(t *testing.T) {
	t.Setenv("CARRIER_DEFAULT_PROVIDER_ID", "")

	provider, reason, err := pickMinimalProviderWithReason()
	if err != nil {
		t.Fatalf("pickMinimalProviderWithReason error: %v", err)
	}
	if provider.ID != "openai-codex" {
		t.Fatalf("provider.ID = %q, want %q", provider.ID, "openai-codex")
	}
	if !strings.Contains(strings.ToLower(reason), "fallback") {
		t.Fatalf("reason should describe fallback selection, got %q", reason)
	}
}

func TestLookupPIDsByPortViaProc(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "net"), 0o755); err != nil {
		t.Fatalf("mkdir net: %v", err)
	}
	tcpData := strings.Join([]string{
		"  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode",
		"   0: 0100007F:2253 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 12345 1 0000000000000000 100 0 0 10 0",
	}, "\n")
	if err := os.WriteFile(filepath.Join(tmp, "net", "tcp"), []byte(tcpData), 0o644); err != nil {
		t.Fatalf("write tcp file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "net", "tcp6"), []byte(""), 0o644); err != nil {
		t.Fatalf("write tcp6 file: %v", err)
	}

	fdDir := filepath.Join(tmp, "111", "fd")
	if err := os.MkdirAll(fdDir, 0o755); err != nil {
		t.Fatalf("mkdir fd dir: %v", err)
	}
	if err := os.Symlink("socket:[12345]", filepath.Join(fdDir, "5")); err != nil {
		t.Fatalf("create socket symlink: %v", err)
	}

	oldRoot := procFSRoot
	procFSRoot = tmp
	defer func() { procFSRoot = oldRoot }()

	pids, err := lookupPIDsByPortViaProc(8787)
	if err != nil {
		t.Fatalf("lookupPIDsByPortViaProc error: %v", err)
	}
	if len(pids) != 1 || pids[0] != 111 {
		t.Fatalf("pids = %v, want [111]", pids)
	}
}

func TestPersistBackgroundProcessRejectsInvalidProcess(t *testing.T) {
	if err := persistBackgroundProcess("daemon", nil); err == nil {
		t.Fatal("expected error for nil process")
	}
	if err := persistBackgroundProcess("daemon", &os.Process{Pid: 0}); err == nil {
		t.Fatal("expected error for empty pid")
	}
}

func TestPersistBackgroundProcessCleansUpAfterPIDWriteFailure(t *testing.T) {
	origWrite := writeBootstrapPIDFileFunc
	origTerminate := terminateBackgroundProcessFunc
	t.Cleanup(func() {
		writeBootstrapPIDFileFunc = origWrite
		terminateBackgroundProcessFunc = origTerminate
	})

	writeBootstrapPIDFileFunc = func(name string, pid int) error {
		if name != "daemon" {
			t.Fatalf("name = %q, want daemon", name)
		}
		if pid != 321 {
			t.Fatalf("pid = %d, want 321", pid)
		}
		return errors.New("disk full")
	}

	cleanupCalled := false
	terminateBackgroundProcessFunc = func(proc *os.Process) error {
		cleanupCalled = true
		if proc == nil || proc.Pid != 321 {
			t.Fatalf("cleanup process = %#v, want pid 321", proc)
		}
		return nil
	}

	err := persistBackgroundProcess("daemon", &os.Process{Pid: 321})
	if err == nil {
		t.Fatal("expected pid write error")
	}
	if !strings.Contains(err.Error(), "write bootstrap pid file") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cleanupCalled {
		t.Fatal("expected cleanup to run after pid write failure")
	}
}

func TestPersistBackgroundProcessIncludesCleanupFailure(t *testing.T) {
	origWrite := writeBootstrapPIDFileFunc
	origTerminate := terminateBackgroundProcessFunc
	t.Cleanup(func() {
		writeBootstrapPIDFileFunc = origWrite
		terminateBackgroundProcessFunc = origTerminate
	})

	writeBootstrapPIDFileFunc = func(string, int) error {
		return errors.New("write failed")
	}
	terminateBackgroundProcessFunc = func(*os.Process) error {
		return errors.New("cleanup failed")
	}

	err := persistBackgroundProcess("gateway", &os.Process{Pid: 444})
	if err == nil {
		t.Fatal("expected combined error")
	}
	if !strings.Contains(err.Error(), "cleanup failed") {
		t.Fatalf("expected cleanup context, got %v", err)
	}
}
