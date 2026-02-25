package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func buildRemoteFeatureMux(t *testing.T) http.Handler {
	return buildRemoteFeatureMuxWithConfigAndDaemonHandlers(t, nil, nil)
}

func buildRemoteFeatureMuxWithDaemonHandlers(t *testing.T, daemonHandlers map[string]http.HandlerFunc) http.Handler {
	return buildRemoteFeatureMuxWithConfigAndDaemonHandlers(t, nil, daemonHandlers)
}

func buildRemoteFeatureMuxWithConfigAndDaemonHandlers(t *testing.T, cfg *GatewayConfig, daemonHandlers map[string]http.HandlerFunc) http.Handler {
	t.Helper()
	t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(t.TempDir(), "instances.json"))
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))
	resetRemoteMetricsForTests()

	srv := newMockDaemon(daemonHandlers)
	t.Cleanup(srv.Close)
	dc := NewDaemonClient(srv.URL, "test-token", 5*time.Second)
	sessions := NewSessionStore("", 0, nil)
	t.Cleanup(sessions.Stop)
	downloads := NewDownloadStore("", nil)
	rl := NewGatewayRateLimiter(100, 1000, time.Minute, nil)
	onboard := NewOnboardStore()
	setup := NewSetupStore()
	gatewayCfg := cfg
	if gatewayCfg == nil {
		gatewayCfg = &GatewayConfig{
			APIToken:                  "test-gateway-token",
			MaxCommandBodyBytes:       64 * 1024,
			RemoteControlPlaneEnabled: true,
			RemoteChatEnabled:         true,
			ProviderBindingEnabled:    true,
		}
	} else {
		if strings.TrimSpace(gatewayCfg.APIToken) == "" {
			gatewayCfg.APIToken = "test-gateway-token"
		}
		if gatewayCfg.MaxCommandBodyBytes <= 0 {
			gatewayCfg.MaxCommandBodyBytes = 64 * 1024
		}
	}
	return buildGatewayMux(gatewayCfg, dc, sessions, downloads, rl, onboard, setup)
}

func runJSONRequest(t *testing.T, mux http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func decodeJSONMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var out map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode json: %v; body=%s", err, rec.Body.String())
	}
	return out
}

func configureSSHRunner(t *testing.T, fn func(command string) remoteExecResult) {
	t.Helper()
	orig := sshExecRunner
	sshExecRunner = func(_ context.Context, args []string) (remoteExecResult, error) {
		if len(args) == 0 {
			return remoteExecResult{ExitCode: 1, Stderr: "missing ssh args"}, nil
		}
		cmd := args[len(args)-1]
		result := fn(cmd)
		result.Command = cmd
		return result, nil
	}
	t.Cleanup(func() {
		sshExecRunner = orig
	})
}

func createRemoteHostForTests(t *testing.T, mux http.Handler) string {
	t.Helper()
	rec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/hosts", `{
		"name":"test-host",
		"host":"127.0.0.1",
		"port":22,
		"user":"ubuntu",
		"authMode":"private_key",
		"keyPath":"~/.ssh/id_ed25519",
		"runtimeMode":"on_demand"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("create host status=%d body=%s", rec.Code, rec.Body.String())
	}
	payload := decodeJSONMap(t, rec)
	hostMap, _ := payload["host"].(map[string]interface{})
	hostID, _ := hostMap["id"].(string)
	if hostID == "" {
		t.Fatalf("missing host id in response: %v", payload)
	}
	return hostID
}

func TestRemoteHostsCRUDAndCheck(t *testing.T) {
	configureSSHRunner(t, func(command string) remoteExecResult {
		switch {
		case strings.Contains(command, "echo carrier-ssh-ok"):
			return remoteExecResult{ExitCode: 0, Stdout: "carrier-ssh-ok\n"}
		case strings.Contains(command, "command -v openclaw"):
			return remoteExecResult{ExitCode: 0}
		default:
			return remoteExecResult{ExitCode: 0}
		}
	})

	mux := buildRemoteFeatureMux(t)
	hostID := createRemoteHostForTests(t, mux)

	listRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/remote/hosts", "")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list hosts status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	listPayload := decodeJSONMap(t, listRec)
	hosts, _ := listPayload["hosts"].([]interface{})
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d payload=%v", len(hosts), listPayload)
	}

	checkRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/hosts/"+hostID+"/check", `{}`)
	if checkRec.Code != http.StatusOK {
		t.Fatalf("check host status=%d body=%s", checkRec.Code, checkRec.Body.String())
	}

	patchRec := runJSONRequest(t, mux, http.MethodPatch, "/api/v1/remote/hosts/"+hostID, `{"name":"patched-host"}`)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("patch host status=%d body=%s", patchRec.Code, patchRec.Body.String())
	}
	patchPayload := decodeJSONMap(t, patchRec)
	hostMap, _ := patchPayload["host"].(map[string]interface{})
	if hostMap["name"] != "patched-host" {
		t.Fatalf("expected patched host name, payload=%v", patchPayload)
	}

	deleteRec := runJSONRequest(t, mux, http.MethodDelete, "/api/v1/remote/hosts/"+hostID, "")
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete host status=%d body=%s", deleteRec.Code, deleteRec.Body.String())
	}
}

