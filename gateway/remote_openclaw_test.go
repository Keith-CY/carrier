package gateway

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestExtractChatResponseText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload map[string]interface{}
		want    string
	}{
		{
			name:    "direct message",
			payload: map[string]interface{}{"message": "hello"},
			want:    "hello",
		},
		{
			name: "payload text array",
			payload: map[string]interface{}{
				"payloads": []interface{}{
					map[string]interface{}{"text": "hey BOSS 👋"},
				},
			},
			want: "hey BOSS 👋",
		},
		{
			name: "choices message content",
			payload: map[string]interface{}{
				"choices": []interface{}{
					map[string]interface{}{
						"message": map[string]interface{}{"content": "from choices"},
					},
				},
			},
			want: "from choices",
		},
		{
			name: "nested output content parts",
			payload: map[string]interface{}{
				"output": map[string]interface{}{
					"content": []interface{}{
						map[string]interface{}{"type": "text", "text": "line one"},
						map[string]interface{}{"type": "text", "text": "line two"},
					},
				},
			},
			want: "line one\nline two",
		},
		{
			name: "no chat text fields",
			payload: map[string]interface{}{
				"meta": map[string]interface{}{"model": "gpt-5.3-codex"},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractChatResponseText(tt.payload)
			if got != tt.want {
				t.Fatalf("extractChatResponseText()=%q want %q", got, tt.want)
			}
		})
	}
}

func TestParseRemoteHostPlatform(t *testing.T) {
	t.Parallel()

	got := parseRemoteHostPlatform("CARRIER_PLATFORM_PROBE\nOS=Linux\nDISTRO=ubuntu\nVERSION=22.04\n")
	if got.OS != "Linux" {
		t.Fatalf("os=%q, want Linux", got.OS)
	}
	if got.Distro != "ubuntu" {
		t.Fatalf("distro=%q, want ubuntu", got.Distro)
	}
	if got.Version != "22.04" {
		t.Fatalf("version=%q, want 22.04", got.Version)
	}
}

func TestDetectRemoteHostPlatformRejectsAlpine(t *testing.T) {
	configureSSHRunner(t, func(command string) remoteExecResult {
		switch {
		case strings.Contains(command, "CARRIER_PLATFORM_PROBE"):
			return remoteExecResult{
				ExitCode: 0,
				Stdout:   "CARRIER_PLATFORM_PROBE\nOS=Linux\nDISTRO=alpine\nVERSION=3.19\n",
			}
		default:
			return remoteExecResult{ExitCode: 0}
		}
	})

	platform, _, err := detectRemoteHostPlatform(context.Background(), RemoteHost{
		ID:      "h1",
		Name:    "h1",
		Host:    "127.0.0.1",
		Port:    22,
		User:    "carrier",
		KeyPath: "~/.ssh/id_ed25519",
	})
	if err != nil {
		t.Fatalf("detectRemoteHostPlatform() error: %v", err)
	}
	if platform.Supported {
		t.Fatalf("expected unsupported platform for alpine, got %+v", platform)
	}
	if !strings.Contains(strings.ToLower(platform.Reason), "alpine") {
		t.Fatalf("expected alpine reason, got %q", platform.Reason)
	}
}

func TestWrapRemoteCommandWithMemoryContractPreservesHomeExpansion(t *testing.T) {
	t.Parallel()

	contract := remoteMemoryRuntimeContract{
		ContractID:     "cid",
		ContractDigest: "digest",
		MemoryPath:     "$HOME/.openclaw/agents/main/memory",
	}
	got := wrapRemoteCommandWithMemoryContract("echo ok", contract)
	if !strings.Contains(got, `mem_path="$HOME"/'.openclaw/agents/main/memory'`) {
		t.Fatalf("expected HOME expansion-safe assignment, got: %s", got)
	}
	if strings.Contains(got, `mem_path='$HOME/.openclaw/agents/main/memory'`) {
		t.Fatalf("expected not to single-quote full $HOME path, got: %s", got)
	}
}

