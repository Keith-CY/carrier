package main

import (
	"carrier/configv2"
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

func TestParseOnboardCommandArgsWithFlags(t *testing.T) {
	opts, err := parseOnboardCommandArgs([]string{"--telegram-bot-token", "tg-token", "--provider", "openai-oauth"})
	if err != nil {
		t.Fatalf("parseOnboardCommandArgs error: %v", err)
	}
	if opts.WebUI {
		t.Fatal("expected WebUI=false")
	}
	if opts.TelegramBotToken != "tg-token" {
		t.Fatalf("TelegramBotToken = %q, want tg-token", opts.TelegramBotToken)
	}
	if opts.ProviderID != "openai-codex" {
		t.Fatalf("ProviderID = %q, want openai-codex", opts.ProviderID)
	}
}

func TestParseOnboardCommandArgsRejectsWebUICombination(t *testing.T) {
	_, err := parseOnboardCommandArgs([]string{"--webui", "--telegram-bot-token", "tg-token"})
	if err == nil {
		t.Fatal("expected parseOnboardCommandArgs to reject --webui combination")
	}
}

func TestParseAddCommandArgsWithFlags(t *testing.T) {
	opts, err := parseAddCommandArgs([]string{"openclaw", "--telegram-bot-token", "tg-token", "--provider", "openai-oauth"})
	if err != nil {
		t.Fatalf("parseAddCommandArgs error: %v", err)
	}
	if opts.AgentID != "openclaw" {
		t.Fatalf("AgentID = %q, want openclaw", opts.AgentID)
	}
	if opts.TelegramBotToken != "tg-token" {
		t.Fatalf("TelegramBotToken = %q, want tg-token", opts.TelegramBotToken)
	}
	if opts.ProviderID != "openai-codex" {
		t.Fatalf("ProviderID = %q, want openai-codex", opts.ProviderID)
	}
}

func TestPickProviderForManagedAddWithReasonUsesCarrierConfig(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.v2.json")
	t.Setenv("CARRIER_CONFIG", cfgPath)
	t.Setenv("CARRIER_DEFAULT_PROVIDER_ID", "")

	cfg := &configv2.Config{
		ConfigVersion: configv2.CurrentVersion,
		Channels: []configv2.Channel{
			{ID: "telegram", Enabled: true, BotToken: "tg-token"},
		},
		ModelList: []configv2.Model{
			{
				ModelName:  "openai-default",
				Model:      "openai/gpt-5.2",
				ProviderID: "openai",
			},
		},
		DefaultModel: "openai-default",
	}
	if _, err := configv2.Save(cfg); err != nil {
		t.Fatalf("configv2.Save error: %v", err)
	}

	provider, reason, err := pickProviderForManagedAddWithReason("")
	if err != nil {
		t.Fatalf("pickProviderForManagedAddWithReason error: %v", err)
	}
	if provider.ID != "openai" {
		t.Fatalf("provider.ID = %q, want openai", provider.ID)
	}
	if !strings.Contains(reason, "Carrier config") {
		t.Fatalf("expected reason to mention Carrier config, got %q", reason)
	}
}

func TestLoadConfiguredTelegramBotToken(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.v2.json")
	t.Setenv("CARRIER_CONFIG", cfgPath)

	cfg := &configv2.Config{
		ConfigVersion: configv2.CurrentVersion,
		Channels: []configv2.Channel{
			{ID: "telegram", Enabled: true, BotToken: "tg-token-from-config"},
		},
	}
	if _, err := configv2.Save(cfg); err != nil {
		t.Fatalf("configv2.Save error: %v", err)
	}

	token := loadConfiguredTelegramBotToken()
	if token != "tg-token-from-config" {
		t.Fatalf("token = %q, want tg-token-from-config", token)
	}
}

func TestPrepareManagedAgentAddArtifactsIncludesPairedTelegramChat(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	provider := choiceOption{
		ID:           "openai",
		Name:         "OpenAI",
		ProviderEnv:  "OPENAI_API_KEY",
		ExampleModel: "openai/gpt-5.2",
	}
	envVars := map[string]string{
		"OPENAI_API_KEY": "sk-test",
	}

	result, err := prepareManagedAgentAddArtifacts(
		"openclaw",
		"openclaw-test-instance",
		"telegram",
		"tg-token",
		provider,
		envVars,
		"418258935",
	)
	if err != nil {
		t.Fatalf("prepareManagedAgentAddArtifacts error: %v", err)
	}
	if result.PairedChatID != "418258935" {
		t.Fatalf("PairedChatID = %q, want 418258935", result.PairedChatID)
	}

	raw, err := os.ReadFile(result.ConfigPath)
	if err != nil {
		t.Fatalf("read config path %s: %v", result.ConfigPath, err)
	}
	if !strings.Contains(string(raw), `"allow_from": [`) || !strings.Contains(string(raw), `"418258935"`) {
		t.Fatalf("expected allow_from to include paired chat id, got %s", string(raw))
	}
}
