package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func anyToBool(value interface{}) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func runJSONRequestWithToken(t *testing.T, mux http.Handler, token, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func testRBACGatewayConfig() *GatewayConfig {
	return &GatewayConfig{
		APIToken:                  "admin-token",
		MaxCommandBodyBytes:       64 * 1024,
		RemoteControlPlaneEnabled: true,
		RemoteChatEnabled:         true,
		ProviderBindingEnabled:    true,
		RoleTokens: map[string]GatewayRole{
			"viewer-token":   GatewayRoleViewer,
			"operator-token": GatewayRoleOperator,
			"approver-token": GatewayRoleApprover,
		},
	}
}

func TestExecutionRBAC(t *testing.T) {
	mux := buildRemoteFeatureMuxWithConfigAndDaemonHandlers(t, testRBACGatewayConfig(), nil)
	body := `{
		"goal":"collect logs",
		"requiredWorkers":[{"hostId":"local","agentId":"zeroclaw","count":1}],
		"taskUnits":[{"id":"task-1","input":"collect logs","hostId":"local","agentId":"zeroclaw"}]
	}`

	viewerRec := runJSONRequestWithToken(t, mux, "viewer-token", http.MethodPost, "/api/v1/orchestrator/executions", body)
	if viewerRec.Code != http.StatusForbidden {
		t.Fatalf("viewer create status=%d body=%s", viewerRec.Code, viewerRec.Body.String())
	}
	viewerPayload := decodeJSONMap(t, viewerRec)
	errMap, _ := viewerPayload["error"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(errMap["code"])); got != "E_RBAC_EXECUTION_LAUNCH" {
		t.Fatalf("viewer create error code=%q payload=%+v", got, viewerPayload)
	}

	operatorRec := runJSONRequestWithToken(t, mux, "operator-token", http.MethodPost, "/api/v1/orchestrator/executions", body)
	if operatorRec.Code != http.StatusCreated {
		t.Fatalf("operator create status=%d body=%s", operatorRec.Code, operatorRec.Body.String())
	}
	operatorPayload := decodeJSONMap(t, operatorRec)
	execMap, _ := operatorPayload["execution"].(map[string]interface{})
	execID := strings.TrimSpace(anyToString(execMap["id"]))
	if execID == "" {
		t.Fatalf("missing execution id payload=%+v", operatorPayload)
	}

	operatorAuthorize := runJSONRequestWithToken(t, mux, "operator-token", http.MethodPost, "/api/v1/orchestrator/executions/"+execID+"/authorize", `{}`)
	if operatorAuthorize.Code != http.StatusForbidden {
		t.Fatalf("operator authorize status=%d body=%s", operatorAuthorize.Code, operatorAuthorize.Body.String())
	}
	operatorAuthorizePayload := decodeJSONMap(t, operatorAuthorize)
	authErrMap, _ := operatorAuthorizePayload["error"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(authErrMap["code"])); got != "E_RBAC_EXECUTION_APPROVE" {
		t.Fatalf("operator authorize error code=%q payload=%+v", got, operatorAuthorizePayload)
	}

	approverAuthorize := runJSONRequestWithToken(t, mux, "approver-token", http.MethodPost, "/api/v1/orchestrator/executions/"+execID+"/authorize", `{}`)
	if approverAuthorize.Code != http.StatusOK {
		t.Fatalf("approver authorize status=%d body=%s", approverAuthorize.Code, approverAuthorize.Body.String())
	}
}

func TestPolicyRBAC(t *testing.T) {
	mux := buildRemoteFeatureMuxWithConfigAndDaemonHandlers(t, testRBACGatewayConfig(), nil)
	body := `{"name":"review prod","action":"ask","hostLabels":["prod"]}`

	operatorRec := runJSONRequestWithToken(t, mux, "operator-token", http.MethodPost, "/api/v1/orchestrator/policies", body)
	if operatorRec.Code != http.StatusForbidden {
		t.Fatalf("operator policy create status=%d body=%s", operatorRec.Code, operatorRec.Body.String())
	}
	operatorPayload := decodeJSONMap(t, operatorRec)
	errMap, _ := operatorPayload["error"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(errMap["code"])); got != "E_RBAC_POLICY_MANAGE" {
		t.Fatalf("operator policy error code=%q payload=%+v", got, operatorPayload)
	}

	adminRec := runJSONRequestWithToken(t, mux, "admin-token", http.MethodPost, "/api/v1/orchestrator/policies", body)
	if adminRec.Code != http.StatusOK {
		t.Fatalf("admin policy create status=%d body=%s", adminRec.Code, adminRec.Body.String())
	}

	viewerList := runJSONRequestWithToken(t, mux, "viewer-token", http.MethodGet, "/api/v1/orchestrator/policies", "")
	if viewerList.Code != http.StatusOK {
		t.Fatalf("viewer policy list status=%d body=%s", viewerList.Code, viewerList.Body.String())
	}
}

func TestRemoteHostRBAC(t *testing.T) {
	mux := buildRemoteFeatureMuxWithConfigAndDaemonHandlers(t, testRBACGatewayConfig(), nil)
	body := `{"name":"prod-host-1","host":"10.0.0.7","port":22,"user":"root","authMode":"private_key","keyPath":"/tmp/id_rsa"}`

	approverRec := runJSONRequestWithToken(t, mux, "approver-token", http.MethodPost, "/api/v1/remote/hosts", body)
	if approverRec.Code != http.StatusForbidden {
		t.Fatalf("approver host create status=%d body=%s", approverRec.Code, approverRec.Body.String())
	}
	approverPayload := decodeJSONMap(t, approverRec)
	errMap, _ := approverPayload["error"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(errMap["code"])); got != "E_RBAC_HOST_MANAGE" {
		t.Fatalf("approver host error code=%q payload=%+v", got, approverPayload)
	}

	adminRec := runJSONRequestWithToken(t, mux, "admin-token", http.MethodPost, "/api/v1/remote/hosts", body)
	if adminRec.Code != http.StatusOK {
		t.Fatalf("admin host create status=%d body=%s", adminRec.Code, adminRec.Body.String())
	}

	viewerRec := runJSONRequestWithToken(t, mux, "viewer-token", http.MethodGet, "/api/v1/remote/hosts", "")
	if viewerRec.Code != http.StatusOK {
		t.Fatalf("viewer host list status=%d body=%s", viewerRec.Code, viewerRec.Body.String())
	}
}

func TestProviderRBAC(t *testing.T) {
	mux := buildRemoteFeatureMuxWithConfigAndDaemonHandlers(t, testRBACGatewayConfig(), nil)
	body := `{"name":"openrouter-default","provider":"openrouter","model":"openai/gpt-4o-mini"}`

	operatorRec := runJSONRequestWithToken(t, mux, "operator-token", http.MethodPost, "/api/v1/provider-profiles", body)
	if operatorRec.Code != http.StatusForbidden {
		t.Fatalf("operator provider create status=%d body=%s", operatorRec.Code, operatorRec.Body.String())
	}
	operatorPayload := decodeJSONMap(t, operatorRec)
	errMap, _ := operatorPayload["error"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(errMap["code"])); got != "E_RBAC_PROVIDER_MANAGE" {
		t.Fatalf("operator provider error code=%q payload=%+v", got, operatorPayload)
	}

	adminRec := runJSONRequestWithToken(t, mux, "admin-token", http.MethodPost, "/api/v1/provider-profiles", body)
	if adminRec.Code != http.StatusOK {
		t.Fatalf("admin provider create status=%d body=%s", adminRec.Code, adminRec.Body.String())
	}

	viewerRec := runJSONRequestWithToken(t, mux, "viewer-token", http.MethodGet, "/api/v1/provider-profiles", "")
	if viewerRec.Code != http.StatusOK {
		t.Fatalf("viewer provider list status=%d body=%s", viewerRec.Code, viewerRec.Body.String())
	}
}

func TestPolicyScopeEvaluation(t *testing.T) {
	timeoutMs := 60000
	retryBudget := 1
	hosts := []RemoteHost{
		{ID: "host-1", Labels: []string{"prod", "gpu"}},
	}
	rules := []OrchestratorPolicyRule{
		{Name: "allow team", Enabled: true, Priority: 10, Action: "allow", Teams: []string{"ops"}},
		{Name: "review project", Enabled: true, Priority: 20, Action: "ask", Projects: []string{"checkout"}, MaxTaskTimeoutMs: &timeoutMs, MaxRetryBudget: &retryBudget},
		{Name: "deny prod rollout", Enabled: true, Priority: 5, Action: "deny", Environments: []string{"prod"}, TemplateIDs: []string{"rollout-smoke"}, Reason: "prod rollout smoke requires manual exception"},
	}

	execution, err := normalizeOrchestratorExecution(OrchestratorExecution{
		Goal:        "run rollout smoke",
		Team:        "ops",
		Project:     "checkout",
		Environment: "prod",
		TemplateID:  "rollout-smoke",
		RequiredWorkers: []OrchestratorRequiredWorker{
			{HostID: "host-1", AgentID: "picoclaw", Count: 1},
		},
		TaskUnits: []OrchestratorTaskUnit{
			{ID: "task-1", Input: "run smoke", HostID: "host-1", AgentID: "picoclaw", TimeoutMs: 120000, RetryBudget: 2},
		},
	})
	if err != nil {
		t.Fatalf("normalize execution failed: %v", err)
	}

	denied := applyOrchestratorExecutionPolicy(execution, rules, hosts)
	if denied.Policy.Decision != orchestratorPolicyDecisionDeny || denied.Policy.MatchedRuleName != "deny prod rollout" {
		t.Fatalf("expected deny prod rollout, got policy=%+v", denied.Policy)
	}

	execution.Environment = "staging"
	asked := applyOrchestratorExecutionPolicy(execution, rules, hosts)
	if asked.Policy.Decision != orchestratorPolicyDecisionAsk || asked.Policy.MatchedRuleName != "review project" {
		t.Fatalf("expected ask review project, got policy=%+v", asked.Policy)
	}
	if asked.TaskUnits[0].TimeoutMs != timeoutMs || asked.TaskUnits[0].RetryBudget != retryBudget {
		t.Fatalf("expected ask policy clamps timeout=%d retry=%d task=%+v", timeoutMs, retryBudget, asked.TaskUnits[0])
	}
}

func TestPolicyExplainAPI(t *testing.T) {
	mux := buildRemoteFeatureMuxWithConfigAndDaemonHandlers(t, testRBACGatewayConfig(), nil)
	timeoutMs := 45000
	retryBudget := 1
	if _, err := upsertOrchestratorPolicy(OrchestratorPolicyRule{
		Name:             "review checkout prod",
		Enabled:          true,
		Action:           "ask",
		Projects:         []string{"checkout"},
		Environments:     []string{"prod"},
		MaxTaskTimeoutMs: &timeoutMs,
		MaxRetryBudget:   &retryBudget,
	}); err != nil {
		t.Fatalf("upsert policy failed: %v", err)
	}

	rec := runJSONRequestWithToken(t, mux, "viewer-token", http.MethodPost, "/api/v1/policies/evaluate", `{
		"goal":"check production rollout",
		"project":"checkout",
		"environment":"prod",
		"requiredWorkers":[{"hostId":"local","agentId":"zeroclaw","count":1}],
		"taskUnits":[{"id":"task-1","input":"check rollout","hostId":"local","agentId":"zeroclaw","timeoutMs":90000,"retryBudget":3}]
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("policy evaluate status=%d body=%s", rec.Code, rec.Body.String())
	}

	payload := decodeJSONMap(t, rec)
	evaluation, _ := payload["evaluation"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(evaluation["decision"])); got != orchestratorPolicyDecisionAsk {
		t.Fatalf("evaluation.decision=%q payload=%+v", got, payload)
	}
	matchedRule, _ := evaluation["matchedRule"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(matchedRule["name"])); got != "review checkout prod" {
		t.Fatalf("matchedRule.name=%q payload=%+v", got, payload)
	}
	requiredApprovals, _ := evaluation["requiredApprovals"].(map[string]interface{})
	if !anyToBool(requiredApprovals["policy"]) {
		t.Fatalf("expected policy approval required payload=%+v", payload)
	}
	effective, _ := evaluation["effective"].(map[string]interface{})
	if got := int(anyToFloat(effective["maxTaskTimeoutMs"])); got != timeoutMs {
		t.Fatalf("effective.maxTaskTimeoutMs=%d want %d payload=%+v", got, timeoutMs, payload)
	}
	if got := int(anyToFloat(effective["maxRetryBudget"])); got != retryBudget {
		t.Fatalf("effective.maxRetryBudget=%d want %d payload=%+v", got, retryBudget, payload)
	}
}
