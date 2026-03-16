package integration

import (
	"path/filepath"
	"testing"
)

func TestNormalizeBindingDefaults(t *testing.T) {
	binding, err := NormalizeBinding(Binding{
		Adapter:        " one-tok ",
		Account:        " provider-org-1 ",
		CallbackURL:    " https://platform.invalid/callbacks ",
		CallbackKeyID:  " key-1 ",
		CallbackSecret: " secret-1 ",
		Target: BindingTarget{
			HostID:        " host-1 ",
			AgentID:       " MAIN ",
			Backend:       " codex ",
			WorkspaceRoot: " /workspace ",
		},
		Capabilities: []string{" repo_fix ", "repo_fix", " diagnostics "},
	})
	if err != nil {
		t.Fatalf("NormalizeBinding() error = %v", err)
	}
	if binding.ID == "" {
		t.Fatal("expected generated binding id")
	}
	if binding.Adapter != "one-tok" {
		t.Fatalf("adapter=%q want one-tok", binding.Adapter)
	}
	if binding.Account != "provider-org-1" {
		t.Fatalf("account=%q want provider-org-1", binding.Account)
	}
	if binding.CallbackURL != "https://platform.invalid/callbacks" {
		t.Fatalf("callbackUrl=%q", binding.CallbackURL)
	}
	if binding.CallbackKeyID != "key-1" {
		t.Fatalf("callbackKeyId=%q", binding.CallbackKeyID)
	}
	if binding.CallbackSecret != "secret-1" {
		t.Fatalf("callbackSecret=%q", binding.CallbackSecret)
	}
	if binding.Status != BindingStatusDraft {
		t.Fatalf("status=%q want %q", binding.Status, BindingStatusDraft)
	}
	if binding.Target.AgentID != "main" {
		t.Fatalf("target.agentId=%q want main", binding.Target.AgentID)
	}
	if len(binding.Capabilities) != 2 || binding.Capabilities[0] != "diagnostics" || binding.Capabilities[1] != "repo_fix" {
		t.Fatalf("capabilities=%v want [diagnostics repo_fix]", binding.Capabilities)
	}
}

func TestNormalizeBindingRejectsMissingTuple(t *testing.T) {
	_, err := NormalizeBinding(Binding{
		Adapter: "one-tok",
		Account: "provider-org-1",
		Target: BindingTarget{
			HostID:        "host-1",
			Backend:       "codex",
			WorkspaceRoot: "/workspace",
		},
	})
	if err == nil {
		t.Fatal("expected missing agent tuple error")
	}
}

func TestResolvePathsUsesCarrierAppRoot(t *testing.T) {
	t.Setenv("CARRIER_ROOT", "/tmp/carrier-root")

	paths, err := ResolvePaths()
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	if paths.Root != filepath.Join("/tmp/carrier-root", "app", "integrations") {
		t.Fatalf("root=%q", paths.Root)
	}
	if paths.DBPath != filepath.Join("/tmp/carrier-root", "app", "integrations", "state.sqlite") {
		t.Fatalf("db=%q", paths.DBPath)
	}
}
