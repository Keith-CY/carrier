package gateway

import (
	"net/http"
	"strings"
	"testing"
)

func TestHandleExecutionTemplatesListAndShow(t *testing.T) {
	mux := buildRemoteFeatureMux(t)

	listRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/templates", "")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	listPayload := decodeJSONMap(t, listRec)
	templates, _ := listPayload["templates"].([]interface{})
	if len(templates) < 4 {
		t.Fatalf("templates len=%d, want at least 4 payload=%+v", len(templates), listPayload)
	}

	showRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/templates/incident-diagnosis", "")
	if showRec.Code != http.StatusOK {
		t.Fatalf("show status=%d body=%s", showRec.Code, showRec.Body.String())
	}
	showPayload := decodeJSONMap(t, showRec)
	templateMap, _ := showPayload["template"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(templateMap["id"])); got != "incident-diagnosis" {
		t.Fatalf("template id=%q want incident-diagnosis payload=%+v", got, showPayload)
	}
	inputSchema, _ := templateMap["inputSchema"].([]interface{})
	if len(inputSchema) == 0 {
		t.Fatalf("inputSchema empty payload=%+v", showPayload)
	}
}

func TestHandleOrchestratorPlansWithTemplate(t *testing.T) {
	mux := buildRemoteFeatureMux(t)

	rec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/plans", `{
		"templateId":"rollout-smoke-check",
		"inputs":{
			"service":"carrier-api",
			"environment":"prod",
			"releaseVersion":"2026.03.09"
		},
		"provider":"openrouter",
		"hostLabels":["gpu","prod"],
		"maxConcurrency":9
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("plan status=%d body=%s", rec.Code, rec.Body.String())
	}
	payload := decodeJSONMap(t, rec)
	plan, _ := payload["plan"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(plan["templateId"])); got != "rollout-smoke-check" {
		t.Fatalf("plan templateId=%q want rollout-smoke-check payload=%+v", got, payload)
	}
	if got := strings.TrimSpace(anyToString(plan["provider"])); got != "openrouter" {
		t.Fatalf("plan provider=%q want openrouter payload=%+v", got, payload)
	}
	if got := strings.TrimSpace(anyToString(plan["goal"])); got == "" || !strings.Contains(got, "carrier-api") {
		t.Fatalf("plan goal=%q want rendered goal payload=%+v", got, payload)
	}
	taskUnits, _ := plan["taskUnits"].([]interface{})
	if len(taskUnits) != 3 {
		t.Fatalf("taskUnits len=%d want 3 payload=%+v", len(taskUnits), payload)
	}
}

func TestHandleTemplateLaunchCreatesAuthorizedExecution(t *testing.T) {
	mux := buildRemoteFeatureMux(t)
	hostID := createRemoteHostForTests(t, mux)

	orchestratorLaunchExecutionFn = func(string) {}
	t.Cleanup(func() {
		orchestratorLaunchExecutionFn = startOrchestratorExecutionAsync
	})

	rec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/templates/pr-triage/launch", `{
		"inputs":{
			"repository":"Keith-CY/carrier",
			"prNumber":"1554",
			"focus":"runtime rollback risk"
		},
		"provider":"openrouter",
		"hostIds":["`+hostID+`"],
		"maxConcurrency":4,
		"policyApprove":true,
		"actor":"carrier-cli"
	}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("launch status=%d body=%s", rec.Code, rec.Body.String())
	}
	payload := decodeJSONMap(t, rec)
	execution, _ := payload["execution"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(execution["templateId"])); got != "pr-triage" {
		t.Fatalf("execution templateId=%q want pr-triage payload=%+v", got, payload)
	}
	if got := strings.TrimSpace(anyToString(execution["status"])); got != string(OrchestratorExecutionStatusProvisioning) {
		t.Fatalf("execution status=%q want provisioning payload=%+v", got, payload)
	}
	auth, _ := execution["authorization"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(auth["approvedBy"])); got != "carrier-cli" {
		t.Fatalf("authorization.approvedBy=%q want carrier-cli auth=%+v", got, auth)
	}
}

func TestHandleTemplateLaunchRejectsMissingInputs(t *testing.T) {
	mux := buildRemoteFeatureMux(t)

	rec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/templates/issue-investigation/launch", `{
		"inputs":{"repository":"Keith-CY/carrier"}
	}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request, got %d body=%s", rec.Code, rec.Body.String())
	}
}
