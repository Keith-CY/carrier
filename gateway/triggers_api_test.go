package gateway

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHandleExecutionTriggersCRUD(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))
	mux := buildRemoteFeatureMux(t)

	createRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/triggers", `{
		"name":"PR triage comment trigger",
		"type":"github",
		"templateId":"pr-triage",
		"createdBy":"admin",
		"config":{
			"webhookSecret":"gh-secret",
			"githubCommand":"/carrier triage",
			"githubRepository":"Keith-CY/carrier",
			"inputs":{
				"repository":"{{payload.repository.full_name}}",
				"prNumber":"{{payload.issue.number}}",
				"focus":"comment from {{payload.sender.login}}"
			}
		}
	}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	createPayload := decodeJSONMap(t, createRec)
	triggerMap, _ := createPayload["trigger"].(map[string]interface{})
	triggerID := strings.TrimSpace(anyToString(triggerMap["id"]))
	if triggerID == "" {
		t.Fatalf("missing trigger id payload=%+v", createPayload)
	}
	if got := strings.TrimSpace(anyToString(triggerMap["type"])); got != "github" {
		t.Fatalf("trigger type=%q want github payload=%+v", got, createPayload)
	}
	if got := strings.TrimSpace(anyToString(triggerMap["templateId"])); got != "pr-triage" {
		t.Fatalf("templateId=%q want pr-triage payload=%+v", got, createPayload)
	}
	configMap, _ := triggerMap["config"].(map[string]interface{})
	requiredMemory, _ := configMap["requiredMemory"].([]interface{})
	if len(requiredMemory) == 0 {
		t.Fatalf("expected trigger config requiredMemory, payload=%+v", createPayload)
	}

	listRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/triggers", "")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	listPayload := decodeJSONMap(t, listRec)
	triggers, _ := listPayload["triggers"].([]interface{})
	if len(triggers) != 1 {
		t.Fatalf("triggers len=%d want 1 payload=%+v", len(triggers), listPayload)
	}

	patchRec := runJSONRequest(t, mux, http.MethodPatch, "/api/v1/triggers/"+triggerID, `{"enabled":false}`)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", patchRec.Code, patchRec.Body.String())
	}
	patchPayload := decodeJSONMap(t, patchRec)
	patched, _ := patchPayload["trigger"].(map[string]interface{})
	if patched["enabled"] != false {
		t.Fatalf("enabled=%v want false payload=%+v", patched["enabled"], patchPayload)
	}

	deleteRec := runJSONRequest(t, mux, http.MethodDelete, "/api/v1/triggers/"+triggerID, "")
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", deleteRec.Code, deleteRec.Body.String())
	}
}

func TestHandleExecutionTriggerWebhookLaunchesExecution(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))
	mux := buildRemoteFeatureMux(t)
	hostID := createRemoteHostForTests(t, mux)

	orchestratorLaunchExecutionFn = func(string) {}
	t.Cleanup(func() {
		orchestratorLaunchExecutionFn = startOrchestratorExecutionAsync
	})

	createRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/triggers", `{
		"name":"incident webhook",
		"type":"webhook",
		"templateId":"incident-diagnosis",
		"createdBy":"admin",
		"config":{
			"webhookSecret":"hook-secret",
			"hostIds":["`+hostID+`"],
			"policyApprove":true,
			"inputs":{
				"service":"{{payload.service}}",
				"environment":"{{payload.environment}}",
				"incidentSummary":"{{payload.summary}}"
			}
		}
	}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	createPayload := decodeJSONMap(t, createRec)
	triggerMap, _ := createPayload["trigger"].(map[string]interface{})
	triggerID := strings.TrimSpace(anyToString(triggerMap["id"]))
	if triggerID == "" {
		t.Fatalf("missing trigger id payload=%+v", createPayload)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/triggers/webhook/"+triggerID, strings.NewReader(`{
		"service":"checkout",
		"environment":"prod",
		"summary":"checkout api returns 502s"
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Carrier-Trigger-Secret", "hook-secret")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("webhook status=%d body=%s", rec.Code, rec.Body.String())
	}

	executions, err := listOrchestratorExecutions()
	if err != nil {
		t.Fatalf("listOrchestratorExecutions failed: %v", err)
	}
	if len(executions) != 1 {
		t.Fatalf("executions len=%d want 1", len(executions))
	}
	if got := executions[0].TemplateID; got != "incident-diagnosis" {
		t.Fatalf("templateId=%q want incident-diagnosis", got)
	}
	if got := executions[0].TriggerSource; got != "webhook" {
		t.Fatalf("triggerSource=%q want webhook", got)
	}
	if got := executions[0].TriggerID; got != triggerID {
		t.Fatalf("triggerID=%q want %q", got, triggerID)
	}
	if got := executions[0].Goal; !strings.Contains(got, "checkout") {
		t.Fatalf("goal=%q want checkout", got)
	}
}

