package gateway

import (
	"path/filepath"
	"testing"
)

func snapshotProviderCatalog(t *testing.T) {
	t.Helper()
	origCatalog := append([]LLMProvider(nil), llmProviderCatalog...)
	origIndex := make(map[string]*LLMProvider, len(llmProviderIndex))
	for k, v := range llmProviderIndex {
		if v == nil {
			origIndex[k] = nil
			continue
		}
		cp := *v
		origIndex[k] = &cp
	}
	t.Cleanup(func() {
		llmProviderCatalog = append([]LLMProvider(nil), origCatalog...)
		llmProviderIndex = make(map[string]*LLMProvider, len(origIndex))
		for k, v := range origIndex {
			if v == nil {
				llmProviderIndex[k] = nil
				continue
			}
			cp := *v
			llmProviderIndex[k] = &cp
		}
	})
}

func rebuildProviderIndexForTest() {
	idx := make(map[string]*LLMProvider, len(llmProviderCatalog)+len(llmProviderAliases))
	for i := range llmProviderCatalog {
		p := &llmProviderCatalog[i]
		idx[p.ID] = p
	}
	for alias, canonicalID := range llmProviderAliases {
		if canonical, ok := idx[canonicalID]; ok {
			idx[alias] = canonical
		}
	}
	llmProviderIndex = idx
}

func setProviderEnvVarForTest(providerID, envVar string) bool {
	for i := range llmProviderCatalog {
		if llmProviderCatalog[i].ID == providerID {
			llmProviderCatalog[i].EnvVar = envVar
			return true
		}
	}
	return false
}

func prepareResolveProviderTestEnv(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("CARRIER_CONFIG", filepath.Join(tmp, "missing-config.json"))
	t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
	t.Setenv("OPENAI_API_KEY", "")
}

func TestResolveWebUIAddProviderID_OpenAIFallbackWhenCodexIncompatible(t *testing.T) {
	prepareResolveProviderTestEnv(t)
	snapshotProviderCatalog(t)

	if !setProviderEnvVarForTest("openai-codex", "") {
		t.Fatal("openai-codex provider not found")
	}
	rebuildProviderIndexForTest()

	if got := resolveWebUIAddProviderID("openclaw"); got != "openai" {
		t.Fatalf("resolveWebUIAddProviderID(openclaw) = %q, want openai", got)
	}
}

func TestResolveWebUIAddProviderID_FinalLoopFallback(t *testing.T) {
	prepareResolveProviderTestEnv(t)
	snapshotProviderCatalog(t)

	if !setProviderEnvVarForTest("openai-codex", "") {
		t.Fatal("openai-codex provider not found")
	}
	if !setProviderEnvVarForTest("openai", "") {
		t.Fatal("openai provider not found")
	}
	rebuildProviderIndexForTest()

	if got := resolveWebUIAddProviderID("openclaw"); got != "anthropic" {
		t.Fatalf("resolveWebUIAddProviderID(openclaw) = %q, want anthropic", got)
	}
}

func TestResolveWebUIAddProviderID_ReturnsEmptyWhenNoCompatibleProvider(t *testing.T) {
	prepareResolveProviderTestEnv(t)
	snapshotProviderCatalog(t)

	for i := range llmProviderCatalog {
		llmProviderCatalog[i].EnvVar = ""
	}
	rebuildProviderIndexForTest()

	if got := resolveWebUIAddProviderID("openclaw"); got != "" {
		t.Fatalf("resolveWebUIAddProviderID(openclaw) = %q, want empty", got)
	}
}
