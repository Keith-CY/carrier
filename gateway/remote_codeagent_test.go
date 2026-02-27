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
	sshExecRunner = func(_ context.Context, args []string) (remoteExecResult, error) {
		captured = append(captured, args...)
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
	command := captured[len(captured)-1]
	if !strings.Contains(command, "npm install -g opencode-ai") {
		t.Fatalf("expected opencode-ai install command, got %q", command)
	}
}