func TestHandleExecutionTriggerGitHubIssueCommentLaunchesExecution(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))
	mux := buildRemoteFeatureMux(t)
	hostID := createRemoteHostForTests(t, mux)

	orchestratorLaunchExecutionFn = func(string) {}
	t.Cleanup(func() {
		orchestratorLaunchExecutionFn = startOrchestratorExecutionAsync
	})

	createRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/triggers", `{
		"name":"pr triage comment",
		"type":"github",
		"templateId":"pr-triage",
		"createdBy":"admin",
		"config":{
			"webhookSecret":"github-secret",
			"hostIds":["`+hostID+`"],
			"policyApprove":true,
			"githubCommand":"/carrier triage",
			"githubRepository":"Keith-CY/carrier",
			"inputs":{
				"repository":"{{payload.repository.full_name}}",
				"prNumber":"{{payload.issue.number}}",
				"focus":"comment from {{payload.sender.login}}"
			}
		}
	}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	createPayload := decodeJSONMap(t, createRec)
	triggerMap, _ := createPayload["trigger"].(map[string]interface{})
	triggerID := strings.TrimSpace(anyToString(triggerMap["id"]))
	if triggerID == "" {
		t.Fatalf("missing trigger id payload=%+v", createPayload)
	}

	body := `{
		"action":"created",
		"repository":{"full_name":"Keith-CY/carrier"},
		"issue":{"number":1554,"pull_request":{"url":"https://api.github.com/repos/Keith-CY/carrier/pulls/1554"}},
		"comment":{"body":"/carrier triage"},
		"sender":{"login":"alice"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/triggers/webhook/"+triggerID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "issue_comment")
	req.Header.Set("X-Hub-Signature-256", signGitHubWebhookBody("github-secret", body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("github webhook status=%d body=%s", rec.Code, rec.Body.String())
	}

	executions, err := listOrchestratorExecutions()
	if err != nil {
		t.Fatalf("listOrchestratorExecutions failed: %v", err)
	}
	if len(executions) != 1 {
		t.Fatalf("executions len=%d want 1", len(executions))
	}
	if got := executions[0].TriggerSource; got != "github" {
		t.Fatalf("triggerSource=%q want github", got)
	}
	if got := executions[0].Initiator; got != "github:alice" {
		t.Fatalf("initiator=%q want github:alice", got)
	}
	if got := executions[0].Goal; !strings.Contains(got, "1554") {
		t.Fatalf("goal=%q want rendered pr number", got)
	}
}

func TestRunScheduledExecutionTriggersLaunchesDueTrigger(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))
	mux := buildRemoteFeatureMux(t)
	hostID := createRemoteHostForTests(t, mux)

	orchestratorLaunchExecutionFn = func(string) {}
	t.Cleanup(func() {
		orchestratorLaunchExecutionFn = startOrchestratorExecutionAsync
	})

	saved, err := upsertExecutionTrigger(ExecutionTrigger{
		ID:         "schedule-1",
		Name:       "nightly smoke",
		Type:       ExecutionTriggerTypeSchedule,
		TemplateID: "rollout-smoke-check",
		Enabled:    true,
		CreatedBy:  "admin",
		Config: ExecutionTriggerConfig{
			Cron:          "* * * * *",
			HostIDs:       []string{hostID},
			PolicyApprove: true,
			Inputs: map[string]string{
				"service":        "carrier-api",
				"environment":    "prod",
				"releaseVersion": "2026.03.09",
			},
		},
		NextRunAt: "2026-03-09T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("upsertExecutionTrigger failed: %v", err)
	}

	if err := runScheduledExecutionTriggers(time.Date(2026, 3, 9, 0, 1, 0, 0, time.UTC), "req-schedule", &GatewayConfig{RemoteControlPlaneEnabled: true}); err != nil {
		t.Fatalf("runScheduledExecutionTriggers failed: %v", err)
	}

	triggers, err := listExecutionTriggers()
	if err != nil {
		t.Fatalf("listExecutionTriggers failed: %v", err)
	}
	if len(triggers) != 1 {
		t.Fatalf("triggers len=%d want 1", len(triggers))
	}
	if got := triggers[0].LastExecutionID; got == "" {
		t.Fatalf("LastExecutionID empty trigger=%+v", triggers[0])
	}
	if got := triggers[0].TriggeredCount; got != 1 {
		t.Fatalf("TriggeredCount=%d want 1", got)
	}
	if triggers[0].NextRunAt == saved.NextRunAt {
		t.Fatalf("NextRunAt=%q want advanced from %q", triggers[0].NextRunAt, saved.NextRunAt)
	}

	executions, err := listOrchestratorExecutions()
	if err != nil {
		t.Fatalf("listOrchestratorExecutions failed: %v", err)
	}
	if len(executions) != 1 {
		t.Fatalf("executions len=%d want 1", len(executions))
	}
	if got := executions[0].TriggerSource; got != "schedule" {
		t.Fatalf("triggerSource=%q want schedule", got)
	}
}

func signGitHubWebhookBody(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
