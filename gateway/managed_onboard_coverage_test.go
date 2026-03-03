package gateway

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedAgentChannelsAndChannelPromptHelpers(t *testing.T) {
	channels, ok := managedAgentChannels("openclaw")
	if !ok {
		t.Fatalf("expected supported channels for openclaw")
	}
	if len(channels) == 0 {
		t.Fatalf("expected at least one managed channel for openclaw")
	}

	if _, ok := managedAgentChannels("unknown-agent"); ok {
		t.Fatalf("expected unsupported managed agent to return no channels")
	}

	prompt := renderManagedChannelTokenPrompt("openclaw", picoclawChannel{
		ID:         "telegram",
		Name:       "Telegram",
		TokenLabel: "Telegram bot token for OpenClaw",
	})
	if !strings.Contains(prompt, "OpenClaw") {
		t.Fatalf("expected OpenClaw in token prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "Telegram bot token for OpenClaw") {
		t.Fatalf("expected token label in prompt, got %q", prompt)
	}
}

func TestResolveManagedConfigPathResolvesHomeAndRelativePaths(t *testing.T) {
	home := t.TempDir()
	cfg := managedAgentConfig{
		ConfigDir:  ".openclaw",
		ConfigFile: "openclaw.json",
	}

	defaultPath := resolveManagedConfigPath(home, cfg, managedRendererSelection{})
	expectedDefault := filepath.Join(home, ".openclaw", "openclaw.json")
	if defaultPath != expectedDefault {
		t.Fatalf("expected default managed config path %q, got %q", expectedDefault, defaultPath)
	}

	homePath := resolveManagedConfigPath(home, cfg, managedRendererSelection{
		ConfigPath: "~/override/openclaw.json",
	})
	expectedHome := filepath.Join(home, "override", "openclaw.json")
	if homePath != expectedHome {
		t.Fatalf("expected home-expanded config path %q, got %q", expectedHome, homePath)
	}

	absolutePath := resolveManagedConfigPath(home, cfg, managedRendererSelection{
		ConfigPath: "/var/lib/openclaw.json",
	})
	if absolutePath != filepath.Clean("/var/lib/openclaw.json") {
		t.Fatalf("expected absolute config path to be preserved, got %q", absolutePath)
	}

	relativePath := resolveManagedConfigPath(home, cfg, managedRendererSelection{
		ConfigPath: "configs/openclaw.json",
	})
	expectedRelative := filepath.Join(home, ".openclaw", "configs", "openclaw.json")
	if relativePath != expectedRelative {
		t.Fatalf("expected relative config path %q, got %q", expectedRelative, relativePath)
	}
}
