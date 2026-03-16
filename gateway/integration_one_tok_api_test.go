package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"carrier/shared/integration"
)

func TestHandleOneTokVerifyUsesProviderScopedToken(t *testing.T) {
	t.Setenv("CARRIER_ROOT", t.TempDir())

	host, err := upsertRemoteHost(RemoteHost{
		Name:     "host-1",
		Host:     "example.internal",
		User:     "carrier",
		AuthMode: RemoteAuthModePrivateKey,
		KeyRef:   "ssh-key",
	})
	if err != nil {
		t.Fatalf("upsertRemoteHost() error = %v", err)
	}
	binding, err := upsertIntegrationBinding(integration.Binding{
		Adapter: "one-tok",
		Account: "provider-org-1",
		Target: integration.BindingTarget{
			HostID:        host.ID,
			AgentID:       "main",
			Backend:       "codex",
			WorkspaceRoot: "/workspace",
		},
		Status:       integration.BindingStatusActive,
		Capabilities: []string{"repo_fix", "diagnostics"},
	})
	if err != nil {
		t.Fatalf("upsertIntegrationBinding() error = %v", err)
	}
	_, token, err := issueIntegrationBindingToken(binding.ID, "integration-test")
	if err != nil {
		t.Fatalf("issueIntegrationBindingToken() error = %v", err)
	}

	prev := integrationVerifyBindingTarget
	integrationVerifyBindingTarget = func(binding integration.Binding, req integration.VerifyBindingRequest) (integration.VerifyBindingResult, error) {
		return integration.VerifyBindingResult{
			Verified: true,
			Health: integration.BindingHealth{
				Healthy:       true,
				WorkspaceRoot: req.WorkspaceRoot,
			},
			VersionValue:     "codex 1.2.3",
			ResolvedHostID:   req.HostID,
			ResolvedAgentID:  req.AgentID,
			ResolvedBackend:  req.Backend,
			Capabilities:     req.SupportedCapabilities,
			BindingID:        binding.ID,
			BindingAccountID: binding.Account,
		}, nil
	}
	defer func() { integrationVerifyBindingTarget = prev }()

	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/one-tok/bindings/verify", strings.NewReader(`{
		"hostId":"`+host.ID+`",
		"agentId":"main",
		"backend":"codex",
		"workspaceRoot":"/workspace",
		"supportedCapabilities":["repo_fix","diagnostics"]
	}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Result string                          `json:"result"`
		Verify integration.VerifyBindingResult `json:"verify"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Result != "ok" || !payload.Verify.Verified {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Verify.BindingID != binding.ID {
		t.Fatalf("bindingId=%q want %q", payload.Verify.BindingID, binding.ID)
	}
}
