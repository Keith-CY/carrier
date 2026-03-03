package gateway

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRemoteCommandRepairsKnownHostMismatch(t *testing.T) {
	origRunner := sshExecRunner
	origRepairer := knownHostsRepairer
	defer func() {
		sshExecRunner = origRunner
		knownHostsRepairer = origRepairer
	}()

	attempts := 0
	sshExecRunner = func(_ context.Context, _ []string) (remoteExecResult, error) {
		attempts++
		if attempts == 1 {
			return remoteExecResult{
				ExitCode: 255,
				Stderr:   "WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!\nOffending ECDSA key in /Users/test/.ssh/known_hosts:119\nHost key verification failed.",
			}, nil
		}
		return remoteExecResult{ExitCode: 0, Stdout: "ok"}, nil
	}

	repairCalls := 0
	knownHostsRepairer = func(_ RemoteHost) error {
		repairCalls++
		return nil
	}

	result, err := runRemoteCommand(context.Background(), RemoteHost{
		Host:     "127.0.0.1",
		Port:     2222,
		User:     "carrier",
		AuthMode: RemoteAuthModePrivateKey,
		KeyPath:  "/tmp/id_ed25519",
	}, "echo carrier-ssh-ok")
	if err != nil {
		t.Fatalf("runRemoteCommand returned error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0 after retry, got %d", result.ExitCode)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 ssh attempts, got %d", attempts)
	}
	if repairCalls != 1 {
		t.Fatalf("expected 1 known_hosts repair call, got %d", repairCalls)
	}
}

func TestRunRemoteCommandDoesNotRepairForNonHostKeyErrors(t *testing.T) {
	origRunner := sshExecRunner
	origRepairer := knownHostsRepairer
	defer func() {
		sshExecRunner = origRunner
		knownHostsRepairer = origRepairer
	}()

	attempts := 0
	sshExecRunner = func(_ context.Context, _ []string) (remoteExecResult, error) {
		attempts++
		return remoteExecResult{ExitCode: 255, Stderr: "Permission denied (publickey)."}, nil
	}

	repairCalls := 0
	knownHostsRepairer = func(_ RemoteHost) error {
		repairCalls++
		return nil
	}

	result, err := runRemoteCommand(context.Background(), RemoteHost{
		Host:     "127.0.0.1",
		Port:     2222,
		User:     "carrier",
		AuthMode: RemoteAuthModePrivateKey,
		KeyPath:  "/tmp/id_ed25519",
	}, "echo carrier-ssh-ok")
	if err != nil {
		t.Fatalf("runRemoteCommand returned error: %v", err)
	}
	if result.ExitCode != 255 {
		t.Fatalf("expected exit code 255, got %d", result.ExitCode)
	}
	if attempts != 1 {
		t.Fatalf("expected 1 ssh attempt, got %d", attempts)
	}
	if repairCalls != 0 {
		t.Fatalf("expected 0 known_hosts repair calls, got %d", repairCalls)
	}
}

func TestRunRemoteCommandStreamRepairsKnownHostMismatch(t *testing.T) {
	origRunner := sshExecStreamRunner
	origRepairer := knownHostsRepairer
	defer func() {
		sshExecStreamRunner = origRunner
		knownHostsRepairer = origRepairer
	}()

	attempts := 0
	sshExecStreamRunner = func(_ context.Context, _ []string, _ func(remoteStreamChunk)) (remoteExecResult, error) {
		attempts++
		if attempts == 1 {
			return remoteExecResult{
				ExitCode: 255,
				Stderr:   "WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!\nOffending ECDSA key in /Users/test/.ssh/known_hosts:119\nHost key verification failed.",
			}, nil
		}
		return remoteExecResult{ExitCode: 0, Stdout: "stream-ok"}, nil
	}

	repairCalls := 0
	knownHostsRepairer = func(_ RemoteHost) error {
		repairCalls++
		return nil
	}

	result, err := runRemoteCommandStream(context.Background(), RemoteHost{
		Host:     "127.0.0.1",
		Port:     2222,
		User:     "carrier",
		AuthMode: RemoteAuthModePrivateKey,
		KeyPath:  "/tmp/id_ed25519",
	}, "echo carrier-ssh-ok", nil)
	if err != nil {
		t.Fatalf("runRemoteCommandStream returned error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0 after retry, got %d", result.ExitCode)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 ssh attempts, got %d", attempts)
	}
	if repairCalls != 1 {
		t.Fatalf("expected 1 known_hosts repair call, got %d", repairCalls)
	}
}

