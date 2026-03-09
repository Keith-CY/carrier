package gateway

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoteControlStateLoadAndSaveValidation(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))

	state, path, err := loadRemoteControlState()
	if err != nil {
		t.Fatalf("loadRemoteControlState() unexpected error: %v", err)
	}
	if path == "" {
		t.Fatalf("expected store path")
	}
	if len(state.Hosts) != 0 || len(state.Profiles) != 0 || len(state.Bindings) != 0 {
		t.Fatalf("expected fresh state to be empty: %#v", state)
	}

	if err := saveRemoteControlState("", state); err == nil {
		t.Fatal("expected error for empty save path")
	}
	if err := saveRemoteControlState(path, nil); err == nil {
		t.Fatal("expected error for nil save state")
	}

	raw := "not-json"
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write invalid state file: %v", err)
	}
	if _, _, err := loadRemoteControlState(); err == nil {
		t.Fatal("expected parse failure for invalid state JSON")
	}
}

func TestRemoteHostManagementHelpers(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))

	host, err := upsertRemoteHost(RemoteHost{
		Name:        "test-host",
		Host:        "127.0.0.1",
		Port:        22,
		User:        "root",
		AuthMode:    RemoteAuthModePrivateKey,
		KeyPath:     filepath.Join(t.TempDir(), "id.key"),
		Labels:      []string{" Prod ", "gpu", "prod"},
		RuntimeMode: RemoteRuntimeModeOnDemand,
	})
	if err != nil {
		t.Fatalf("upsertRemoteHost valid host failed: %v", err)
	}
	if host.ID == "" || host.Name == "" || host.CreatedAt == "" || host.UpdatedAt == "" {
		t.Fatalf("expected normalized host metadata, got %+v", host)
	}
	if got, ok, err := getRemoteHost(host.ID); err != nil || !ok {
		t.Fatalf("getRemoteHost(%q) expected hit, got ok=%v err=%v got=%+v", host.ID, ok, err, got)
	}
	if strings.Join(host.Labels, ",") != "gpu,prod" {
		t.Fatalf("expected normalized labels gpu,prod got %+v", host.Labels)
	}

	got, err := patchRemoteHost(host.ID, RemoteHost{
		Name:   "patched",
		Port:   23,
		Labels: []string{" staging ", "gpu", "staging"},
	})
	if err != nil {
		t.Fatalf("patchRemoteHost failed: %v", err)
	}
	if got.Name != "patched" || got.Port != 23 {
		t.Fatalf("unexpected patched host: %+v", got)
	}
	if strings.Join(got.Labels, ",") != "gpu,staging" {
		t.Fatalf("expected patched labels gpu,staging got %+v", got.Labels)
	}

	if err := updateRemoteHostHealth(host.ID, RemoteHealthHealthy, "ok"); err != nil {
		t.Fatalf("updateRemoteHostHealth failed: %v", err)
	}
	if removed, err := deleteRemoteHost(host.ID); err != nil || !removed {
		t.Fatalf("expected host deletion success removed=%v err=%v", removed, err)
	}
	if removed, err := deleteRemoteHost(host.ID); err != nil || removed {
		t.Fatalf("expected idempotent host deletion removed=%v err=%v", removed, err)
	}
	if _, ok, err := getRemoteHost(host.ID); err != nil || ok {
		t.Fatalf("expected host not found after deletion, got ok=%v err=%v", ok, err)
	}

	if _, err := patchRemoteHost(host.ID, RemoteHost{Name: "missing"}); err == nil {
		t.Fatal("expected patchRemoteHost to fail for missing host")
	}
}

func TestProviderProfileAndBindingManagement(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))

	if _, err := upsertProviderProfile(ProviderProfile{
		Provider: "",
		Model:    "gpt-4.1",
	}); err == nil {
		t.Fatal("expected provider profile validation failure for missing provider")
	}
	profile, err := upsertProviderProfile(ProviderProfile{
		Provider: "openai",
		Model:    "gpt-4.1",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("upsertProviderProfile failed: %v", err)
	}
	if profile.Name != "openai/gpt-4.1" {
		t.Fatalf("expected auto-derived profile name, got %q", profile.Name)
	}

	loaded, found, err := getProviderProfile(profile.ID)
	if err != nil || !found {
		t.Fatalf("getProviderProfile(%q) failed found=%v err=%v loaded=%+v", profile.ID, found, err, loaded)
	}

	patched, err := patchProviderProfile(profile.ID, providerProfilePatch{
		Name:     ptrString("openai-chatgpt"),
		Enabled:  boolPtr(false),
		BaseURL:  ptrString("https://api.openai.com/v1"),
		Model:    ptrString("gpt-4o"),
		Provider: ptrString("OpenAI"),
		AuthRef:  ptrString("ref-123"),
	})
	if err != nil {
		t.Fatalf("patchProviderProfile failed: %v", err)
	}
	if patched.Name != "openai-chatgpt" || patched.Enabled || patched.Model != "gpt-4o" {
		t.Fatalf("unexpected patched provider profile: %+v", patched)
	}

	binding, err := upsertProviderBinding(ProviderBinding{
		ProfileID:  profile.ID,
		TargetType: "host",
		TargetID:   "host-1",
	})
	if err != nil {
		t.Fatalf("upsertProviderBinding failed: %v", err)
	}

	profiles, err := listProviderProfiles()
	if err != nil || len(profiles) != 1 {
		t.Fatalf("listProviderProfiles failed: len=%d err=%v", len(profiles), err)
	}
	bindings, err := listProviderBindings()
	if err != nil || len(bindings) != 1 || bindings[0].ID != binding.ID {
		t.Fatalf("listProviderBindings unexpected: %#v err=%v", bindings, err)
	}

	if removed, err := deleteProviderProfile(profile.ID); err != nil || !removed {
		t.Fatalf("deleteProviderProfile failed removed=%v err=%v", removed, err)
	}
	bindings, err = listProviderBindings()
	if err != nil || len(bindings) != 0 {
		t.Fatalf("expected binding cleanup after profile deletion, len=%d err=%v", len(bindings), err)
	}
}

