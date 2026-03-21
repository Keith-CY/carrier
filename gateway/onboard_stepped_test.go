package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestHandleOnboard_StatusAndCancel(t *testing.T) {
	_, daemon, _, _, store := setupTestEnv(t, map[string]http.HandlerFunc{
		"GET /api/v1/agents/status": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"statuses": []map[string]interface{}{
					{
						"id":           "agent-1",
						"name":         "Agent 1",
						"installState": "installed",
						"runtimeState": "running",
						"health":       "healthy",
					},
				},
			})
		},
	})

	resp := handleOnboard(context.Background(), &GatewayCommand{
		Provider:  "telegram",
		ChatID:    "42",
		RequestID: "req-status",
		Args:      []string{"status"},
	}, daemon, store)
	if resp.Result != "ok" || !strings.Contains(resp.Message, "Agent status:") {
		t.Fatalf("expected status response, got %+v", resp)
	}

	sessionKey := "telegram:42"
	store.start(sessionKey)
	store.update(sessionKey, func(s *OnboardSession) { s.Step = OnboardAgentSelected })
	resp = handleOnboard(context.Background(), &GatewayCommand{
		Provider:  "telegram",
		ChatID:    "42",
		RequestID: "req-cancel",
		Args:      []string{"cancel"},
	}, daemon, store)
	if resp.Result != "ok" || !strings.Contains(resp.Message, "cancelled") {
		t.Fatalf("expected cancel response, got %+v", resp)
	}
}

func TestHandleOnboard_StartAndSelectionShortcut(t *testing.T) {
	_, daemon, _, _, store := setupTestEnv(t, map[string]http.HandlerFunc{
		"GET /api/v1/agents": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"agents": []map[string]interface{}{
					{"id": "openclaw", "name": "OpenClaw", "installState": "not_installed"},
					{"id": "worker", "name": "Worker", "installState": "installed"},
				},
			})
		},
	})

	resp := handleOnboard(context.Background(), &GatewayCommand{
		Provider:  "telegram",
		ChatID:    "77",
		RequestID: "req-start",
		Args:      nil,
	}, daemon, store)
	if resp.Result != "ok" || !strings.Contains(resp.Message, "Available agents:") {
		t.Fatalf("expected interactive start response, got %+v", resp)
	}

	resp = handleOnboard(context.Background(), &GatewayCommand{
		Provider:  "telegram",
		ChatID:    "77",
		RequestID: "req-select",
		Args:      []string{"openclaw"},
	}, daemon, store)
	if resp.Result != "ok" || !strings.Contains(resp.Message, "Choose a channel for OpenClaw") {
		t.Fatalf("expected managed channel selection response, got %+v", resp)
	}
	if got := store.get("telegram:77").Step; got != OnboardChannelSelect {
		t.Fatalf("expected channel select step, got %q", got)
	}
}

func TestHandleOnboard_RoutesActiveSessionReply(t *testing.T) {
	_, daemon, _, _, store := setupTestEnv(t, nil)
	sessionKey := "telegram:active"
	store.start(sessionKey)
	store.update(sessionKey, func(s *OnboardSession) {
		s.Step = OnboardChannelSelect
		s.SelectedAgent = "openclaw"
		s.SelectedAgentName = "OpenClaw"
	})

	resp := handleOnboard(context.Background(), &GatewayCommand{
		Provider:  "telegram",
		ChatID:    "active",
		RequestID: "req-active",
		Args:      []string{"telegram"},
	}, daemon, store)
	if resp.Result != "ok" || !strings.Contains(resp.Message, "skips bot token input") {
		t.Fatalf("expected token-skip response, got %+v", resp)
	}
	sess := store.get(sessionKey)
	if sess.Step != OnboardAgentSelected || !sess.ChannelSetupPending || sess.ChannelToken != "" {
		t.Fatalf("unexpected session state after channel select: %+v", sess)
	}
}