func TestShouldRepairKnownHostMismatch(t *testing.T) {
	if !shouldRepairKnownHostMismatch(remoteExecResult{
		ExitCode: 255,
		Stderr:   "WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!",
	}, nil) {
		t.Fatalf("expected known host mismatch to be repairable")
	}
	if shouldRepairKnownHostMismatch(remoteExecResult{
		ExitCode: 255,
		Stderr:   "Permission denied (publickey).",
	}, nil) {
		t.Fatalf("did not expect non-host-key failure to trigger repair")
	}
}

func TestRepairKnownHostEntriesReturnsNilWhenAnyEntryRepaired(t *testing.T) {
	origRemover := knownHostEntryRemover
	defer func() { knownHostEntryRemover = origRemover }()

	knownHostEntryRemover = func(entry string) (bool, error) {
		if strings.HasPrefix(entry, "[") {
			return false, fmt.Errorf("simulated removal failure for %s", entry)
		}
		return true, nil
	}

	err := repairKnownHostEntries(RemoteHost{
		Host:     "127.0.0.1",
		Port:     2222,
		User:     "carrier",
		AuthMode: RemoteAuthModePrivateKey,
		KeyPath:  "/tmp/id_ed25519",
	})
	if err != nil {
		t.Fatalf("expected nil error when at least one entry is repaired, got %v", err)
	}
}

func TestRepairKnownHostEntriesReturnsErrorWhenAllEntriesFail(t *testing.T) {
	origRemover := knownHostEntryRemover
	defer func() { knownHostEntryRemover = origRemover }()

	knownHostEntryRemover = func(entry string) (bool, error) {
		return false, fmt.Errorf("simulated removal failure for %s", entry)
	}

	err := repairKnownHostEntries(RemoteHost{
		Host:     "127.0.0.1",
		Port:     2222,
		User:     "carrier",
		AuthMode: RemoteAuthModePrivateKey,
		KeyPath:  "/tmp/id_ed25519",
	})
	if err == nil {
		t.Fatalf("expected error when all known_hosts removals fail")
	}
}

func TestRunRemoteCommandWrapsCommandForStableEnv(t *testing.T) {
	origRunner := sshExecRunner
	defer func() { sshExecRunner = origRunner }()

	var gotArgs []string
	sshExecRunner = func(_ context.Context, args []string) (remoteExecResult, error) {
		gotArgs = append([]string{}, args...)
		return remoteExecResult{ExitCode: 0}, nil
	}

	_, err := runRemoteCommand(context.Background(), RemoteHost{
		Host:     "127.0.0.1",
		Port:     2222,
		User:     "carrier",
		AuthMode: RemoteAuthModePrivateKey,
		KeyPath:  "/tmp/id_ed25519",
	}, "echo carrier-ssh-ok")
	if err != nil {
		t.Fatalf("runRemoteCommand returned error: %v", err)
	}
	if len(gotArgs) == 0 {
		t.Fatalf("expected ssh args to be captured")
	}
	last := gotArgs[len(gotArgs)-1]
	if !strings.Contains(last, "bash -lc") {
		t.Fatalf("expected wrapped bash -lc command, got %q", last)
	}
	if !strings.Contains(last, "export LC_ALL=C LANG=C") {
		t.Fatalf("expected locale export in command wrapper, got %q", last)
	}
	if !strings.Contains(last, "export PATH=\"$HOME/.npm-global/bin:$HOME/.local/bin:$PATH\"") {
		t.Fatalf("expected PATH export in command wrapper, got %q", last)
	}
	if !strings.Contains(last, "echo carrier-ssh-ok") {
		t.Fatalf("expected original command in wrapper, got %q", last)
	}
}