func TestProviderBindingUpdateAndLookup(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))

	profile, err := upsertProviderProfile(ProviderProfile{
		Provider: "openai",
		Model:    "gpt-5",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("upsertProviderProfile failed: %v", err)
	}
	binding, err := upsertProviderBinding(ProviderBinding{
		ProfileID:  profile.ID,
		TargetType: "instance",
		TargetID:   "agent-a",
		SyncMode:   "manual",
	})
	if err != nil {
		t.Fatalf("upsertProviderBinding failed: %v", err)
	}

	deleted, err := deleteProviderBinding("missing")
	if err != nil {
		t.Fatalf("deleteProviderBinding on missing binding returned error: %v", err)
	}
	if deleted {
		t.Fatalf("deleteProviderBinding on missing binding should be false")
	}
	deleted, err = deleteProviderBinding(binding.ID)
	if err != nil || !deleted {
		t.Fatalf("deleteProviderBinding failed deleted=%v err=%v", deleted, err)
	}
	deleted, err = deleteProviderBinding(binding.ID)
	if err != nil || deleted {
		t.Fatalf("deleteProviderBinding should be idempotent: deleted=%v err=%v", deleted, err)
	}
}

func TestOrchestratorPolicyScopePersistence(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))

	timeoutMs := 45000
	retryBudget := 2
	policy, err := upsertOrchestratorPolicy(OrchestratorPolicyRule{
		Name:             "scoped policy",
		Action:           "ask",
		Enabled:          true,
		Teams:            []string{"platform"},
		Projects:         []string{"carrier"},
		Environments:     []string{"prod"},
		TemplateIDs:      []string{"rollout-smoke"},
		MaxTaskTimeoutMs: &timeoutMs,
		MaxRetryBudget:   &retryBudget,
	})
	if err != nil {
		t.Fatalf("upsertOrchestratorPolicy failed: %v", err)
	}

	policies, err := listOrchestratorPolicies()
	if err != nil {
		t.Fatalf("listOrchestratorPolicies failed: %v", err)
	}
	if len(policies) != 1 {
		t.Fatalf("expected 1 policy got %d", len(policies))
	}
	if strings.Join(policy.Teams, ",") != "platform" || strings.Join(policy.Projects, ",") != "carrier" {
		t.Fatalf("unexpected normalized scope fields: %+v", policy)
	}
	if strings.Join(policy.Environments, ",") != "prod" || strings.Join(policy.TemplateIDs, ",") != "rollout-smoke" {
		t.Fatalf("unexpected normalized env/template fields: %+v", policy)
	}
	if policy.MaxTaskTimeoutMs == nil || *policy.MaxTaskTimeoutMs != timeoutMs {
		t.Fatalf("unexpected timeout policy: %+v", policy)
	}
	if policy.MaxRetryBudget == nil || *policy.MaxRetryBudget != retryBudget {
		t.Fatalf("unexpected retry policy: %+v", policy)
	}
}

func TestRemoteInstanceSyncStatusLifecycle(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))

	if _, err := upsertRemoteInstanceSyncStatus(RemoteInstanceSyncStatus{
		HostID:   "host-a",
		AgentID:  "",
		SyncMode: "manual",
	}); err == nil {
		t.Fatal("expected missing agentId validation error")
	}

	status, err := upsertRemoteInstanceSyncStatus(RemoteInstanceSyncStatus{
		HostID:   "host-a",
		AgentID:  "agent-a",
		SyncMode: "manual",
	})
	if err != nil {
		t.Fatalf("upsertRemoteInstanceSyncStatus failed: %v", err)
	}
	if status.DriftState != "unknown" || status.MemoryLastSyncStatus != "unknown" || status.MemoryGit.AuthMode != "system" {
		t.Fatalf("expected defaults after upsert, got %+v", status)
	}

	got, found, err := getRemoteInstanceSyncStatus("host-a", "agent-a")
	if err != nil || !found || got.SyncMode != "manual" {
		t.Fatalf("getRemoteInstanceSyncStatus failed found=%v err=%v got=%+v", found, err, got)
	}
	if !strings.EqualFold(remoteInstanceSyncKey("Host-A", "Agent-A"), "host-a:agent-a") {
		t.Fatalf("unexpected remoteInstanceSyncKey normalization")
	}

	if err := deleteRemoteInstanceSyncStatus("host-a", "agent-a"); err != nil {
		t.Fatalf("deleteRemoteInstanceSyncStatus failed: %v", err)
	}
	if _, found, err := getRemoteInstanceSyncStatus("host-a", "agent-a"); err != nil || found {
		t.Fatalf("expected status removed after delete found=%v err=%v", found, err)
	}
}

func ptrString(v string) *string { return &v }
func boolPtr(v bool) *bool       { return &v }