func TestProviderProfilesPatchAndBinding(t *testing.T) {
	configureSSHRunner(t, func(command string) remoteExecResult {
		switch {
		case strings.Contains(command, "cat \"$HOME/.openclaw/openclaw.json\""):
			return remoteExecResult{ExitCode: 0, Stdout: `{"agents":{"defaults":{"provider":"openai","model":"gpt-4.1"}}}`}
		case strings.Contains(command, "cat > \"$HOME/.openclaw/openclaw.json\""):
			return remoteExecResult{ExitCode: 0}
		default:
			return remoteExecResult{ExitCode: 0}
		}
	})

	mux := buildRemoteFeatureMux(t)
	hostID := createRemoteHostForTests(t, mux)

	createProfileRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/provider-profiles", `{
		"name":"openai-gpt5",
		"provider":"openai",
		"model":"gpt-5",
		"enabled":false
	}`)
	if createProfileRec.Code != http.StatusOK {
		t.Fatalf("create profile status=%d body=%s", createProfileRec.Code, createProfileRec.Body.String())
	}
	createPayload := decodeJSONMap(t, createProfileRec)
	profile, _ := createPayload["profile"].(map[string]interface{})
	profileID, _ := profile["id"].(string)
	if profileID == "" {
		t.Fatalf("missing profile id: %v", createPayload)
	}
	if enabled, _ := profile["enabled"].(bool); enabled {
		t.Fatalf("expected profile enabled=false at creation")
	}

	patchModelRec := runJSONRequest(t, mux, http.MethodPatch, "/api/v1/provider-profiles/"+profileID, `{"model":"gpt-5-mini"}`)
	if patchModelRec.Code != http.StatusOK {
		t.Fatalf("patch profile model status=%d body=%s", patchModelRec.Code, patchModelRec.Body.String())
	}
	patchModelPayload := decodeJSONMap(t, patchModelRec)
	patchModelProfile, _ := patchModelPayload["profile"].(map[string]interface{})
	if enabled, _ := patchModelProfile["enabled"].(bool); enabled {
		t.Fatalf("expected enabled to remain false when omitted from patch")
	}

	patchEnabledRec := runJSONRequest(t, mux, http.MethodPatch, "/api/v1/provider-profiles/"+profileID, `{"enabled":true}`)
	if patchEnabledRec.Code != http.StatusOK {
		t.Fatalf("patch profile enabled status=%d body=%s", patchEnabledRec.Code, patchEnabledRec.Body.String())
	}
	patchEnabledPayload := decodeJSONMap(t, patchEnabledRec)
	patchEnabledProfile, _ := patchEnabledPayload["profile"].(map[string]interface{})
	if enabled, _ := patchEnabledProfile["enabled"].(bool); !enabled {
		t.Fatalf("expected enabled=true after explicit patch")
	}

	bindRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/provider-bindings", `{
		"profileId":"`+profileID+`",
		"targetType":"host",
		"targetId":"`+hostID+`",
		"syncMode":"always_push"
	}`)
	if bindRec.Code != http.StatusOK {
		t.Fatalf("create binding status=%d body=%s", bindRec.Code, bindRec.Body.String())
	}

	listBindingsRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/provider-bindings", "")
	if listBindingsRec.Code != http.StatusOK {
		t.Fatalf("list bindings status=%d body=%s", listBindingsRec.Code, listBindingsRec.Body.String())
	}
	listBindingsPayload := decodeJSONMap(t, listBindingsRec)
	bindings, _ := listBindingsPayload["bindings"].([]interface{})
	if len(bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d payload=%v", len(bindings), listBindingsPayload)
	}
	bindingMap, _ := bindings[0].(map[string]interface{})
	bindingID, _ := bindingMap["id"].(string)
	if bindingID == "" {
		t.Fatalf("expected binding id in payload=%v", listBindingsPayload)
	}

	deleteBindingRec := runJSONRequest(t, mux, http.MethodDelete, "/api/v1/provider-bindings/"+bindingID, "")
	if deleteBindingRec.Code != http.StatusOK {
		t.Fatalf("delete binding status=%d body=%s", deleteBindingRec.Code, deleteBindingRec.Body.String())
	}

	listBindingsAfterDeleteRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/provider-bindings", "")
	if listBindingsAfterDeleteRec.Code != http.StatusOK {
		t.Fatalf("list bindings after delete status=%d body=%s", listBindingsAfterDeleteRec.Code, listBindingsAfterDeleteRec.Body.String())
	}
	listAfterDeletePayload := decodeJSONMap(t, listBindingsAfterDeleteRec)
	bindingsAfterDelete, _ := listAfterDeletePayload["bindings"].([]interface{})
	if len(bindingsAfterDelete) != 0 {
		t.Fatalf("expected 0 bindings after delete, got %d payload=%v", len(bindingsAfterDelete), listAfterDeletePayload)
	}
}

func TestRemoteChatStreamSSE(t *testing.T) {
	configureSSHRunner(t, func(command string) remoteExecResult {
		switch {
		case strings.Contains(command, "command -v openclaw"):
			return remoteExecResult{ExitCode: 0}
		case strings.Contains(command, "openclaw agent --local"):
			return remoteExecResult{ExitCode: 0, Stdout: `{"message":"hello from remote","sessionId":"sess-123"}`}
		default:
			return remoteExecResult{ExitCode: 0}
		}
	})

	mux := buildRemoteFeatureMux(t)
	hostID := createRemoteHostForTests(t, mux)

	rec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/chat/stream", `{
		"hostId":"`+hostID+`",
		"agentId":"main",
		"message":"hello"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("remote chat status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"type":"text-delta"`) {
		t.Fatalf("expected text-delta event in stream body: %s", body)
	}
	if !strings.Contains(body, `"type":"session"`) {
		t.Fatalf("expected session event in stream body: %s", body)
	}
	if !strings.Contains(body, `"type":"finish"`) {
		t.Fatalf("expected finish event in stream body: %s", body)
	}

	unifiedRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/chat/stream", `{
		"target":"remote",
		"hostId":"`+hostID+`",
		"agentId":"main",
		"message":"hello"
	}`)
	if unifiedRec.Code != http.StatusOK {
		t.Fatalf("unified remote chat status=%d body=%s", unifiedRec.Code, unifiedRec.Body.String())
	}
	unifiedBody := unifiedRec.Body.String()
	if !strings.Contains(unifiedBody, `"type":"text-delta"`) || !strings.Contains(unifiedBody, `"type":"finish"`) {
		t.Fatalf("expected sse events in unified remote stream body: %s", unifiedBody)
	}
}

func TestRemoteChatDisabledWhenRemoteControlFlagOff(t *testing.T) {
	mux := buildRemoteFeatureMuxWithConfigAndDaemonHandlers(t, &GatewayConfig{
		APIToken:                  "test-gateway-token",
		MaxCommandBodyBytes:       64 * 1024,
		RemoteControlPlaneEnabled: false,
		RemoteChatEnabled:         true, // intentionally true; should be normalized off.
		ProviderBindingEnabled:    true,
	}, nil)

	rec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/chat/stream", `{
		"hostId":"host-1",
		"agentId":"main",
		"message":"hello"
	}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected remote chat disabled status=404, got status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "remote chat is disabled") {
		t.Fatalf("expected disabled message, got body=%s", rec.Body.String())
	}

	unifiedRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/chat/stream", `{
		"target":"remote",
		"hostId":"host-1",
		"agentId":"main",
		"message":"hello"
	}`)
	if unifiedRec.Code != http.StatusNotFound {
		t.Fatalf("expected unified remote chat disabled status=404, got status=%d body=%s", unifiedRec.Code, unifiedRec.Body.String())
	}
}

func TestUnifiedChatStreamLocal(t *testing.T) {
	mux := buildRemoteFeatureMuxWithDaemonHandlers(t, map[string]http.HandlerFunc{
		"POST /api/v1/base-agent/chat": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"message":"hello from local base-agent","action":"chat"}`))
		},
	})

	rec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/chat/stream", `{
		"target":"local",
		"message":"what is the status",
		"sessionId":"local-session-1",
		"provider":"webui",
		"chatId":"local-chat-1"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("unified local chat status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"type":"text-delta"`) {
		t.Fatalf("expected text-delta event in stream body: %s", body)
	}
	if !strings.Contains(body, `"sessionId":"local-session-1"`) {
		t.Fatalf("expected session event in stream body: %s", body)
	}
	if !strings.Contains(body, `"type":"finish"`) {
		t.Fatalf("expected finish event in stream body: %s", body)
	}
}

