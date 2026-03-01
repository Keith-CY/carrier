package gateway

import (
	"context"
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
