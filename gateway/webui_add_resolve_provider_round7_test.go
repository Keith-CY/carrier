package gateway

import (
	"path/filepath"
	"testing"
	"time"
)

func prepareResolveProviderTestEnv(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("CARRIER_CONFIG", filepath.Join(tmp, "missing-config.json"))
	t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_CODEX_TOKEN", "")
	t.Setenv("OPENAI_COMPATIBLE_API_KEY", "")
}

func TestResolveWebUIAddProviderID_DefaultFallbackPrefersOpenAICodex(t *testing.T) {
	prepareResolveProviderTestEnv(t)

	if got := resolveWebUIAddProviderID("openclaw"); got != "openai-codex" {
		t.Fatalf("resolveWebUIAddProviderID(openclaw) = %q, want openai-codex", got)
	}
}

func TestResolveWebUIAddProviderID_PrefersSavedCredential(t *testing.T) {
	prepareResolveProviderTestEnv(t)
	if _, err := saveProviderCredential("openai", "sk-openai"); err != nil {
		t.Fatalf("saveProviderCredential(openai): %v", err)
	}

	if got := resolveWebUIAddProviderID("openclaw"); got != "openai" {
		t.Fatalf("resolveWebUIAddProviderID(openclaw) = %q, want openai", got)
	}
}

func TestResolveWebUIAddProviderID_PrefersConfiguredDefaultOverLatestInstance(t *testing.T) {
	prepareResolveProviderTestEnv(t)
	writeGatewayDefaultProviderConfig(t, "anthropic", "anthropic/claude-opus-4-6", "ANTHROPIC_API_KEY")

	storePath := filepath.Join(t.TempDir(), "instances.json")
	t.Setenv("CARRIER_INSTANCE_STORE", storePath)
	now := time.Now().UTC()
	if err := saveManagedInstances(storePath, []managedAgentInstance{
		{
			ID:        "openclaw-old",
			AgentID:   "openclaw",
			Provider:  "openai",
			UpdatedAt: now.Add(-time.Minute).Format(time.RFC3339Nano),
		},
		{
			ID:        "openclaw-new",
			AgentID:   "openclaw",
			Provider:  "openai-codex",
			UpdatedAt: now.Format(time.RFC3339Nano),
		},
	}); err != nil {
		t.Fatalf("saveManagedInstances: %v", err)
	}

	if got := resolveWebUIAddProviderID("openclaw"); got != "anthropic" {
		t.Fatalf("resolveWebUIAddProviderID(openclaw) = %q, want anthropic", got)
	}
}

func TestResolveWebUIAddProviderID_UsesLatestInstanceWhenConfiguredDefaultUnavailable(t *testing.T) {
	prepareResolveProviderTestEnv(t)
	writeGatewayDefaultProviderConfig(t, "unknown-provider", "unknown/default", "UNKNOWN_API_KEY")

	storePath := filepath.Join(t.TempDir(), "instances.json")
	t.Setenv("CARRIER_INSTANCE_STORE", storePath)
	now := time.Now().UTC()
	if err := saveManagedInstances(storePath, []managedAgentInstance{
		{
			ID:        "openclaw-old",
			AgentID:   "openclaw",
			Provider:  "openai",
			UpdatedAt: now.Add(-2 * time.Minute).Format(time.RFC3339Nano),
		},
		{
			ID:        "openclaw-new",
			AgentID:   "openclaw",
			Provider:  "anthropic",
			UpdatedAt: now.Format(time.RFC3339Nano),
		},
	}); err != nil {
		t.Fatalf("saveManagedInstances: %v", err)
	}

	if got := resolveWebUIAddProviderID("openclaw"); got != "anthropic" {
		t.Fatalf("resolveWebUIAddProviderID(openclaw) = %q, want anthropic", got)
	}
}