func TestOnboardStartAndReply_Branches(t *testing.T) {
	_, daemon, _, _, store := setupTestEnv(t, map[string]http.HandlerFunc{
		"GET /api/v1/agents": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"agents": []map[string]interface{}{
					{"id": "worker", "name": "Worker"},
				},
			})
		},
	})

	startResp := onboardStart(context.Background(), "req-start", "telegram:r", daemon, store, "telegram:r")
	if startResp.Result != "ok" || !strings.Contains(startResp.Message, "Welcome to Carrier") {
		t.Fatalf("expected successful start response, got %+v", startResp)
	}

	replyFromMissing := onboardReply(context.Background(), "req-reply-missing", "telegram:missing", []string{"worker"}, daemon, store, "telegram:missing")
	if replyFromMissing.Result != "ok" || !strings.Contains(replyFromMissing.Message, "Available agents:") {
		t.Fatalf("expected missing session to fallback to start, got %+v", replyFromMissing)
	}

	store.start("telegram:installing")
	store.update("telegram:installing", func(s *OnboardSession) { s.Step = OnboardInstalling })
	installingResp := onboardReply(context.Background(), "req-installing", "telegram:installing", nil, daemon, store, "telegram:installing")
	if installingResp.Result != "ok" || !strings.Contains(installingResp.Message, "Installation is in progress") {
		t.Fatalf("expected installing response, got %+v", installingResp)
	}

	store.start("telegram:done")
	store.update("telegram:done", func(s *OnboardSession) { s.Step = OnboardDone })
	doneResp := onboardReply(context.Background(), "req-done", "telegram:done", nil, daemon, store, "telegram:done")
	if doneResp.Result != "ok" || !strings.Contains(doneResp.Message, "Onboarding is complete") {
		t.Fatalf("expected done response, got %+v", doneResp)
	}

	store.start("telegram:bad")
	store.update("telegram:bad", func(s *OnboardSession) { s.Step = OnboardStep("bad-state") })
	errResp := onboardReply(context.Background(), "req-bad", "telegram:bad", nil, daemon, store, "telegram:bad")
	if errResp.Result != "error" || errResp.ErrorCode != "E_USAGE" {
		t.Fatalf("expected usage error for bad state, got %+v", errResp)
	}
}

func TestOnboardStart_Error(t *testing.T) {
	_, daemon, _, _, store := setupTestEnv(t, map[string]http.HandlerFunc{
		"GET /api/v1/agents": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"code":    "E_AGENT_NOT_FOUND",
					"message": "missing",
				},
			})
		},
	})

	resp := onboardStart(context.Background(), "req-start-error", "telegram:e", daemon, store, "telegram:e")
	if resp.Result != "error" || resp.ErrorCode != "E_AGENT_NOT_FOUND" {
		t.Fatalf("expected mapped daemon error, got %+v", resp)
	}
}

func TestOnboardSelectAgent_Branches(t *testing.T) {
	store := NewOnboardStore()
	resp := onboardSelectAgent(context.Background(), "req-empty", "telegram:a", "", nil, store, "telegram:a")
	if resp.Result != "error" || resp.ErrorCode != "E_USAGE" {
		t.Fatalf("expected usage error on empty agent id, got %+v", resp)
	}

	_, daemonErr, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
		"GET /api/v1/agents": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": map[string]string{"code": "E_COMMAND_FAILED", "message": "boom"}})
		},
	})
	resp = onboardSelectAgent(context.Background(), "req-daemon", "telegram:a", "openclaw", daemonErr, store, "telegram:a")
	if resp.Result != "error" || resp.ErrorCode == "" {
		t.Fatalf("expected daemon error response, got %+v", resp)
	}

	_, daemonOK, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
		"GET /api/v1/agents": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"agents": []map[string]interface{}{
					{"id": "openclaw", "name": "OpenClaw"},
					{"id": "worker", "name": "Worker"},
				},
			})
		},
	})

	resp = onboardSelectAgent(context.Background(), "req-not-found", "telegram:a", "missing", daemonOK, store, "telegram:a")
	if resp.Result != "error" || resp.ErrorCode != "E_AGENT_NOT_FOUND" {
		t.Fatalf("expected not found response, got %+v", resp)
	}

	resp = onboardSelectAgent(context.Background(), "req-worker", "telegram:a", "worker", daemonOK, store, "telegram:a")
	if resp.Result != "ok" || !strings.Contains(resp.Message, "Choose an LLM Provider") {
		t.Fatalf("expected provider list response, got %+v", resp)
	}
	if got := store.get("telegram:a").Step; got != OnboardAgentSelected {
		t.Fatalf("expected agent selected step, got %q", got)
	}

	resp = onboardSelectAgent(context.Background(), "req-openclaw", "telegram:a", "openclaw", daemonOK, store, "telegram:a")
	if resp.Result != "ok" || !strings.Contains(resp.Message, "Choose a channel for OpenClaw") {
		t.Fatalf("expected managed channel prompt, got %+v", resp)
	}
	if got := store.get("telegram:a").Step; got != OnboardChannelSelect {
		t.Fatalf("expected channel select step for managed agent, got %q", got)
	}
}

