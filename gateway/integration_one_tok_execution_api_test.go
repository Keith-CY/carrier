package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"carrier/shared/integration"
)

func TestHandleOneTokExecutionsCreateStatusAndCancelIdempotency(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CARRIER_ROOT", root)
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(root, "remote-control.json"))

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
		Adapter:     "one-tok",
		Account:     "provider-org-1",
		CallbackURL: "https://platform.invalid/callbacks",
		Target: integration.BindingTarget{
			HostID:        host.ID,
			AgentID:       "main",
			Backend:       "codex",
			WorkspaceRoot: "/workspace",
		},
		Status: integration.BindingStatusActive,
	})
	if err != nil {
		t.Fatalf("upsertIntegrationBinding() error = %v", err)
	}
	_, token, err := issueIntegrationBindingToken(binding.ID, "integration-test")
	if err != nil {
		t.Fatalf("issueIntegrationBindingToken() error = %v", err)
	}

	prevLaunch := orchestratorLaunchExecutionFn
	orchestratorLaunchExecutionFn = func(string) {}
	defer func() { orchestratorLaunchExecutionFn = prevLaunch }()

	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	createBody := []byte(`{
		"idempotencyKey":"idem-1",
		"externalExecutionId":"platform-exec-1",
		"goal":"Fix repo regression",
		"input":"Run tests and fix failures",
		"requestedProvider":"openrouter"
	}`)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/one-tok/executions", bytes.NewReader(createBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Result    string                `json:"result"`
		Existing  bool                  `json:"existing"`
		Execution integration.Execution `json:"execution"`
		Attempt   integration.Attempt   `json:"attempt"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.Execution.ID == "" || created.Attempt.ID == "" {
		t.Fatalf("expected execution and attempt ids, got %+v", created)
	}
	if created.Existing {
		t.Fatalf("first create should not be existing: %+v", created)
	}
	internal, found, err := getOrchestratorExecution(created.Execution.OrchestratorExecutionID)
	if err != nil || !found {
		t.Fatalf("getOrchestratorExecution() found=%v err=%v", found, err)
	}
	if internal.Goal != "Fix repo regression" {
		t.Fatalf("internal goal=%q", internal.Goal)
	}
	events, err := listIntegrationEventsByExecutionID(created.Execution.ID)
	if err != nil {
		t.Fatalf("listIntegrationEventsByExecutionID() error = %v", err)
	}
	if len(events) != 1 || events[0].Sequence != 1 || events[0].EventType != "execution.accepted" {
		t.Fatalf("unexpected integration events: %+v", events)
	}
	deliveries, err := listIntegrationCallbackDeliveriesByExecutionID(created.Execution.ID)
	if err != nil {
		t.Fatalf("listIntegrationCallbackDeliveriesByExecutionID() error = %v", err)
	}
	if len(deliveries) != 1 || deliveries[0].Status != "pending" {
		t.Fatalf("unexpected callback deliveries: %+v", deliveries)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/integrations/one-tok/executions", bytes.NewReader(createBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("idempotent create status=%d body=%s", rec.Code, rec.Body.String())
	}
	var again struct {
		Existing  bool                  `json:"existing"`
		Execution integration.Execution `json:"execution"`
		Attempt   integration.Attempt   `json:"attempt"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &again); err != nil {
		t.Fatalf("decode idempotent create response: %v", err)
	}
	if !again.Existing {
		t.Fatalf("expected idempotent create to report existing")
	}
	if again.Execution.ID != created.Execution.ID || again.Attempt.ID != created.Attempt.ID {
		t.Fatalf("idempotent mismatch got=%+v want execution=%s attempt=%s", again, created.Execution.ID, created.Attempt.ID)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/integrations/one-tok/executions/"+created.Execution.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status get=%d body=%s", rec.Code, rec.Body.String())
	}
	var fetched struct {
		Execution          integration.Execution          `json:"execution"`
		Events             []integration.Event            `json:"events"`
		UsageProofs        []integration.UsageProof       `json:"usageProofs"`
		ArtifactRefs       []integration.ArtifactRef      `json:"artifactRefs"`
		CallbackDeliveries []integration.CallbackDelivery `json:"callbackDeliveries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if len(fetched.Events) == 0 || fetched.Events[0].EventType != "execution.accepted" {
		t.Fatalf("unexpected fetched events: %+v", fetched.Events)
	}
	if len(fetched.UsageProofs) != 0 || len(fetched.ArtifactRefs) != 0 {
		t.Fatalf("expected empty proofs/artifacts, got proofs=%v artifacts=%v", fetched.UsageProofs, fetched.ArtifactRefs)
	}
	if len(fetched.CallbackDeliveries) != 1 || fetched.CallbackDeliveries[0].Status != "pending" {
		t.Fatalf("unexpected callback deliveries in status: %+v", fetched.CallbackDeliveries)
	}

	registerOrchestratorExecutionCancel(internal.ID, func() {})
	defer unregisterOrchestratorExecutionCancel(internal.ID)

	pauseBody := []byte(`{"type":"pause","idempotencyKey":"pause-1","reason":"budget guard"}`)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/integrations/one-tok/executions/"+created.Execution.ID+"/actions", bytes.NewReader(pauseBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("pause status=%d body=%s", rec.Code, rec.Body.String())
	}

	waitCtx, cancelWait := context.WithCancel(context.Background())
	defer cancelWait()
	pausedDone := make(chan error, 1)
	go func() {
		pausedDone <- waitIfOrchestratorExecutionPaused(waitCtx, internal.ID)
	}()
	select {
	case err := <-pausedDone:
		t.Fatalf("pause waiter returned too early: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/integrations/one-tok/executions/"+created.Execution.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("paused status get=%d body=%s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("decode paused status response: %v", err)
	}
	if fetched.Execution.State != integration.ExecutionStatePaused {
		t.Fatalf("expected paused execution state, got %q", fetched.Execution.State)
	}

	resumeBody := []byte(`{"type":"resume","idempotencyKey":"resume-1","reason":"budget recovered"}`)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/integrations/one-tok/executions/"+created.Execution.ID+"/actions", bytes.NewReader(resumeBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("resume status=%d body=%s", rec.Code, rec.Body.String())
	}
	select {
	case err := <-pausedDone:
		if err != nil {
			t.Fatalf("pause waiter returned error after resume: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pause waiter did not resume")
	}

	internal, found, err = getOrchestratorExecution(created.Execution.OrchestratorExecutionID)
	if err != nil || !found {
		t.Fatalf("reload internal execution found=%v err=%v", found, err)
	}
	internal.Outcome.Artifacts = []OrchestratorArtifact{{
		ID:          "artifact-1",
		Name:        "run.log",
		Kind:        "log",
		DownloadURL: "https://example.invalid/run.log",
	}}
	if _, err := upsertOrchestratorExecution(internal); err != nil {
		t.Fatalf("upsertOrchestratorExecution(artifact) error = %v", err)
	}
	if _, err := upsertIntegrationUsageProof(integration.UsageProof{
		ExecutionID: created.Execution.ID,
		ProofRef:    "usage://proof-1",
		MeterRef:    "meter-1",
		UsageKind:   "provider_tokens",
		AmountCents: 42,
	}); err != nil {
		t.Fatalf("upsertIntegrationUsageProof() error = %v", err)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/integrations/one-tok/executions/"+created.Execution.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("enriched status get=%d body=%s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("decode enriched status response: %v", err)
	}
	if len(fetched.UsageProofs) != 1 || fetched.UsageProofs[0].ProofRef != "usage://proof-1" {
		t.Fatalf("unexpected usage proofs: %+v", fetched.UsageProofs)
	}
	if len(fetched.ArtifactRefs) != 1 || fetched.ArtifactRefs[0].ArtifactRef == "" {
		t.Fatalf("unexpected artifact refs: %+v", fetched.ArtifactRefs)
	}
	if len(fetched.CallbackDeliveries) < 3 {
		t.Fatalf("expected callback deliveries to grow with events, got %+v", fetched.CallbackDeliveries)
	}

	cancelBody := []byte(`{"type":"cancel","idempotencyKey":"cancel-1","reason":"budget exhausted"}`)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/integrations/one-tok/executions/"+created.Execution.ID+"/actions", bytes.NewReader(cancelBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("cancel status=%d body=%s", rec.Code, rec.Body.String())
	}
	var cancelled struct {
		Result    string                `json:"result"`
		Execution integration.Execution `json:"execution"`
		Action    integration.Action    `json:"action"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cancelled); err != nil {
		t.Fatalf("decode cancel response: %v", err)
	}
	if cancelled.Execution.State != integration.ExecutionStateCancelled {
		t.Fatalf("cancelled execution state=%q", cancelled.Execution.State)
	}
	if cancelled.Action.Type != integration.ActionTypeCancel || cancelled.Action.State != integration.ActionStateApplied {
		t.Fatalf("cancel action=%+v", cancelled.Action)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/integrations/one-tok/executions/"+created.Execution.ID+"/actions", bytes.NewReader(cancelBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("idempotent cancel status=%d body=%s", rec.Code, rec.Body.String())
	}
	var cancelledAgain struct {
		Execution integration.Execution `json:"execution"`
		Action    integration.Action    `json:"action"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cancelledAgain); err != nil {
		t.Fatalf("decode idempotent cancel response: %v", err)
	}
	if cancelledAgain.Action.ID != cancelled.Action.ID {
		t.Fatalf("expected same action id on idempotent cancel, got %q want %q", cancelledAgain.Action.ID, cancelled.Action.ID)
	}
}

func TestHandleOneTokExecutionStatusAutoMaterializesUsageProofs(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CARRIER_ROOT", root)
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(root, "remote-control.json"))

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
		Status: integration.BindingStatusActive,
	})
	if err != nil {
		t.Fatalf("upsertIntegrationBinding() error = %v", err)
	}
	_, token, err := issueIntegrationBindingToken(binding.ID, "integration-test")
	if err != nil {
		t.Fatalf("issueIntegrationBindingToken() error = %v", err)
	}

	prevLaunch := orchestratorLaunchExecutionFn
	orchestratorLaunchExecutionFn = func(string) {}
	defer func() { orchestratorLaunchExecutionFn = prevLaunch }()

	created, err := createIntegrationExecution(binding, integration.CreateExecutionRequest{
		IdempotencyKey:      "idem-usage-1",
		ExternalExecutionID: "platform-exec-usage-1",
		Goal:                "Summarize provider usage",
		Input:               "Run tests and summarize usage",
		RequestedProvider:   "openrouter",
	})
	if err != nil {
		t.Fatalf("createIntegrationExecution() error = %v", err)
	}
	internal, found, err := getOrchestratorExecution(created.Execution.OrchestratorExecutionID)
	if err != nil || !found {
		t.Fatalf("getOrchestratorExecution() found=%v err=%v", found, err)
	}
	internal.Status = OrchestratorExecutionStatusCompleted
	internal.TaskUnits = []OrchestratorTaskUnit{{
		ID:      "task-1",
		HostID:  host.ID,
		AgentID: "main",
		Input:   "Run regression tests for provider usage estimates",
	}}
	internal.Results = []OrchestratorTaskResult{{
		TaskID:    "task-1",
		HostID:    host.ID,
		AgentID:   "main",
		Status:    OrchestratorTaskStatusCompleted,
		Output:    "Regression tests passed and produced a compact summary.",
		LatencyMs: 245,
	}}
	internal.Governance.ProviderResolutions = []ProviderGovernanceResolution{{
		HostID:   host.ID,
		AgentID:  "main",
		Provider: "openrouter",
		Model:    "gpt-4o-mini",
		Enabled:  true,
	}}
	if _, err := upsertOrchestratorExecution(internal); err != nil {
		t.Fatalf("upsertOrchestratorExecution() error = %v", err)
	}

	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/integrations/one-tok/executions/"+created.Execution.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status get=%d body=%s", rec.Code, rec.Body.String())
	}
	var fetched struct {
		Execution   integration.Execution    `json:"execution"`
		UsageProofs []integration.UsageProof `json:"usageProofs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if fetched.Execution.State != integration.ExecutionStateCompleted {
		t.Fatalf("execution state=%q want %q", fetched.Execution.State, integration.ExecutionStateCompleted)
	}
	if len(fetched.UsageProofs) != 1 {
		t.Fatalf("usage proofs=%d want 1: %+v", len(fetched.UsageProofs), fetched.UsageProofs)
	}
	if fetched.UsageProofs[0].UsageKind != "provider_cost_estimate" {
		t.Fatalf("usage kind=%q want provider_cost_estimate", fetched.UsageProofs[0].UsageKind)
	}
	if fetched.UsageProofs[0].MeterRef != "provider:openrouter:model:gpt-4o-mini" {
		t.Fatalf("meter ref=%q", fetched.UsageProofs[0].MeterRef)
	}
	if fetched.UsageProofs[0].Digest == "" {
		t.Fatalf("expected digest in usage proof: %+v", fetched.UsageProofs[0])
	}
}

