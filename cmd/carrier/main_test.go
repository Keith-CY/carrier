package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeDefaultProviderConfigForTest(t *testing.T, providerID, modelID string) {
	t.Helper()

	cfgPath := filepath.Join(t.TempDir(), "config.v2.json")
	t.Setenv("CARRIER_CONFIG", cfgPath)
	payload := map[string]interface{}{
		"config_version": 2,
		"default_model":  providerID + "-default",
		"model_list": []map[string]string{
			{
				"model_name":  providerID + "-default",
				"model":       modelID,
				"provider_id": providerID,
			},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal test config: %v", err)
	}
	if err := os.WriteFile(cfgPath, raw, 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}
}

func TestPickMinimalProviderWithReasonUsesConfigDefault(t *testing.T) {
	writeDefaultProviderConfigForTest(t, "openai", "openai/gpt-5.2")

	provider, reason, err := pickMinimalProviderWithReason()
	if err != nil {
		t.Fatalf("pickMinimalProviderWithReason error: %v", err)
	}
	if provider.ID != "openai" {
		t.Fatalf("provider.ID = %q, want %q", provider.ID, "openai")
	}
	if !strings.Contains(reason, "config.v2") {
		t.Fatalf("reason should include config-default context, got %q", reason)
	}
}

func TestPickMinimalProviderWithReasonFallsBackToOpenAICodex(t *testing.T) {
	t.Setenv("CARRIER_CONFIG", filepath.Join(t.TempDir(), "missing-config.v2.json"))

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

func TestResolveChoice_ProviderAlias(t *testing.T) {
	opt, ok := resolveChoice("vllm", providerOptions)
	if !ok {
		t.Fatal("expected vllm alias to resolve via providerOptions")
	}
	if opt.ID != "openai-compatible" {
		t.Fatalf("resolveChoice(vllm).ID = %q, want openai-compatible", opt.ID)
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

func TestResolveManagedAgentChannelUsesManagedChannelRegistry(t *testing.T) {
	for _, agentID := range []string{"picoclaw", "openclaw", "zeroclaw"} {
		t.Run(agentID, func(t *testing.T) {
			channels, ok := managedAgentChannels(agentID)
			if !ok || len(channels) == 0 {
				t.Fatalf("expected channel registry for %s", agentID)
			}

			channel, ok := resolveManagedAgentChannel(agentID)
			if !ok {
				t.Fatalf("expected managed channel resolution to succeed for %s", agentID)
			}
			if channel.ID != channels[0].ID {
				t.Fatalf("channel.ID = %q, want %q", channel.ID, channels[0].ID)
			}
		})
	}
}

func TestResolveManagedAgentChannelRejectsUnknownAgent(t *testing.T) {
	if _, ok := resolveManagedAgentChannel("unknown-agent"); ok {
		t.Fatal("expected unknown managed agent channel resolution to fail")
	}
}

func TestPromptManagedChannelSelectionSupportsWebUIOnly(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("\n"))
	var out bytes.Buffer

	channel, hasChannel, err := promptManagedChannelSelection(reader, &out, "openclaw", false)
	if err != nil {
		t.Fatalf("promptManagedChannelSelection: %v", err)
	}
	if hasChannel {
		t.Fatalf("hasChannel = true, want false (WebUI-only)")
	}
	if channel.ID != "" {
		t.Fatalf("channel.ID = %q, want empty", channel.ID)
	}
	if !strings.Contains(out.String(), "WebUI-only mode selected.") {
		t.Fatalf("output missing WebUI-only hint: %q", out.String())
	}
}

func TestPromptManagedChannelSelectionQuietFallsBackToWebUIOnly(t *testing.T) {
	t.Setenv("CARRIER_TELEGRAM_BOT_TOKEN", "")
	t.Setenv("CARRIER_CONFIG", filepath.Join(t.TempDir(), "missing-config.v2.json"))
	var out bytes.Buffer

	channel, hasChannel, err := promptManagedChannelSelection(bufio.NewReader(strings.NewReader("")), &out, "openclaw", true)
	if err != nil {
		t.Fatalf("promptManagedChannelSelection: %v", err)
	}
	if hasChannel {
		t.Fatalf("hasChannel = true, want false in quiet mode without token")
	}
	if channel.ID != "" {
		t.Fatalf("channel.ID = %q, want empty", channel.ID)
	}
	if !strings.Contains(out.String(), "quiet mode") {
		t.Fatalf("output missing quiet mode message: %q", out.String())
	}
}

func TestResolveManagedChannelTokenPrefersEnvironment(t *testing.T) {
	t.Setenv("CARRIER_TELEGRAM_BOT_TOKEN", "tg-env-token")

	token, source := resolveManagedChannelToken("telegram")
	if token != "tg-env-token" {
		t.Fatalf("token = %q, want %q", token, "tg-env-token")
	}
	if !strings.Contains(source, "CARRIER_TELEGRAM_BOT_TOKEN") {
		t.Fatalf("source should mention token env var, got %q", source)
	}
}

func TestResolveManagedChannelTokenFallsBackToConfig(t *testing.T) {
	t.Setenv("CARRIER_TELEGRAM_BOT_TOKEN", "")
	cfgPath := filepath.Join(t.TempDir(), "config.v2.json")
	t.Setenv("CARRIER_CONFIG", cfgPath)

	raw, err := json.Marshal(map[string]interface{}{
		"config_version": 2,
		"channels": []map[string]interface{}{
			{
				"id":        "telegram",
				"bot_token": "tg-config-token",
				"enabled":   true,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(cfgPath, raw, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	token, source := resolveManagedChannelToken("telegram")
	if token != "tg-config-token" {
		t.Fatalf("token = %q, want %q", token, "tg-config-token")
	}
	if !strings.Contains(source, "config") {
		t.Fatalf("source should mention config fallback, got %q", source)
	}
}

func TestManagedAddReusesChannelTokenPolicy(t *testing.T) {
	cases := []struct {
		name      string
		agentID   string
		channelID string
		want      bool
	}{
		{name: "openclaw telegram", agentID: "openclaw", channelID: "telegram", want: false},
		{name: "picoclaw telegram", agentID: "picoclaw", channelID: "telegram", want: true},
		{name: "zeroclaw telegram", agentID: "zeroclaw", channelID: "telegram", want: true},
		{name: "openclaw discord", agentID: "openclaw", channelID: "discord", want: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := managedAddReusesChannelToken(tc.agentID, tc.channelID)
			if got != tc.want {
				t.Fatalf("managedAddReusesChannelToken(%q, %q) = %v, want %v", tc.agentID, tc.channelID, got, tc.want)
			}
		})
	}
}

func TestEnsureWindowsWSLPrereqForOpenClawSkipsNonOpenClaw(t *testing.T) {
	origGOOS := carrierGOOS
	origLister := windowsWSLListDistros
	t.Cleanup(func() {
		carrierGOOS = origGOOS
		windowsWSLListDistros = origLister
	})

	carrierGOOS = "windows"
	windowsWSLListDistros = func() ([]string, error) {
		t.Fatal("windowsWSLListDistros should not be called for non-openclaw agent")
		return nil, nil
	}

	if err := ensureWindowsWSLPrereqForOpenClaw("picoclaw"); err != nil {
		t.Fatalf("ensureWindowsWSLPrereqForOpenClaw(picoclaw) error: %v", err)
	}
}

func TestEnsureWindowsWSLPrereqForOpenClawSkipsNonWindows(t *testing.T) {
	origGOOS := carrierGOOS
	origLister := windowsWSLListDistros
	t.Cleanup(func() {
		carrierGOOS = origGOOS
		windowsWSLListDistros = origLister
	})

	carrierGOOS = "linux"
	windowsWSLListDistros = func() ([]string, error) {
		t.Fatal("windowsWSLListDistros should not be called on non-windows")
		return nil, nil
	}

	if err := ensureWindowsWSLPrereqForOpenClaw("openclaw"); err != nil {
		t.Fatalf("ensureWindowsWSLPrereqForOpenClaw(openclaw) error: %v", err)
	}
}

func TestEnsureWindowsWSLPrereqForOpenClawRejectsMissingDistros(t *testing.T) {
	origGOOS := carrierGOOS
	origLister := windowsWSLListDistros
	t.Cleanup(func() {
		carrierGOOS = origGOOS
		windowsWSLListDistros = origLister
	})

	carrierGOOS = "windows"
	windowsWSLListDistros = func() ([]string, error) {
		return nil, nil
	}

	err := ensureWindowsWSLPrereqForOpenClaw("openclaw")
	if err == nil {
		t.Fatal("expected missing WSL distro error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "requires wsl2") {
		t.Fatalf("expected WSL2 requirement in error, got: %v", err)
	}
}

func TestEnsureWindowsWSLPrereqForOpenClawAcceptsAvailableDistro(t *testing.T) {
	origGOOS := carrierGOOS
	origLister := windowsWSLListDistros
	t.Cleanup(func() {
		carrierGOOS = origGOOS
		windowsWSLListDistros = origLister
	})

	carrierGOOS = "windows"
	windowsWSLListDistros = func() ([]string, error) {
		return []string{"Ubuntu-24.04"}, nil
	}

	if err := ensureWindowsWSLPrereqForOpenClaw("openclaw"); err != nil {
		t.Fatalf("ensureWindowsWSLPrereqForOpenClaw(openclaw) error: %v", err)
	}
}

func TestEnsureWindowsWSLPrereqForOpenClawPropagatesQueryError(t *testing.T) {
	origGOOS := carrierGOOS
	origLister := windowsWSLListDistros
	t.Cleanup(func() {
		carrierGOOS = origGOOS
		windowsWSLListDistros = origLister
	})

	carrierGOOS = "windows"
	windowsWSLListDistros = func() ([]string, error) {
		return nil, errors.New("wsl query failed")
	}

	err := ensureWindowsWSLPrereqForOpenClaw("openclaw")
	if err == nil {
		t.Fatal("expected query failure error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "failed to query wsl") {
		t.Fatalf("expected query failure context in error, got: %v", err)
	}
}

func TestParseAddCommandArgsSupportsQuietOptions(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want addCommandOptions
	}{
		{
			name: "quiet short flag after agent",
			args: []string{"openclaw", "-q"},
			want: addCommandOptions{AgentID: "openclaw", Quiet: true, TUI: true},
		},
		{
			name: "quiet long flag before agent",
			args: []string{"--quiet", "picoclaw"},
			want: addCommandOptions{AgentID: "picoclaw", Quiet: true, TUI: true},
		},
		{
			name: "quiet typo alias supported",
			args: []string{"zeroclaw", "--quite"},
			want: addCommandOptions{AgentID: "zeroclaw", Quiet: true, TUI: true},
		},
		{
			name: "combined webui and quiet flags",
			args: []string{"-q", "openclaw", "--webui"},
			want: addCommandOptions{AgentID: "openclaw", Quiet: true, WebUI: true},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseAddCommandArgs(tc.args)
			if err != nil {
				t.Fatalf("parseAddCommandArgs(%v) error: %v", tc.args, err)
			}
			if got != tc.want {
				t.Fatalf("parseAddCommandArgs(%v) = %+v, want %+v", tc.args, got, tc.want)
			}
		})
	}
}

func TestParseAddCommandArgsSupportsTerminalModeFlags(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want addCommandOptions
	}{
		{
			name: "explicit cli mode",
			args: []string{"openclaw", "--cli"},
			want: addCommandOptions{AgentID: "openclaw", CLI: true, TUI: true},
		},
		{
			name: "explicit tui mode",
			args: []string{"openclaw", "--tui"},
			want: addCommandOptions{AgentID: "openclaw", TUI: true},
		},
		{
			name: "default terminal mode",
			args: []string{"openclaw"},
			want: addCommandOptions{AgentID: "openclaw", TUI: true},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseAddCommandArgs(tc.args)
			if err != nil {
				t.Fatalf("parseAddCommandArgs(%v) error: %v", tc.args, err)
			}
			if got != tc.want {
				t.Fatalf("parseAddCommandArgs(%v) = %+v, want %+v", tc.args, got, tc.want)
			}
		})
	}

	if _, err := parseAddCommandArgs([]string{"openclaw", "--webui", "--cli"}); err == nil {
		t.Fatal("expected conflict error when combining --webui with --cli")
	}
}

func TestParseAddCommandArgsSupportsIsolationFlag(t *testing.T) {
	opts, err := parseAddCommandArgs([]string{"openclaw", "--isolation"})
	if err != nil {
		t.Fatalf("parseAddCommandArgs error: %v", err)
	}
	if !opts.Isolation {
		t.Fatalf("expected isolation=true, got %+v", opts)
	}
	if !opts.TUI {
		t.Fatalf("expected default tui mode, got %+v", opts)
	}
}

func TestParseOnboardCommandArgsSupportsTerminalModeFlags(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want onboardCommandOptions
	}{
		{
			name: "default terminal mode",
			args: nil,
			want: onboardCommandOptions{TUI: true},
		},
		{
			name: "explicit cli mode",
			args: []string{"--cli"},
			want: onboardCommandOptions{CLI: true, TUI: true},
		},
		{
			name: "explicit tui mode",
			args: []string{"--tui"},
			want: onboardCommandOptions{TUI: true},
		},
		{
			name: "webui mode",
			args: []string{"--webui"},
			want: onboardCommandOptions{WebUI: true},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseOnboardCommandArgs(tc.args)
			if err != nil {
				t.Fatalf("parseOnboardCommandArgs(%v) error: %v", tc.args, err)
			}
			if got != tc.want {
				t.Fatalf("parseOnboardCommandArgs(%v) = %+v, want %+v", tc.args, got, tc.want)
			}
		})
	}

	if _, err := parseOnboardCommandArgs([]string{"--webui", "--tui"}); err == nil {
		t.Fatal("expected conflict error when combining --webui with --tui")
	}
	if _, err := parseOnboardCommandArgs([]string{"--unknown"}); err == nil {
		t.Fatal("expected unknown option error")
	}
}

func TestPickManagedAddProviderWithReasonUsesLatestManagedInstanceProvider(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CARRIER_CONFIG", filepath.Join(t.TempDir(), "missing-config.v2.json"))
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
	t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))

	instancesPath, err := managedInstancesPath()
	if err != nil {
		t.Fatalf("managedInstancesPath: %v", err)
	}
	instances := []managedAgentInstance{
		{
			ID:        "openclaw-a",
			Type:      "openclaw",
			AgentID:   "openclaw",
			Provider:  "openai",
			UpdatedAt: time.Date(2026, 2, 1, 8, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		},
		{
			ID:        "openclaw-b",
			Type:      "openclaw",
			AgentID:   "openclaw",
			Provider:  "anthropic",
			UpdatedAt: time.Date(2026, 2, 2, 8, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		},
	}
	if err := saveManagedInstances(instancesPath, instances); err != nil {
		t.Fatalf("saveManagedInstances: %v", err)
	}

	provider, reason, err := pickManagedAddProviderWithReason("openclaw")
	if err != nil {
		t.Fatalf("pickManagedAddProviderWithReason error: %v", err)
	}
	if provider.ID != "anthropic" {
		t.Fatalf("provider.ID = %q, want %q", provider.ID, "anthropic")
	}
	if !strings.Contains(strings.ToLower(reason), "latest openclaw instance") {
		t.Fatalf("reason should mention latest managed instance reuse, got %q", reason)
	}
}

func TestPickManagedAddProviderWithReasonUsesLatestManagedInstanceAliasProvider(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CARRIER_CONFIG", filepath.Join(t.TempDir(), "missing-config.v2.json"))
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
	t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))

	instancesPath, err := managedInstancesPath()
	if err != nil {
		t.Fatalf("managedInstancesPath: %v", err)
	}
	instances := []managedAgentInstance{
		{
			ID:        "openclaw-legacy",
			Type:      "openclaw",
			AgentID:   "openclaw",
			Provider:  "claude-code",
			UpdatedAt: time.Date(2026, 2, 2, 8, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		},
	}
	if err := saveManagedInstances(instancesPath, instances); err != nil {
		t.Fatalf("saveManagedInstances: %v", err)
	}

	provider, reason, err := pickManagedAddProviderWithReason("openclaw")
	if err != nil {
		t.Fatalf("pickManagedAddProviderWithReason error: %v", err)
	}
	if provider.ID != "openai-codex" {
		t.Fatalf("provider.ID = %q, want %q", provider.ID, "openai-codex")
	}
	if !strings.Contains(strings.ToLower(reason), "latest openclaw instance") {
		t.Fatalf("reason should mention latest managed instance reuse, got %q", reason)
	}
}

func TestParseCarrierCommandRoutesVersionAliases(t *testing.T) {
	for _, command := range []string{"version", "--version", "-v", "-V"} {
		t.Run(command, func(t *testing.T) {
			cmd, args, err := parseCarrierCommand([]string{"carrier", command})
			if err != nil {
				t.Fatalf("parseCarrierCommand(%q) error: %v", command, err)
			}
			if cmd != "version" {
				t.Fatalf("parseCarrierCommand(%q) = %q, want version", command, cmd)
			}
			if len(args) != 0 {
				t.Fatalf("args = %v, want empty", args)
			}
		})
	}
}

func TestParseCarrierCommandRoutesUpdateCommand(t *testing.T) {
	cmd, args, err := parseCarrierCommand([]string{"carrier", "update", "--check"})
	if err != nil {
		t.Fatalf("parseCarrierCommand(update) error: %v", err)
	}
	if cmd != "update" {
		t.Fatalf("cmd = %q, want update", cmd)
	}
	if len(args) != 1 || args[0] != "--check" {
		t.Fatalf("args = %v, want [--check]", args)
	}
}

func TestParseUpdateCommandArgsDefaultsAndPriority(t *testing.T) {
	opts, err := parseUpdateCommandArgs(nil)
	if err != nil {
		t.Fatalf("parseUpdateCommandArgs(nil) error: %v", err)
	}
	if opts.Channel != "stable" {
		t.Fatalf("channel = %q, want stable", opts.Channel)
	}
	if opts.Timeout != defaultUpdateTimeout {
		t.Fatalf("timeout = %s, want %s", opts.Timeout, defaultUpdateTimeout)
	}

	opts, err = parseUpdateCommandArgs([]string{
		"--tag", "v9.9.9",
		"--channel", "beta",
	})
	if err != nil {
		t.Fatalf("parseUpdateCommandArgs with tag+channel error: %v", err)
	}
	if opts.Tag != "v9.9.9" {
		t.Fatalf("tag = %q, want %q", opts.Tag, "v9.9.9")
	}
	if opts.Channel != "beta" {
		t.Fatalf("channel = %q, want beta", opts.Channel)
	}
}

func TestParseUpdateCommandArgsRejectsInvalidInput(t *testing.T) {
	if _, err := parseUpdateCommandArgs([]string{"--timeout", "0"}); err == nil {
		t.Fatal("expected timeout validation failure")
	}
	if _, err := parseUpdateCommandArgs([]string{"--channel", "nightly"}); err == nil {
		t.Fatal("expected invalid channel validation failure")
	}
}

func TestResolveUpdateTargetPrefersTag(t *testing.T) {
	origGit := execGitCommand
	t.Cleanup(func() { execGitCommand = origGit })
	execGitCommand = func(_ context.Context, _ string, args ...string) (string, error) {
		switch strings.Join(args, " ") {
		case "tag --list v1.2.3":
			return "v1.2.3", nil
		default:
			t.Fatalf("unexpected git command: %v", args)
			return "", nil
		}
	}

	target, source, err := resolveUpdateTarget(10*time.Second, "/tmp", updateCommandOptions{
		Tag:     "v1.2.3",
		Channel: "beta",
	})
	if err != nil {
		t.Fatalf("resolveUpdateTarget error: %v", err)
	}
	if target != "v1.2.3" {
		t.Fatalf("target = %q, want v1.2.3", target)
	}
	if source != "tag" {
		t.Fatalf("source = %q, want tag", source)
	}
}

func TestResolveUpdateTargetResolvesStableAndBetaTags(t *testing.T) {
	origGit := execGitCommand
	t.Cleanup(func() { execGitCommand = origGit })
	execGitCommand = func(_ context.Context, _ string, args ...string) (string, error) {
		switch strings.Join(args, " ") {
		case "tag --list --sort=-creatordate":
			return "v2.0.0\nv2.0.0-beta.2\nv1.9.0", nil
		default:
			t.Fatalf("unexpected git command: %v", args)
			return "", nil
		}
	}

	target, source, err := resolveUpdateTarget(10*time.Second, "/tmp", updateCommandOptions{Channel: "beta"})
	if err != nil {
		t.Fatalf("resolveUpdateTarget(beta) error: %v", err)
	}
	if target != "v2.0.0-beta.2" {
		t.Fatalf("target(beta) = %q, want v2.0.0-beta.2", target)
	}
	if source != "channel beta" {
		t.Fatalf("source(beta) = %q, want channel beta", source)
	}

	target, source, err = resolveUpdateTarget(10*time.Second, "/tmp", updateCommandOptions{Channel: "stable"})
	if err != nil {
		t.Fatalf("resolveUpdateTarget(stable) error: %v", err)
	}
	if target != "v2.0.0" {
		t.Fatalf("target(stable) = %q, want v2.0.0", target)
	}
	if source != "channel stable" {
		t.Fatalf("source(stable) = %q, want channel stable", source)
	}
}

func TestRunVersionCommandJSON(t *testing.T) {
	var out bytes.Buffer
	if err := runVersionCommand(&out, versionCommandOptions{JSON: true}); err != nil {
		t.Fatalf("runVersionCommand error: %v", err)
	}
	var payload versionInfo
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal version output: %v", err)
	}
	if payload.Version == "" {
		t.Fatalf("payload.version is empty")
	}
	if payload.GoVersion == "" {
		t.Fatalf("payload.goVersion is empty")
	}
}

func TestRunVersionCommandText(t *testing.T) {
	var out bytes.Buffer
	if err := runVersionCommand(&out, versionCommandOptions{}); err != nil {
		t.Fatalf("runVersionCommand error: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "carrier") {
		t.Fatalf("version text should contain command name, got: %q", text)
	}
	if !strings.Contains(text, "go version:") {
		t.Fatalf("version text should contain go version, got: %q", text)
	}
}

func TestParseVersionCommandArgs(t *testing.T) {
	opts, err := parseVersionCommandArgs([]string{"--json"})
	if err != nil {
		t.Fatalf("parseVersionCommandArgs error: %v", err)
	}
	if !opts.JSON {
		t.Fatal("opts.JSON = false, want true")
	}
	if _, err := parseVersionCommandArgs([]string{"--bad"}); err == nil {
		t.Fatal("expected parse error for unknown version option")
	}
}

func TestRunUpdateJSONWithoutYesReturnsError(t *testing.T) {
	origGit := execGitCommand
	t.Cleanup(func() { execGitCommand = origGit })
	execGitCommand = func(_ context.Context, _ string, args ...string) (string, error) {
		switch {
		case len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel":
			return "/tmp/repo", nil
		case len(args) >= 1 && args[0] == "fetch":
			return "", nil
		case len(args) >= 2 && args[0] == "tag" && args[1] == "--list" && len(args) == 3:
			return args[2], nil
		case len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--abbrev-ref":
			return "main", nil
		case len(args) >= 2 && args[0] == "status" && args[1] == "--porcelain":
			return "", nil
		default:
			return "", nil
		}
	}

	var out bytes.Buffer
	err := runUpdate(
		strings.NewReader(""),
		&out,
		updateCommandOptions{
			Tag:     "v1.0.0",
			Channel: "stable",
			Timeout: 10 * time.Second,
			JSON:    true,
			// Yes is NOT set — should error
		},
	)
	if err == nil {
		t.Fatal("expected error when --json is used without --yes in apply mode")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("error should mention --yes, got: %v", err)
	}
}

func TestRunUpdateJSONWithYesProducesOnlyJSON(t *testing.T) {
	origGit := execGitCommand
	t.Cleanup(func() { execGitCommand = origGit })
	execGitCommand = func(_ context.Context, _ string, args ...string) (string, error) {
		switch {
		case len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel":
			return "/tmp/repo", nil
		case len(args) >= 1 && args[0] == "fetch":
			return "", nil
		case len(args) >= 2 && args[0] == "tag" && args[1] == "--list" && len(args) == 3:
			return args[2], nil
		case len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--abbrev-ref":
			return "main", nil
		case len(args) >= 2 && args[0] == "status" && args[1] == "--porcelain":
			return "", nil
		case len(args) >= 1 && args[0] == "checkout":
			return "", nil
		default:
			return "", nil
		}
	}

	var out bytes.Buffer
	err := runUpdate(
		strings.NewReader(""),
		&out,
		updateCommandOptions{
			Tag:     "v1.0.0",
			Channel: "stable",
			Timeout: 10 * time.Second,
			JSON:    true,
			Yes:     true,
		},
	)
	if err != nil {
		t.Fatalf("runUpdate error: %v", err)
	}
	output := strings.TrimSpace(out.String())
	if output == "" {
		t.Fatal("expected JSON output, got empty")
	}
	if output[0] != '{' {
		t.Fatalf("expected JSON output starting with '{', got: %q", output)
	}
	// Ensure no non-JSON text after the payload
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %q", err, output)
	}
}
