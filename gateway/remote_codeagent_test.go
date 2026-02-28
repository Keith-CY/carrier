package gateway

import (
	"context"
	"strings"
	"testing"
)

func TestRemoteInstallCodeAgentBinaryUsesOpenCodeAIPackage(t *testing.T) {
	origRunner := sshExecRunner
	defer func() { sshExecRunner = origRunner }()

	var captured []string
	opencodeLookups := 0
	sshExecRunner = func(_ context.Context, args []string) (remoteExecResult, error) {
		captured = append(captured, args...)
		command := strings.Join(args, " ")
		switch {
		case strings.Contains(command, "command -v opencode"):
			opencodeLookups++
			if opencodeLookups == 1 {
				return remoteExecResult{ExitCode: 1}, nil
			}
			return remoteExecResult{ExitCode: 0}, nil
		case strings.Contains(command, "command -v bun"):
			return remoteExecResult{ExitCode: 0}, nil
		case strings.Contains(command, "command -v npm"):
			return remoteExecResult{ExitCode: 1}, nil
		case strings.Contains(command, "bun add -g opencode-ai"):
			return remoteExecResult{ExitCode: 0}, nil
		}
		return remoteExecResult{ExitCode: 0}, nil
	}

	err := remoteInstallCodeAgentBinary(context.Background(), RemoteHost{
		Host:     "127.0.0.1",
		Port:     2222,
		User:     "carrier",
		AuthMode: RemoteAuthModePrivateKey,
		KeyPath:  "/tmp/id_ed25519",
	}, "opencode")
	if err != nil {
		t.Fatalf("remoteInstallCodeAgentBinary returned error: %v", err)
	}
	if len(captured) == 0 {
		t.Fatalf("expected ssh command to be captured")
	}
	full := strings.Join(captured, " ")
	if !strings.Contains(full, "bun add -g opencode-ai") {
		t.Fatalf("expected deterministic bun install command, got %q", full)
	}
}

func TestRemoteInstallCodeAgentBinaryUsesCodexPackageWithBun(t *testing.T) {
	origRunner := sshExecRunner
	defer func() { sshExecRunner = origRunner }()

	var captured []string
	codexLookups := 0
	sshExecRunner = func(_ context.Context, args []string) (remoteExecResult, error) {
		captured = append(captured, args...)
		command := strings.Join(args, " ")
		switch {
		case strings.Contains(command, "command -v codex"):
			codexLookups++
			if codexLookups == 1 {
				return remoteExecResult{ExitCode: 1}, nil
			}
			return remoteExecResult{ExitCode: 0}, nil
		case strings.Contains(command, "command -v bun"):
			return remoteExecResult{ExitCode: 0}, nil
		case strings.Contains(command, "command -v npm"):
			return remoteExecResult{ExitCode: 1}, nil
		case strings.Contains(command, "bun add -g @openai/codex"):
			return remoteExecResult{ExitCode: 0}, nil
		}
		return remoteExecResult{ExitCode: 0}, nil
	}

	err := remoteInstallCodeAgentBinary(context.Background(), RemoteHost{
		Host:     "127.0.0.1",
		Port:     2222,
		User:     "carrier",
		AuthMode: RemoteAuthModePrivateKey,
		KeyPath:  "/tmp/id_ed25519",
	}, "codex")
	if err != nil {
		t.Fatalf("remoteInstallCodeAgentBinary returned error: %v", err)
	}
	if len(captured) == 0 {
		t.Fatalf("expected ssh command to be captured")
	}
	full := strings.Join(captured, " ")
	if !strings.Contains(full, "bun add -g @openai/codex") {
		t.Fatalf("expected deterministic bun codex install command, got %q", full)
	}
}

func TestRemoteInstallCodeAgentBinaryFailsWhenNoSupportedInstaller(t *testing.T) {
	origRunner := sshExecRunner
	defer func() { sshExecRunner = origRunner }()

	sshExecRunner = func(_ context.Context, args []string) (remoteExecResult, error) {
		command := strings.Join(args, " ")
		switch {
		case strings.Contains(command, "command -v codex"):
			return remoteExecResult{ExitCode: 1}, nil
		case strings.Contains(command, "command -v bun"):
			return remoteExecResult{ExitCode: 1}, nil
		case strings.Contains(command, "command -v npm"):
			return remoteExecResult{ExitCode: 1}, nil
		default:
			return remoteExecResult{ExitCode: 1}, nil
		}
	}

	err := remoteInstallCodeAgentBinary(context.Background(), RemoteHost{
		Host:     "127.0.0.1",
		Port:     2222,
		User:     "carrier",
		AuthMode: RemoteAuthModePrivateKey,
		KeyPath:  "/tmp/id_ed25519",
	}, "codex")
	if err == nil {
		t.Fatal("expected install failure without bun/npm")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "neither bun nor npm") {
		t.Fatalf("unexpected error: %v", err)
	}
}
