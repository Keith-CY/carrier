package gateway

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"carrier/shared/integration"
)

func TestDispatchPendingIntegrationCallbacksSendsSignedRequestAndMarksDelivered(t *testing.T) {
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

	var captured struct {
		Method    string
		KeyID     string
		EventID   string
		Sequence  string
		Signature string
		Body      []byte
	}
	callbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			t.Fatalf("ReadAll(callback body) error = %v", readErr)
		}
		captured.Method = r.Method
		captured.KeyID = r.Header.Get("X-Carrier-Key-Id")
		captured.EventID = r.Header.Get("X-Carrier-Event-Id")
		captured.Sequence = r.Header.Get("X-Carrier-Event-Sequence")
		captured.Signature = r.Header.Get("X-Carrier-Signature")
		captured.Body = append([]byte(nil), body...)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accepted":true}`))
	}))
	defer callbackServer.Close()

	binding, err := upsertIntegrationBinding(integration.Binding{
		Adapter:        "one-tok",
		Account:        "provider-org-1",
		CallbackURL:    callbackServer.URL,
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

	prevLaunch := orchestratorLaunchExecutionFn
	orchestratorLaunchExecutionFn = func(string) {}
	defer func() { orchestratorLaunchExecutionFn = prevLaunch }()

	created, err := createIntegrationExecution(binding, integration.CreateExecutionRequest{
		IdempotencyKey:      "idem-1",
		ExternalExecutionID: "platform-exec-1",
		Goal:                "Fix repo regression",
		Input:               "Run tests and fix failures",
		RequestedProvider:   "openrouter",
	})
	if err != nil {
		t.Fatalf("createIntegrationExecution() error = %v", err)
	}

	dispatched, err := dispatchPendingIntegrationCallbacks(context.Background(), callbackServer.Client(), 10)
	if err != nil {
		t.Fatalf("dispatchPendingIntegrationCallbacks() error = %v", err)
	}
	if dispatched != 1 {
		t.Fatalf("dispatchPendingIntegrationCallbacks()=%d want 1", dispatched)
	}
	if captured.Method != http.MethodPost {
		t.Fatalf("callback method=%q want POST", captured.Method)
	}
	if captured.KeyID != "kid-1" {
		t.Fatalf("callback key id=%q want kid-1", captured.KeyID)
	}
	if captured.EventID == "" || captured.Sequence == "" {
		t.Fatalf("expected event headers, got eventId=%q sequence=%q", captured.EventID, captured.Sequence)
	}
	expectedMAC := hmac.New(sha256.New, []byte("secret-1"))
	expectedMAC.Write(captured.Body)
	expectedSig := "sha256=" + hex.EncodeToString(expectedMAC.Sum(nil))
	if captured.Signature != expectedSig {
		t.Fatalf("callback signature=%q want %q", captured.Signature, expectedSig)
	}
	var envelope struct {
		EventID     string          `json:"eventId"`
		ExecutionID string          `json:"carrierExecutionId"`
		EventType   string          `json:"eventType"`
		Sequence    int64           `json:"sequence"`
		Payload     json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(captured.Body, &envelope); err != nil {
		t.Fatalf("decode callback body: %v", err)
	}
	if envelope.ExecutionID != created.Execution.ID || envelope.EventType != "execution.accepted" || envelope.Sequence != 1 {
		t.Fatalf("unexpected callback envelope: %+v", envelope)
	}

	deliveries, err := listIntegrationCallbackDeliveriesByExecutionID(created.Execution.ID)
	if err != nil {
		t.Fatalf("listIntegrationCallbackDeliveriesByExecutionID() error = %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("callback deliveries=%d want 1", len(deliveries))
	}
	if deliveries[0].Status != "delivered" || deliveries[0].AttemptCount != 1 || strings.TrimSpace(deliveries[0].LastError) != "" {
		t.Fatalf("unexpected delivery after dispatch: %+v", deliveries[0])
	}
}

func TestDispatchPendingIntegrationCallbacksAppliesRecommendedAction(t *testing.T) {
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

	callbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accepted":true,"recommendedAction":{"type":"pause","reason":"budget low"}}`))
	}))
	defer callbackServer.Close()

	binding, err := upsertIntegrationBinding(integration.Binding{
		Adapter:        "one-tok",
		Account:        "provider-org-1",
		CallbackURL:    callbackServer.URL,
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

	prevLaunch := orchestratorLaunchExecutionFn
	orchestratorLaunchExecutionFn = func(string) {}
	defer func() { orchestratorLaunchExecutionFn = prevLaunch }()

	created, err := createIntegrationExecution(binding, integration.CreateExecutionRequest{
		IdempotencyKey: "idem-1",
		Goal:           "Fix repo regression",
		Input:          "Run tests and fix failures",
	})
	if err != nil {
		t.Fatalf("createIntegrationExecution() error = %v", err)
	}
	internal, found, err := getOrchestratorExecution(created.Execution.OrchestratorExecutionID)
	if err != nil || !found {
		t.Fatalf("getOrchestratorExecution() found=%v err=%v", found, err)
	}
	registerOrchestratorExecutionCancel(internal.ID, func() {})
	defer unregisterOrchestratorExecutionCancel(internal.ID)

	dispatched, err := dispatchPendingIntegrationCallbacks(context.Background(), callbackServer.Client(), 10)
	if err != nil {
		t.Fatalf("dispatchPendingIntegrationCallbacks() error = %v", err)
	}
	if dispatched != 1 {
		t.Fatalf("dispatchPendingIntegrationCallbacks()=%d want 1", dispatched)
	}

	updated, _, found, err := getIntegrationExecutionByID(created.Execution.ID)
	if err != nil || !found {
		t.Fatalf("getIntegrationExecutionByID() found=%v err=%v", found, err)
	}
	if updated.State != integration.ExecutionStatePauseRequested {
		t.Fatalf("execution state=%q want %q", updated.State, integration.ExecutionStatePauseRequested)
	}
	events, err := listIntegrationEventsByExecutionID(created.Execution.ID)
	if err != nil {
		t.Fatalf("listIntegrationEventsByExecutionID() error = %v", err)
	}
	foundPauseRequested := false
	for _, event := range events {
		if event.EventType == "execution.pause_requested" {
			foundPauseRequested = true
			break
		}
	}
	if !foundPauseRequested {
		t.Fatalf("expected execution.pause_requested event, got %+v", events)
	}
	action, found, err := findIntegrationActionByExecutionAndKeyLocked(created.Execution.ID, "callback:"+events[0].ID+":pause")
	if err != nil {
		t.Fatalf("findIntegrationActionByExecutionAndKeyLocked() error = %v", err)
	}
	if !found {
		t.Fatal("expected callback-driven action record")
	}
	if action.Type != integration.ActionTypePause || action.State != integration.ActionStateApplied {
		t.Fatalf("unexpected callback action: %+v", action)
	}
}

func TestDispatchPendingIntegrationCallbacksBacksOffFailedDeliveries(t *testing.T) {
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

	callbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "temporary failure", http.StatusBadGateway)
	}))
	defer callbackServer.Close()

	binding, err := upsertIntegrationBinding(integration.Binding{
		Adapter:        "one-tok",
		Account:        "provider-org-1",
		CallbackURL:    callbackServer.URL,
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

	prevLaunch := orchestratorLaunchExecutionFn
	orchestratorLaunchExecutionFn = func(string) {}
	defer func() { orchestratorLaunchExecutionFn = prevLaunch }()

	created, err := createIntegrationExecution(binding, integration.CreateExecutionRequest{
		IdempotencyKey: "idem-backoff-1",
		Goal:           "Trigger callback retry",
		Input:          "Run tests and wait for callback retry",
	})
	if err != nil {
		t.Fatalf("createIntegrationExecution() error = %v", err)
	}

	dispatched, err := dispatchPendingIntegrationCallbacks(context.Background(), callbackServer.Client(), 10)
	if err == nil {
		t.Fatal("expected callback dispatch failure")
	}
	if dispatched != 0 {
		t.Fatalf("dispatchPendingIntegrationCallbacks()=%d want 0", dispatched)
	}

	deliveries, err := listIntegrationCallbackDeliveriesByExecutionID(created.Execution.ID)
	if err != nil {
		t.Fatalf("listIntegrationCallbackDeliveriesByExecutionID() error = %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("callback deliveries=%d want 1", len(deliveries))
	}
	if deliveries[0].Status != integrationCallbackStatusFailed || deliveries[0].AttemptCount != 1 {
		t.Fatalf("unexpected failed delivery: %+v", deliveries[0])
	}
	if strings.TrimSpace(deliveries[0].NextAttemptAt) == "" {
		t.Fatalf("expected nextAttemptAt after failure: %+v", deliveries[0])
	}

	dispatched, err = dispatchPendingIntegrationCallbacks(context.Background(), callbackServer.Client(), 10)
	if err != nil {
		t.Fatalf("immediate redispatch should skip due backoff, got err=%v", err)
	}
	if dispatched != 0 {
		t.Fatalf("immediate redispatch=%d want 0", dispatched)
	}
}

func TestDispatchPendingIntegrationCallbacksPreservesSequenceOrderPerExecution(t *testing.T) {
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

	requests := []string{}
	callbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		requests = append(requests, r.Header.Get("X-Carrier-Event-Sequence"))
		if len(requests) == 1 {
			http.Error(w, "first event blocked", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accepted":true}`))
	}))
	defer callbackServer.Close()

	binding, err := upsertIntegrationBinding(integration.Binding{
		Adapter:        "one-tok",
		Account:        "provider-org-1",
		CallbackURL:    callbackServer.URL,
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

	prevLaunch := orchestratorLaunchExecutionFn
	orchestratorLaunchExecutionFn = func(string) {}
	defer func() { orchestratorLaunchExecutionFn = prevLaunch }()

	created, err := createIntegrationExecution(binding, integration.CreateExecutionRequest{
		IdempotencyKey: "idem-sequence-1",
		Goal:           "Trigger ordered callbacks",
		Input:          "Emit multiple events in order",
	})
	if err != nil {
		t.Fatalf("createIntegrationExecution() error = %v", err)
	}
	if _, err := appendIntegrationEvent(created.Execution.ID, created.Attempt.ID, "artifact.ready", map[string]interface{}{"name": "run.log"}); err != nil {
		t.Fatalf("appendIntegrationEvent() error = %v", err)
	}

	dispatched, err := dispatchPendingIntegrationCallbacks(context.Background(), callbackServer.Client(), 10)
	if err == nil {
		t.Fatal("expected first dispatch to fail on sequence 1")
	}
	if dispatched != 0 {
		t.Fatalf("first dispatch=%d want 0", dispatched)
	}
	if len(requests) != 1 || requests[0] != "1" {
		t.Fatalf("unexpected first callback requests: %v", requests)
	}

	dispatched, err = dispatchPendingIntegrationCallbacks(context.Background(), callbackServer.Client(), 10)
	if err != nil {
		t.Fatalf("second dispatch should skip later sequence during backoff, got err=%v", err)
	}
	if dispatched != 0 {
		t.Fatalf("second dispatch=%d want 0", dispatched)
	}
	if len(requests) != 1 {
		t.Fatalf("later sequence should not have been delivered during backoff, requests=%v", requests)
	}
}

func TestReplayIntegrationCallbackDeliveriesFromSequenceResetsTail(t *testing.T) {
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

	prevLaunch := orchestratorLaunchExecutionFn
	orchestratorLaunchExecutionFn = func(string) {}
	defer func() { orchestratorLaunchExecutionFn = prevLaunch }()

	created, err := createIntegrationExecution(binding, integration.CreateExecutionRequest{
		IdempotencyKey: "idem-replay-1",
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
	evt3, err := appendIntegrationEvent(created.Execution.ID, created.Attempt.ID, "execution.completed", map[string]interface{}{"status": "completed"})
	if err != nil {
		t.Fatalf("appendIntegrationEvent() error = %v", err)
	}

	deliveries, err := listIntegrationCallbackDeliveriesByExecutionID(created.Execution.ID)
	if err != nil {
		t.Fatalf("listIntegrationCallbackDeliveriesByExecutionID() error = %v", err)
	}
	if len(deliveries) != 3 {
		t.Fatalf("callback deliveries=%d want 3", len(deliveries))
	}
	for _, delivery := range deliveries {
		status := integrationCallbackStatusDelivered
		if delivery.EventID == evt2.ID || delivery.EventID == evt3.ID {
			status = integrationCallbackStatusFailed
		}
		if err := updateIntegrationCallbackDeliveryResult(delivery.ID, 1, status, ""); err != nil {
			t.Fatalf("updateIntegrationCallbackDeliveryResult(%s) error = %v", delivery.ID, err)
		}
	}

	replayed, err := replayIntegrationCallbackDeliveries(created.Execution.ID, 2, "")
	if err != nil {
		t.Fatalf("replayIntegrationCallbackDeliveries() error = %v", err)
	}
	if replayed != 2 {
		t.Fatalf("replayIntegrationCallbackDeliveries()=%d want 2", replayed)
	}

	deliveries, err = listIntegrationCallbackDeliveriesByExecutionID(created.Execution.ID)
	if err != nil {
		t.Fatalf("listIntegrationCallbackDeliveriesByExecutionID() after replay error = %v", err)
	}
	pendingByEvent := map[string]string{}
	for _, delivery := range deliveries {
		pendingByEvent[delivery.EventID] = delivery.Status
	}
	if pendingByEvent[deliveries[0].EventID] != integrationCallbackStatusDelivered {
		t.Fatalf("sequence 1 should remain delivered: %+v", deliveries)
	}
	if pendingByEvent[evt2.ID] != integrationCallbackStatusPending || pendingByEvent[evt3.ID] != integrationCallbackStatusPending {
		t.Fatalf("tail deliveries should be pending after replay: %+v", deliveries)
	}
}