func TestBuildSSHArgsUsesKeyRefWhenKeyPathMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CARRIER_REMOTE_KEY_DIR", tmp)
	keyRef := "aaaaaaaa"
	keyPath := filepath.Join(tmp, keyRef+".pem")
	if err := os.WriteFile(keyPath, []byte("test"), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	args, err := buildSSHArgs(RemoteHost{
		Host:     "127.0.0.1",
		Port:     2222,
		User:     "carrier",
		AuthMode: RemoteAuthModePrivateKey,
		KeyRef:   keyRef,
	}, "echo carrier-ssh-ok")
	if err != nil {
		t.Fatalf("buildSSHArgs returned error: %v", err)
	}
	found := false
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-i" && filepath.Clean(args[i+1]) == filepath.Clean(keyPath) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected -i to use keyRef-resolved path %s, got args=%v", keyPath, args)
	}
}

func TestRunRemoteCommandWithRetryRetriesTransientFailures(t *testing.T) {
	orig := sshExecRunner
	defer func() {
		sshExecRunner = orig
	}()

	attempts := 0
	sshExecRunner = func(_ context.Context, _ []string) (remoteExecResult, error) {
		attempts++
		if attempts == 1 {
			return remoteExecResult{ExitCode: 1, Stderr: "connection reset by peer"}, nil
		}
		return remoteExecResult{ExitCode: 0}, nil
	}

	res, err := runRemoteCommandWithRetry(context.Background(), RemoteHost{
		Host:     "127.0.0.1",
		Port:     22,
		User:     "carrier",
		AuthMode: RemoteAuthModePrivateKey,
		KeyPath:  "/tmp/id_ed25519",
	}, "echo ok", 1)
	if err != nil {
		t.Fatalf("runRemoteCommandWithRetry()=%v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts with retry, got %d", attempts)
	}
	if res.ExitCode != 0 {
		t.Fatalf("expected successful exit code after retry, got %d", res.ExitCode)
	}
}

func TestRunRemoteCommandWithRetryStopsOnNonRetryableFailure(t *testing.T) {
	orig := sshExecRunner
	defer func() {
		sshExecRunner = orig
	}()

	attempts := 0
	sshExecRunner = func(_ context.Context, _ []string) (remoteExecResult, error) {
		attempts++
		return remoteExecResult{ExitCode: 1, Stderr: "permission denied"}, fmt.Errorf("ssh command failed")
	}

	res, err := runRemoteCommandWithRetry(context.Background(), RemoteHost{
		Host:     "127.0.0.1",
		Port:     22,
		User:     "carrier",
		AuthMode: RemoteAuthModePrivateKey,
		KeyPath:  "/tmp/id_ed25519",
	}, "echo nope", 3)
	if err == nil {
		t.Fatal("expected error from non-retryable remote command")
	}
	if attempts != 1 {
		t.Fatalf("expected single attempt for non-retryable failure, got %d", attempts)
	}
	if res.ExitCode != 1 {
		t.Fatalf("expected exit code from failed command, got %d", res.ExitCode)
	}
}

func TestEnsureSSHProcessEnvAddsHomeWhenMissing(t *testing.T) {
	t.Setenv("HOME", "/tmp/carrier-home")
	got := ensureSSHProcessEnv([]string{"PATH=/usr/bin"})
	home, ok := lookupEnvValue(got, "HOME")
	if !ok {
		t.Fatalf("expected HOME to be added")
	}
	if home != "/tmp/carrier-home" {
		t.Fatalf("expected HOME=/tmp/carrier-home, got %q", home)
	}
}

func TestEnsureSSHProcessEnvPreservesExistingHome(t *testing.T) {
	got := ensureSSHProcessEnv([]string{"HOME=/already-set", "PATH=/usr/bin"})
	home, ok := lookupEnvValue(got, "HOME")
	if !ok {
		t.Fatalf("expected HOME to exist")
	}
	if home != "/already-set" {
		t.Fatalf("expected HOME to stay /already-set, got %q", home)
	}
}

func lookupEnvValue(env []string, key string) (string, bool) {
	prefix := key + "="
	for _, entry := range env {
		if !strings.HasPrefix(entry, prefix) {
			continue
		}
		return strings.TrimPrefix(entry, prefix), true
	}
	return "", false
}
