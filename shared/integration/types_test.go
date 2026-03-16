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

func TestNormalizeVerifyBindingRequestNormalizesFields(t *testing.T) {
	req, err := NormalizeVerifyBindingRequest(VerifyBindingRequest{
		HostID:        " host-1 ",
		AgentID:       " MAIN ",
		Backend:       " codex ",
		WorkspaceRoot: " /workspace/../workspace/current ",
		SupportedCapabilities: []string{
			" repo_fix ",
			"diagnostics",
			"repo_fix",
		},
	})
	if err != nil {
		t.Fatalf("NormalizeVerifyBindingRequest() error = %v", err)
	}
	if req.HostID != "host-1" || req.AgentID != "main" || req.Backend != "codex" {
		t.Fatalf("unexpected normalized identifiers: %+v", req)
	}
	if req.WorkspaceRoot != "/workspace/current" {
		t.Fatalf("workspaceRoot=%q", req.WorkspaceRoot)
	}
	if len(req.SupportedCapabilities) != 2 || req.SupportedCapabilities[0] != "diagnostics" || req.SupportedCapabilities[1] != "repo_fix" {
		t.Fatalf("supportedCapabilities=%v", req.SupportedCapabilities)
	}
}

func TestNormalizeVerifyBindingRequestRejectsMissingHost(t *testing.T) {
	_, err := NormalizeVerifyBindingRequest(VerifyBindingRequest{
		AgentID:       "main",
		Backend:       "codex",
		WorkspaceRoot: "/workspace",
	})
	if err == nil {
		t.Fatal("expected missing host validation error")
	}
}

func TestNormalizeBindingTokenDefaultsAndValidation(t *testing.T) {
	token, err := NormalizeBindingToken(BindingToken{
		BindingID:   " bind_1 ",
		Adapter:     " One-Tok ",
		TokenPrefix: " cit_deadbeef ",
		TokenHash:   " hash ",
	})
	if err != nil {
		t.Fatalf("NormalizeBindingToken() error = %v", err)
	}
	if token.BindingID != "bind_1" || token.Adapter != "one-tok" {
		t.Fatalf("unexpected normalized token: %+v", token)
	}
	if token.Status != TokenStatusActive {
		t.Fatalf("status=%q want %q", token.Status, TokenStatusActive)
	}

	if _, err := NormalizeBindingToken(BindingToken{BindingID: "bind_1", Adapter: "one-tok", TokenHash: "hash"}); err == nil {
		t.Fatal("expected missing token prefix validation error")
	}
}

func TestNormalizeCreateExecutionRequestDefaults(t *testing.T) {
	req, err := NormalizeCreateExecutionRequest(CreateExecutionRequest{
		IdempotencyKey:    " idem-1 ",
		Goal:              " Fix repo regression ",
		Input:             " ",
		RequestedProvider: " openrouter ",
		RequiredMemory:    []string{" repo_fix ", "repo_fix", " diagnostics "},
		DistillOutputs:    []string{" summary ", " patch "},
		MaxConcurrency:    -4,
	})
	if err != nil {
		t.Fatalf("NormalizeCreateExecutionRequest() error = %v", err)
	}
	if req.Input != "Fix repo regression" {
		t.Fatalf("input=%q want goal fallback", req.Input)
	}
	if req.RequestedProvider != "openrouter" {
		t.Fatalf("requestedProvider=%q", req.RequestedProvider)
	}
	if req.MaxConcurrency != 0 {
		t.Fatalf("maxConcurrency=%d want 0", req.MaxConcurrency)
	}
	if len(req.RequiredMemory) != 2 || req.RequiredMemory[0] != "diagnostics" || req.RequiredMemory[1] != "repo_fix" {
		t.Fatalf("requiredMemory=%v", req.RequiredMemory)
	}
	if len(req.DistillOutputs) != 2 || req.DistillOutputs[0] != "patch" || req.DistillOutputs[1] != "summary" {
		t.Fatalf("distillOutputs=%v", req.DistillOutputs)
	}
}

func TestNormalizeCreateExecutionRequestRejectsMissingFields(t *testing.T) {
	if _, err := NormalizeCreateExecutionRequest(CreateExecutionRequest{Goal: "goal"}); err == nil {
		t.Fatal("expected missing idempotency key validation error")
	}
	if _, err := NormalizeCreateExecutionRequest(CreateExecutionRequest{IdempotencyKey: "idem-1"}); err == nil {
		t.Fatal("expected missing goal validation error")
	}
}

func TestNormalizeActionRequestAndHelpers(t *testing.T) {
	req, err := NormalizeActionRequest(ActionRequest{
		Type:           " ReSuMe ",
		IdempotencyKey: " action-1 ",
		Reason:         " resume execution ",
	})
	if err != nil {
		t.Fatalf("NormalizeActionRequest() error = %v", err)
	}
	if req.Type != ActionTypeResume || req.IdempotencyKey != "action-1" || req.Reason != "resume execution" {
		t.Fatalf("unexpected normalized action request: %+v", req)
	}
	if _, err := NormalizeActionRequest(ActionRequest{}); err == nil {
		t.Fatal("expected missing action type validation error")
	}

	if got := normalizeTokenStatus(" ReVoKeD "); got != TokenStatusRevoked {
		t.Fatalf("normalizeTokenStatus()=%q", got)
	}
	if got := normalizeExecutionState(" PaUsEd "); got != ExecutionStatePaused {
		t.Fatalf("normalizeExecutionState()=%q", got)
	}
	if got := normalizeActionType(" cancel "); got != ActionTypeCancel {
		t.Fatalf("normalizeActionType()=%q", got)
	}
	if got := normalizeBindingStatus(" active "); got != BindingStatusActive {
		t.Fatalf("normalizeBindingStatus()=%q", got)
	}
	if got := normalizeExecutionState("bogus"); got != "" {
		t.Fatalf("normalizeExecutionState(bogus)=%q want empty", got)
	}
}

func TestGenerateTokenRawAndInternalHelpers(t *testing.T) {
	raw, err := GenerateTokenRaw("cit_")
	if err != nil {
		t.Fatalf("GenerateTokenRaw() error = %v", err)
	}
	if len(raw) != len("cit_")+32 {
		t.Fatalf("token length=%d want %d", len(raw), len("cit_")+32)
	}
	if raw[:4] != "cit_" {
		t.Fatalf("token prefix=%q", raw[:4])
	}

	if got := normalizeWorkspaceRoot(" /workspace/../workspace/current "); got != "/workspace/current" {
		t.Fatalf("normalizeWorkspaceRoot()=%q", got)
	}

	id, err := ensurePrefixedID("bind", "binding id", " Bind 1 ")
	if err != nil {
		t.Fatalf("ensurePrefixedID(existing) error = %v", err)
	}
	if id != "bind-1" {
		t.Fatalf("ensurePrefixedID(existing)=%q", id)
	}

	generated, err := ensurePrefixedID("bind", "binding id", "")
	if err != nil {
		t.Fatalf("ensurePrefixedID(generated) error = %v", err)
	}
	if len(generated) != len("bind_")+16 || generated[:5] != "bind_" {
		t.Fatalf("generated id=%q", generated)
	}
}
