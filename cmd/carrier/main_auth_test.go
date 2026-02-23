package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveProviderCredentialAutoWarnsAndContinuesOnStoreFailure(t *testing.T) {
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")

	invalidStorePath := filepath.Join(t.TempDir(), "credentials-dir")
	if err := os.MkdirAll(invalidStorePath, 0o755); err != nil {
		t.Fatalf("mkdir invalid store path: %v", err)
	}
	t.Setenv("CARRIER_CREDENTIAL_STORE", invalidStorePath)

	provider := choiceOption{ID: "openai-codex", Name: "OpenAI Codex"}
	var out bytes.Buffer
	if err := saveProviderCredentialAuto(&out, provider, "test-token"); err != nil {
		t.Fatalf("saveProviderCredentialAuto error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Warning: failed to save OpenAI Codex credential") {
		t.Fatalf("expected warning output, got %q", output)
	}
}
