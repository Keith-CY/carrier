package gateway

import (
	"os"
	"path/filepath"
	"testing"

	"carrier/shared/integration"
)

func TestIntegrationStoreIssueAndAuthenticateToken(t *testing.T) {
	t.Setenv("CARRIER_ROOT", t.TempDir())

	saved, err := upsertIntegrationBinding(integration.Binding{
		Adapter:        "one-tok",
		Account:        "provider-org-1",
		CallbackURL:    "https://platform.invalid/callbacks",
		CallbackKeyID:  "kid-1",
		CallbackSecret: "secret-1",
		Target: integration.BindingTarget{
			HostID:        "host-1",
			AgentID:       "main",
			Backend:       "codex",
			WorkspaceRoot: "/workspace",
		},
		Status:       integration.BindingStatusActive,
		Capabilities: []string{"repo_fix"},
	})
	if err != nil {
		t.Fatalf("upsertIntegrationBinding() error = %v", err)
	}

	record, token, err := issueIntegrationBindingToken(saved.ID, "integration-test")
	if err != nil {
		t.Fatalf("issueIntegrationBindingToken() error = %v", err)
	}
	if token == "" {
		t.Fatal("expected raw token")
	}
	if record.TokenPrefix == "" {
		t.Fatal("expected token prefix")
	}
	if record.TokenHash == token {
		t.Fatal("token hash should not persist raw token")
	}

	gotBinding, gotToken, ok, err := authenticateIntegrationToken(token, "one-tok")
	if err != nil {
		t.Fatalf("authenticateIntegrationToken() error = %v", err)
	}
	if !ok {
		t.Fatal("expected token auth success")
	}
	if gotBinding.ID != saved.ID {
		t.Fatalf("binding.id=%q want %q", gotBinding.ID, saved.ID)
	}
	if gotBinding.CallbackKeyID != "kid-1" || gotBinding.CallbackSecret != "secret-1" {
		t.Fatalf("callback binding fields not persisted: %+v", gotBinding)
	}
	if gotToken.ID != record.ID {
		t.Fatalf("token.id=%q want %q", gotToken.ID, record.ID)
	}

	if _, _, ok, err := authenticateIntegrationToken(token, "other-adapter"); err != nil || ok {
		t.Fatalf("expected adapter scope mismatch, got ok=%v err=%v", ok, err)
	}
}

func TestIntegrationStorePersistsSQLiteUnderCarrierAppRoot(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CARRIER_ROOT", root)

	_, err := upsertIntegrationBinding(integration.Binding{
		Adapter: "one-tok",
		Account: "provider-org-1",
		Target: integration.BindingTarget{
			HostID:        "host-1",
			AgentID:       "main",
			Backend:       "codex",
			WorkspaceRoot: "/workspace",
		},
	})
	if err != nil {
		t.Fatalf("upsertIntegrationBinding() error = %v", err)
	}

	dbPath := filepath.Join(root, "app", "integrations", "state.sqlite")
	if _, err := loadIntegrationBindingByID("missing"); err != nil {
		t.Fatalf("loadIntegrationBindingByID(missing) unexpected error = %v", err)
	}
	if _, statErr := os.Stat(dbPath); statErr != nil {
		t.Fatalf("expected sqlite db at %s: %v", dbPath, statErr)
	}
}