func TestOnboardCaptureChannelToken_Branches(t *testing.T) {
	store := NewOnboardStore()
	resp := onboardCaptureChannelToken("req-nil", "telegram:nil", "token", store)
	if resp.Result != "error" || resp.ErrorCode != "E_USAGE" {
		t.Fatalf("expected usage error for missing session, got %+v", resp)
	}

	store.start("telegram:token")
	store.update("telegram:token", func(s *OnboardSession) {
		s.Step = OnboardChannelToken
		s.SelectedAgent = "openclaw"
		s.SelectedAgentName = "OpenClaw"
		s.SelectedChannel = "telegram"
	})
	resp = onboardCaptureChannelToken("req-skip", "telegram:token", "   ", store)
	if resp.Result != "ok" || !strings.Contains(resp.Message, "disabled to protect secrets") {
		t.Fatalf("expected token-skip response, got %+v", resp)
	}

	sess := store.get("telegram:token")
	if sess.ChannelToken != "" || sess.Step != OnboardAgentSelected || !sess.ChannelSetupPending {
		t.Fatalf("unexpected session after token skip: %+v", sess)
	}

	store.start("telegram:token-worker")
	store.update("telegram:token-worker", func(s *OnboardSession) {
		s.Step = OnboardChannelToken
		s.SelectedAgent = "worker"
	})
	resp = onboardCaptureChannelToken("req-worker", "telegram:token-worker", "t-123", store)
	if resp.Result != "error" || resp.ErrorCode != "E_USAGE" {
		t.Fatalf("expected usage error for non-managed agent, got %+v", resp)
	}
}

