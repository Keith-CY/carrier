package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestPickManagedAddProviderWithReasonUsesLatestManagedInstanceProvider(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CARRIER_DEFAULT_PROVIDER_ID", "")
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
			Provider:  "deepseek",
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
	if provider.ID != "deepseek" {
		t.Fatalf("provider.ID = %q, want %q", provider.ID, "deepseek")
	}
	if !strings.Contains(strings.ToLower(reason), "latest openclaw instance") {
		t.Fatalf("reason should mention latest managed instance reuse, got %q", reason)
	}
}