func TestShellPathValuePreservingHomeExpansion(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: "''"},
		{name: "whitespace", in: "  ", want: "''"},
		{name: "home only", in: "$HOME", want: `"$HOME"`},
		{name: "home with slash", in: "$HOME/", want: `"$HOME"`},
		{name: "home subpath", in: "$HOME/.picoclaw/memory", want: `"$HOME"/'.picoclaw/memory'`},
		{name: "home subpath with single quote", in: "$HOME/foo'bar", want: `"$HOME"/'foo'\''bar'`},
		{name: "plain path", in: "/tmp/memory path", want: `'/tmp/memory path'`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := shellPathValuePreservingHomeExpansion(tc.in); got != tc.want {
				t.Fatalf("shellPathValuePreservingHomeExpansion(%q)=%q want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRunRemoteRsyncBuildsSSHTransportForPrivateKeyHost(t *testing.T) {
	orig := remoteRsyncRunner
	defer func() { remoteRsyncRunner = orig }()

	var captured []string
	remoteRsyncRunner = func(_ context.Context, args []string) (remoteExecResult, error) {
		captured = append([]string{}, args...)
		return remoteExecResult{ExitCode: 0}, nil
	}

	host := RemoteHost{
		ID:       "h1",
		Name:     "h1",
		Host:     "198.51.100.10",
		Port:     2222,
		User:     "ubuntu",
		AuthMode: RemoteAuthModePrivateKey,
		KeyPath:  "~/.ssh/id_ed25519",
	}
	if _, err := runRemoteRsync(context.Background(), host, "/tmp/local-secrets.json", remoteOpenClawCarrierSecretsPath); err != nil {
		t.Fatalf("runRemoteRsync error: %v", err)
	}
	if len(captured) == 0 {
		t.Fatal("expected rsync args to be captured")
	}
	joined := strings.Join(captured, " ")
	if !strings.Contains(joined, "--rsync-path") || !strings.Contains(joined, "mkdir -p") {
		t.Fatalf("expected --rsync-path mkdir command in rsync args, got %q", joined)
	}
	if !strings.Contains(joined, "ubuntu@198.51.100.10:"+remoteOpenClawCarrierSecretsPath) {
		t.Fatalf("expected destination target in rsync args, got %q", joined)
	}
	if !strings.Contains(joined, "IdentitiesOnly=yes") || !strings.Contains(joined, "id_ed25519") {
		t.Fatalf("expected private key ssh options in rsync args, got %q", joined)
	}
}

func TestRemoteWriteOpenClawCarrierSecretsUsesRsyncPayload(t *testing.T) {
	orig := remoteRsyncRunner
	defer func() { remoteRsyncRunner = orig }()

	var (
		capturedArgs []string
		payload      map[string]interface{}
	)
	remoteRsyncRunner = func(_ context.Context, args []string) (remoteExecResult, error) {
		capturedArgs = append([]string{}, args...)
		if len(args) < 2 {
			t.Fatalf("unexpected rsync args: %v", args)
		}
		raw, err := os.ReadFile(args[len(args)-2])
		if err != nil {
			t.Fatalf("read rsync source file: %v", err)
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("parse rsync payload json: %v", err)
		}
		return remoteExecResult{ExitCode: 0}, nil
	}

	host := RemoteHost{
		ID:            "h2",
		Name:          "h2",
		Host:          "203.0.113.5",
		User:          "carrier",
		AuthMode:      RemoteAuthModeSSHConfig,
		SSHConfigHost: "carrier-h2",
	}
	input := map[string]interface{}{
		"providers": map[string]interface{}{
			"openai": map[string]interface{}{
				"apiKey": "sk-openai-rsync",
			},
		},
	}
	res, err := remoteWriteOpenClawCarrierSecrets(context.Background(), host, input)
	if err != nil {
		t.Fatalf("remoteWriteOpenClawCarrierSecrets error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %+v", res)
	}
	if len(capturedArgs) == 0 {
		t.Fatal("expected rsync invocation")
	}
	dest := capturedArgs[len(capturedArgs)-1]
	if !strings.HasSuffix(dest, ":"+remoteOpenClawCarrierSecretsPath) {
		t.Fatalf("unexpected rsync destination: %q", dest)
	}
	providers, _ := payload["providers"].(map[string]interface{})
	openai, _ := providers["openai"].(map[string]interface{})
	if strings.TrimSpace(anyToString(openai["apiKey"])) != "sk-openai-rsync" {
		t.Fatalf("unexpected secrets payload: %+v", payload)
	}
}