func TestHandleOneTokExecutionCallbackReplay(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CARRIER_ROOT", root)
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(root, "remote-control.json"))

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
		Adapter:        "one-tok",
		Account:        "provider-org-1",
		CallbackURL:    "https://platform.invalid/callbacks",
		CallbackKeyID:  "kid-1",
		CallbackSecret: "secret-1",
		Target: integration.BindingTarget{
			HostID:        host.ID,
			AgentID:       "main",
			Backend:       "codex",
			WorkspaceRoot: "/workspace",
		},
		Status: integration.BindingStatusActive,
	})
	if err != nil {
		t.Fatalf("upsertIntegrationBinding() error = %v", err)
	}
	_, token, err := issueIntegrationBindingToken(binding.ID, "integration-test")
	if err != nil {
		t.Fatalf("issueIntegrationBindingToken() error = %v", err)
	}

	prevLaunch := orchestratorLaunchExecutionFn
	orchestratorLaunchExecutionFn = func(string) {}
	defer func() { orchestratorLaunchExecutionFn = prevLaunch }()

	created, err := createIntegrationExecution(binding, integration.CreateExecutionRequest{
		IdempotencyKey: "idem-replay-api-1",
		Goal:           "Replay callback tail",
		Input:          "Emit callback sequence",
	})
	if err != nil {
		t.Fatalf("createIntegrationExecution() error = %v", err)
	}
	evt2, err := appendIntegrationEvent(created.Execution.ID, created.Attempt.ID, "artifact.ready", map[string]interface{}{"name": "run.log"})
	if err != nil {
		t.Fatalf("appendIntegrationEvent() error = %v", err)
	}

	deliveries, err := listIntegrationCallbackDeliveriesByExecutionID(created.Execution.ID)
	if err != nil {
		t.Fatalf("listIntegrationCallbackDeliveriesByExecutionID() error = %v", err)
	}
	for _, delivery := range deliveries {
		status := integrationCallbackStatusDelivered
		if delivery.EventID == evt2.ID {
			status = integrationCallbackStatusFailed
		}
		if err := updateIntegrationCallbackDeliveryResult(delivery.ID, 1, status, ""); err != nil {
			t.Fatalf("updateIntegrationCallbackDeliveryResult(%s) error = %v", delivery.ID, err)
		}
	}

	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/one-tok/executions/"+created.Execution.ID+"/callbacks/replay", bytes.NewReader([]byte(`{"fromSequence":2}`)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("callback replay status=%d body=%s", rec.Code, rec.Body.String())
	}
	var replayed struct {
		Result             string                         `json:"result"`
		Replayed           int                            `json:"replayed"`
		CallbackDeliveries []integration.CallbackDelivery `json:"callbackDeliveries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &replayed); err != nil {
		t.Fatalf("decode replay response: %v", err)
	}
	if replayed.Result != "ok" || replayed.Replayed != 1 {
		t.Fatalf("unexpected replay payload: %+v", replayed)
	}
	if len(replayed.CallbackDeliveries) != 2 {
		t.Fatalf("unexpected callback deliveries: %+v", replayed.CallbackDeliveries)
	}
	if replayed.CallbackDeliveries[0].Status != integrationCallbackStatusDelivered || replayed.CallbackDeliveries[1].Status != integrationCallbackStatusPending {
		t.Fatalf("unexpected delivery statuses after replay: %+v", replayed.CallbackDeliveries)
	}
}

func TestHandleOneTokExecutionCallbackReplayRejectsUnknownEvent(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CARRIER_ROOT", root)
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(root, "remote-control.json"))

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
		Adapter:        "one-tok",
		Account:        "provider-org-1",
		CallbackURL:    "https://platform.invalid/callbacks",
		CallbackKeyID:  "kid-1",
		CallbackSecret: "secret-1",
		Target: integration.BindingTarget{
			HostID:        host.ID,
			AgentID:       "main",
			Backend:       "codex",
			WorkspaceRoot: "/workspace",
		},
		Status: integration.BindingStatusActive,
	})
	if err != nil {
		t.Fatalf("upsertIntegrationBinding() error = %v", err)
	}
	_, token, err := issueIntegrationBindingToken(binding.ID, "integration-test")
	if err != nil {
		t.Fatalf("issueIntegrationBindingToken() error = %v", err)
	}

	prevLaunch := orchestratorLaunchExecutionFn
	orchestratorLaunchExecutionFn = func(string) {}
	defer func() { orchestratorLaunchExecutionFn = prevLaunch }()

	created, err := createIntegrationExecution(binding, integration.CreateExecutionRequest{
		IdempotencyKey: "idem-replay-api-2",
		Goal:           "Replay callback tail",
		Input:          "Emit callback sequence",
	})
	if err != nil {
		t.Fatalf("createIntegrationExecution() error = %v", err)
	}

	mux, srv, _ := buildTestMux(t, nil)
	defer srv.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/one-tok/executions/"+created.Execution.ID+"/callbacks/replay", bytes.NewReader([]byte(`{"eventId":"evt_missing"}`)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("callback replay status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "E_INTEGRATION_CALLBACK_REPLAY_FAILED") {
		t.Fatalf("unexpected callback replay error body=%s", rec.Body.String())
	}
}
