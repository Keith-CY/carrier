package gateway

import (
	"context"
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
