package gateway

import (
	"strings"
	"testing"
	"time"
)

func TestParseInput_ValidCommands(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantProvider string
		wantChatID   string
		wantReqID    string
		wantCmd      CommandName
		wantArgs     []string
		wantSession  string
	}{
		{
			name:         "pair command",
			input:        "telegram 123 req-1 /pair abc123",
			wantProvider: "telegram", wantChatID: "123", wantReqID: "req-1",
			wantCmd: CmdPair, wantArgs: []string{"abc123"},
		},
		{
			name:         "chat command with session token",
			input:        "telegram 123 req-chat session-abc /chat hello from terminal",
			wantProvider: "telegram", wantChatID: "123", wantReqID: "req-chat",
			wantCmd: CmdChat, wantArgs: []string{"hello", "from", "terminal"}, wantSession: "session-abc",
		},
		{
			name:         "agents command",
			input:        "discord ch-1 req-2 /agents",
			wantProvider: "discord", wantChatID: "ch-1", wantReqID: "req-2",
			wantCmd: CmdAgents, wantArgs: []string{},
		},
		{
			name:         "add with session token",
			input:        "feishu chat-abc req-3 session-xyz123 /add myagent",
			wantProvider: "feishu", wantChatID: "chat-abc", wantReqID: "req-3",
			wantCmd: CmdAdd, wantArgs: []string{"myagent"}, wantSession: "session-xyz123",
		},
		{
			name:         "uninstall with session token",
			input:        "feishu chat-abc req-3b session-xyz123 /uninstall myagent",
			wantProvider: "feishu", wantChatID: "chat-abc", wantReqID: "req-3b",
			wantCmd: CmdUninstall, wantArgs: []string{"myagent"}, wantSession: "session-xyz123",
		},
		{
			name:         "logs with agent and tail",
			input:        "telegram 789 req-4 session-tok /logs myagent 100",
			wantProvider: "telegram", wantChatID: "789", wantReqID: "req-4",
			wantCmd: CmdLogs, wantArgs: []string{"myagent", "100"}, wantSession: "session-tok",
		},
		{
			name:         "diagnose-consent",
			input:        "discord ch r1 session-s1 /diagnose-consent agent1 yes",
			wantProvider: "discord", wantChatID: "ch", wantReqID: "r1",
			wantCmd: CmdDiagnoseConsent, wantArgs: []string{"agent1", "yes"}, wantSession: "session-s1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd, err := ParseInput(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cmd.Provider != tc.wantProvider {
				t.Errorf("provider: got %q, want %q", cmd.Provider, tc.wantProvider)
			}
			if cmd.ChatID != tc.wantChatID {
				t.Errorf("chatID: got %q, want %q", cmd.ChatID, tc.wantChatID)
			}
			if cmd.RequestID != tc.wantReqID {
				t.Errorf("requestID: got %q, want %q", cmd.RequestID, tc.wantReqID)
			}
			if cmd.Name != tc.wantCmd {
				t.Errorf("name: got %q, want %q", cmd.Name, tc.wantCmd)
			}
			if cmd.SessionToken != tc.wantSession {
				t.Errorf("sessionToken: got %q, want %q", cmd.SessionToken, tc.wantSession)
			}
			if len(cmd.Args) != len(tc.wantArgs) {
				t.Errorf("args len: got %d, want %d (%v)", len(cmd.Args), len(tc.wantArgs), cmd.Args)
			} else {
				for i, a := range tc.wantArgs {
					if cmd.Args[i] != a {
						t.Errorf("args[%d]: got %q, want %q", i, cmd.Args[i], a)
					}
				}
			}
		})
	}
}

func TestParseInput_Errors(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{"too few fields", "telegram 123", "usage:"},
		{"unknown provider", "slack 123 req /pair code", "unknown provider"},
		{"unknown command", "telegram 123 req /fly", "unknown command"},
		{"session token without command", "telegram 123 req session-tok", "usage:"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseInput(tc.input)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			pe, ok := err.(*ParseError)
			if !ok {
				t.Fatalf("expected ParseError, got %T", err)
			}
			if !strings.Contains(pe.Err, tc.wantErr) {
				t.Errorf("error message %q does not contain %q", pe.Err, tc.wantErr)
			}
		})
	}
}

func TestInjectSessionToken(t *testing.T) {
	tests := []struct {
		input    string
		token    string
		expected string
	}{
		{"telegram 123 req /agents", "session-abc", "telegram 123 req session-abc /agents"},
		// Already has a non-command 4th token → no injection
		{"telegram 123 req session-existing /agents", "session-new", "telegram 123 req session-existing /agents"},
		{"telegram 123 req /agents arg1", "session-t", "telegram 123 req session-t /agents arg1"},
		{"telegram 123 req /agents", "", "telegram 123 req /agents"},
	}
	for _, tc := range tests {
		got := InjectSessionToken(tc.input, tc.token)
		if got != tc.expected {
			t.Errorf("InjectSessionToken(%q, %q)\n  got  %q\n  want %q", tc.input, tc.token, got, tc.expected)
		}
	}
}

func TestParsePositiveInt(t *testing.T) {
	tests := []struct {
		s        string
		fallback int
		want     int
	}{
		{"100", 200, 100},
		{"", 200, 200},
		{"abc", 200, 200},
		{"0", 200, 200},
		{"-5", 200, 200},
	}
	for _, tc := range tests {
		got := parsePositiveInt(tc.s, tc.fallback)
		if got != tc.want {
			t.Errorf("parsePositiveInt(%q, %d) = %d, want %d", tc.s, tc.fallback, got, tc.want)
		}
	}
}

func TestFormatUptime(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{45 * time.Second, "45s"},
		{90 * time.Second, "1m30s"},
		{3661 * time.Second, "1h1m1s"},
		{86400 * time.Second, "1d"},
		{90061 * time.Second, "1d1h1m1s"},
	}
	for _, tc := range tests {
		got := formatUptime(tc.d)
		if got != tc.want {
			t.Errorf("formatUptime(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestParseConsent(t *testing.T) {
	tests := []struct {
		input  string
		want   bool
		wantOK bool
	}{
		{"yes", true, true},
		{"YES", true, true},
		{"y", true, true},
		{"true", true, true},
		{"no", false, true},
		{"NO", false, true},
		{"n", false, true},
		{"false", false, true},
		{"maybe", false, false},
		{"", false, false},
	}
	for _, tc := range tests {
		got, ok := parseConsent(tc.input)
		if ok != tc.wantOK {
			t.Errorf("parseConsent(%q) ok=%v, want %v", tc.input, ok, tc.wantOK)
		}
		if ok && got != tc.want {
			t.Errorf("parseConsent(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}