func TestBuildProviderListResponse(t *testing.T) {
	resp := buildProviderListResponse("req-provider-list", &AgentState{ID: "worker", Name: "Worker"})
	if resp.Result != "ok" {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	expectContains := []string{
		"Choose an LLM Provider",
		"`anthropic`",
		"[API key]",
		"`openai-codex`",
		"[OAuth device code]",
		"`openai-compatible`",
		"`ollama`",
		"[no auth]",
	}
	for _, needle := range expectContains {
		if !strings.Contains(resp.Message, needle) {
			t.Fatalf("provider list should contain %q, message=%q", needle, resp.Message)
		}
	}
}

func TestAuthModeBadge_AllModes(t *testing.T) {
	cases := []struct {
		mode AuthMode
		want string
	}{
		{AuthModeAPIKey, "[API key]"},
		{AuthModeOAuthDeviceCode, "[OAuth device code]"},
		{AuthModeOAuthPlugin, "[OAuth plugin]"},
		{AuthModeGcloudADC, "[gcloud ADC]"},
		{AuthModeNone, "[no auth]"},
		{AuthMode("unknown"), ""},
	}
	for _, tc := range cases {
		if got := authModeBadge(tc.mode); got != tc.want {
			t.Fatalf("authModeBadge(%q) = %q, want %q", tc.mode, got, tc.want)
		}
	}
}

func TestRenderManagedChannelPrompt(t *testing.T) {
	if got := renderManagedChannelPrompt("unknown-agent"); !strings.Contains(got, "Unsupported managed agent.") {
		t.Fatalf("expected unsupported message, got %q", got)
	}
	if got := renderManagedChannelPrompt("openclaw"); !strings.Contains(got, "Choose a channel for OpenClaw") {
		t.Fatalf("expected openclaw channel prompt, got %q", got)
	} else if !strings.Contains(got, "configure it later in Web UI") {
		t.Fatalf("expected web ui guidance in channel prompt, got %q", got)
	}
}

func TestOnboardConfirm_Branches(t *testing.T) {
	store := NewOnboardStore()
	store.start("telegram:confirm")
	store.update("telegram:confirm", func(s *OnboardSession) {
		s.Step = OnboardEnvConfigured
		s.SelectedAgent = "worker"
	})

	resp := onboardConfirm(context.Background(), "req-back", "telegram:confirm", "no", nil, store, "telegram:confirm")
	if resp.Result != "ok" || !strings.Contains(resp.Message, "Going back") {
		t.Fatalf("expected back response, got %+v", resp)
	}
	if got := store.get("telegram:confirm").Step; got != OnboardAuthConfigured {
		t.Fatalf("expected auth configured after back, got %q", got)
	}

	resp = onboardConfirm(context.Background(), "req-usage", "telegram:confirm", "maybe", nil, store, "telegram:confirm")
	if resp.Result != "error" || resp.ErrorCode != "E_USAGE" {
		t.Fatalf("expected usage error, got %+v", resp)
	}

	resp = onboardConfirm(context.Background(), "req-nosess", "telegram:missing", "yes", nil, store, "telegram:missing")
	if resp.Result != "error" || resp.ErrorCode != "E_USAGE" {
		t.Fatalf("expected no-session error, got %+v", resp)
	}
}

func TestOnboardConfirm_EnvApplyError(t *testing.T) {
	store := NewOnboardStore()
	store.start("telegram:badenv")
	store.update("telegram:badenv", func(s *OnboardSession) {
		s.Step = OnboardEnvConfigured
		s.SelectedAgent = "worker"
		s.EnvVars["BAD=KEY"] = "value"
	})

	resp := onboardConfirm(context.Background(), "req-badenv", "telegram:badenv", "yes", nil, store, "telegram:badenv")
	if resp.Result != "error" || resp.ErrorCode != "E_ENV" {
		t.Fatalf("expected env apply error, got %+v", resp)
	}
	if got := store.get("telegram:badenv").Step; got != OnboardEnvConfigured {
		t.Fatalf("expected rollback to env configured, got %q", got)
	}
}

func TestOnboardConfirm_InstallError(t *testing.T) {
	_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
		"POST /api/v1/agents/worker/install": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{"code": "E_AGENT_NOT_FOUND", "message": "missing"},
			})
		},
	})

	store := NewOnboardStore()
	store.start("telegram:install-fail")
	store.update("telegram:install-fail", func(s *OnboardSession) {
		s.Step = OnboardEnvConfigured
		s.SelectedAgent = "worker"
	})

	resp := onboardConfirm(context.Background(), "req-install-fail", "telegram:install-fail", "yes", daemon, store, "telegram:install-fail")
	if resp.Result != "error" || resp.ErrorCode != "E_AGENT_NOT_FOUND" {
		t.Fatalf("expected mapped install error, got %+v", resp)
	}
	if got := store.get("telegram:install-fail").Step; got != OnboardDone {
		t.Fatalf("expected step done after install error, got %q", got)
	}
}

func TestOnboardConfirm_StartError(t *testing.T) {
	_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
		"POST /api/v1/agents/worker/install": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		},
		"POST /api/v1/agents/worker/start": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{"code": "E_ALREADY_RUNNING", "message": "already running"},
			})
		},
	})

	store := NewOnboardStore()
	store.start("telegram:start-fail")
	store.update("telegram:start-fail", func(s *OnboardSession) {
		s.Step = OnboardEnvConfigured
		s.SelectedAgent = "worker"
	})

	resp := onboardConfirm(context.Background(), "req-start-fail", "telegram:start-fail", "yes", daemon, store, "telegram:start-fail")
	if resp.Result != "ok" || !strings.Contains(resp.Message, "installed but failed to start") {
		t.Fatalf("expected soft start error response, got %+v", resp)
	}
	if !strings.Contains(resp.Message, "already running") {
		t.Fatalf("expected mapped daemon message, got %q", resp.Message)
	}
}