func TestUnifiedChatStreamLocalInstance(t *testing.T) {
	mux := buildRemoteFeatureMuxWithDaemonHandlers(t, map[string]http.HandlerFunc{
		"POST /api/v1/agents/openclaw/chat": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"agentId":"openclaw","sessionId":"sess-local-inst-1","message":"hello from local instance"}`))
		},
	})

	rec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/chat/stream", `{
		"target":"local",
		"agentId":"openclaw",
		"message":"hello local instance",
		"sessionId":"sess-local-inst-1"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("unified local instance chat status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"type":"text-delta"`) {
		t.Fatalf("expected text-delta event in stream body: %s", body)
	}
	if !strings.Contains(body, `"sessionId":"sess-local-inst-1"`) {
		t.Fatalf("expected session event in stream body: %s", body)
	}
	if !strings.Contains(body, `"type":"finish"`) {
		t.Fatalf("expected finish event in stream body: %s", body)
	}
}

func TestRemoteMetricsEndpointTracksOperations(t *testing.T) {
	chatCalls := 0
	configureSSHRunner(t, func(command string) remoteExecResult {
		switch {
		case strings.Contains(command, "echo carrier-ssh-ok"):
			return remoteExecResult{ExitCode: 0, Stdout: "carrier-ssh-ok\n"}
		case strings.Contains(command, "command -v openclaw"):
			return remoteExecResult{ExitCode: 0}
		case strings.Contains(command, "openclaw agent --local"):
			chatCalls++
			if chatCalls == 1 {
				return remoteExecResult{ExitCode: 0, Stdout: `{"message":"ok","sessionId":"sess-1"}`}
			}
			return remoteExecResult{ExitCode: 1, Stderr: "chat failed"}
		default:
			return remoteExecResult{ExitCode: 0}
		}
	})

	mux := buildRemoteFeatureMux(t)
	hostID := createRemoteHostForTests(t, mux)

	checkRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/hosts/"+hostID+"/check", `{}`)
	if checkRec.Code != http.StatusOK {
		t.Fatalf("check host status=%d body=%s", checkRec.Code, checkRec.Body.String())
	}

	chatOK := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/chat/stream", `{
		"hostId":"`+hostID+`",
		"agentId":"main",
		"message":"hello"
	}`)
	if chatOK.Code != http.StatusOK {
		t.Fatalf("chat success status=%d body=%s", chatOK.Code, chatOK.Body.String())
	}

	chatFail := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/chat/stream", `{
		"hostId":"`+hostID+`",
		"agentId":"main",
		"message":"hello again"
	}`)
	if chatFail.Code != http.StatusBadGateway {
		t.Fatalf("chat failure status=%d body=%s", chatFail.Code, chatFail.Body.String())
	}

	metricsRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/remote/metrics", "")
	if metricsRec.Code != http.StatusOK {
		t.Fatalf("metrics status=%d body=%s", metricsRec.Code, metricsRec.Body.String())
	}
	payload := decodeJSONMap(t, metricsRec)
	metrics, _ := payload["metrics"].(map[string]interface{})
	if metrics == nil {
		t.Fatalf("missing metrics payload: %v", payload)
	}
	ops, _ := metrics["operations"].(map[string]interface{})
	if ops == nil {
		t.Fatalf("missing operations metrics: %v", metrics)
	}
	hostCheckRaw, _ := ops["host_check"].(map[string]interface{})
	if hostCheckRaw == nil || int(hostCheckRaw["total"].(float64)) < 1 {
		t.Fatalf("expected host_check total >= 1, got %v", hostCheckRaw)
	}

	chatRaw, _ := metrics["chatStream"].(map[string]interface{})
	if chatRaw == nil {
		t.Fatalf("missing chatStream metrics: %v", metrics)
	}
	if int(chatRaw["total"].(float64)) != 2 {
		t.Fatalf("expected chat total=2, got %v", chatRaw)
	}
	if int(chatRaw["failure"].(float64)) != 1 {
		t.Fatalf("expected chat failure=1, got %v", chatRaw)
	}
	if chatRaw["failureRate"].(float64) <= 0 {
		t.Fatalf("expected positive chat failure rate, got %v", chatRaw)
	}

	rolloutRaw, _ := metrics["rollout"].(map[string]interface{})
	if rolloutRaw == nil {
		t.Fatalf("missing rollout metrics: %v", metrics)
	}
	if rolloutRaw["state"] != "canary" {
		t.Fatalf("expected rollout state canary for chat failure sample, got %v", rolloutRaw)
	}
	if canPromote, ok := rolloutRaw["canPromote"].(bool); !ok || canPromote {
		t.Fatalf("expected canPromote=false for canary state, got %v", rolloutRaw)
	}
}

func TestRemoteMetricsEndpointMethodGuard(t *testing.T) {
	mux := buildRemoteFeatureMux(t)

	rec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/remote/metrics", `{}`)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got status=%d body=%s", rec.Code, rec.Body.String())
	}
}