func TestOnboardConfirm_StartError_UnknownWithDetails(t *testing.T) {
	_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
		"POST /api/v1/agents/worker/install": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		},
		"POST /api/v1/agents/worker/start": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{"code": "E_ANYTHING", "message": "provider rejected"},
			})
		},
	})

	store := NewOnboardStore()
	store.start("telegram:start-fail-details")
	store.update("telegram:start-fail-details", func(s *OnboardSession) {
		s.Step = OnboardEnvConfigured
		s.SelectedAgent = "worker"
	})

	resp := onboardConfirm(context.Background(), "req-start-fail-unknown", "telegram:start-fail-details", "yes", daemon, store, "telegram:start-fail-details")
	if resp.Result != "ok" || !strings.Contains(resp.Message, "installed but failed to start") {
		t.Fatalf("expected soft start error response, got %+v", resp)
	}
	if !strings.Contains(resp.Message, "provider rejected") {
		t.Fatalf("expected daemon detail in response, got %q", resp.Message)
	}
}

func TestOnboardConfirm_Success(t *testing.T) {
	_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
		"POST /api/v1/agents/worker/install": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		},
		"POST /api/v1/agents/worker/start": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		},
		"GET /api/v1/agents/worker/status": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"statuses": []map[string]interface{}{
					{
						"id":           "worker",
						"name":         "Worker",
						"installState": "installed",
						"runtimeState": "running",
						"health":       "healthy",
					},
				},
			})
		},
	})

	store := NewOnboardStore()
	store.start("telegram:success")
	store.update("telegram:success", func(s *OnboardSession) {
		s.Step = OnboardEnvConfigured
		s.SelectedAgent = "worker"
	})

	resp := onboardConfirm(context.Background(), "req-success", "telegram:success", "yes", daemon, store, "telegram:success")
	if resp.Result != "ok" || !strings.Contains(resp.Message, "installed and running (healthy)") {
		t.Fatalf("expected success response, got %+v", resp)
	}
	if got := store.get("telegram:success").Step; got != OnboardDone {
		t.Fatalf("expected onboarding done step, got %q", got)
	}
}

func TestOnboardStatus_Branches(t *testing.T) {
	_, daemonErr, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
		"GET /api/v1/agents/status": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": map[string]string{"code": "E_COMMAND_FAILED", "message": "boom"}})
		},
	})
	resp := onboardStatus(context.Background(), "req-status-err", daemonErr, "telegram:1")
	if resp.Result != "error" {
		t.Fatalf("expected daemon error, got %+v", resp)
	}

	_, daemonEmpty, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
		"GET /api/v1/agents/status": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"statuses": []map[string]interface{}{}})
		},
	})
	resp = onboardStatus(context.Background(), "req-status-empty", daemonEmpty, "telegram:1")
	if resp.Result != "ok" || resp.Message != "No agents configured." {
		t.Fatalf("expected no agents response, got %+v", resp)
	}

	_, daemonOK, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
		"GET /api/v1/agents/status": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"statuses": []map[string]interface{}{
					{
						"id":           "a-healthy",
						"name":         "Healthy",
						"installState": "installed",
						"runtimeState": "running",
						"health":       "healthy",
					},
					{
						"id":           "a-stopped",
						"name":         "Stopped",
						"installState": "installed",
						"runtimeState": "stopped",
						"health":       "unknown",
					},
					{
						"id":           "a-bad",
						"name":         "Bad",
						"installState": "installed",
						"runtimeState": "running",
						"health":       "unhealthy",
					},
				},
			})
		},
	})
	resp = onboardStatus(context.Background(), "req-status-ok", daemonOK, "telegram:1")
	if resp.Result != "ok" {
		t.Fatalf("expected status response, got %+v", resp)
	}
	for _, emoji := range []string{"🟢", "⚪", "🔴"} {
		if !strings.Contains(resp.Message, emoji) {
			t.Fatalf("expected status emoji %q in %q", emoji, resp.Message)
		}
	}
}
