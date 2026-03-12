package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseCarrierCommandRoutesAgent(t *testing.T) {
	cmd, args, err := parseCarrierCommand([]string{"carrier", "agent", "launcher", "picoclaw"})
	if err != nil {
		t.Fatalf("parseCarrierCommand(agent) error: %v", err)
	}
	if cmd != "agent" {
		t.Fatalf("command=%q want agent", cmd)
	}
	if len(args) != 2 || args[0] != "launcher" || args[1] != "picoclaw" {
		t.Fatalf("args=%v want [launcher picoclaw]", args)
	}
}

func TestParseAgentCommandArgs(t *testing.T) {
	runOpts, err := parseAgentCommandArgs([]string{"run", "picoclaw", "-m", "hello", "--provider", "openrouter", "--session-id", "sess-1", "--model-alias", "flash", "--model", "google/gemini-2.0-flash-001", "--json"})
	if err != nil {
		t.Fatalf("parseAgentCommandArgs(run) error: %v", err)
	}
	if runOpts.Action != "run" || runOpts.AgentID != "picoclaw" || runOpts.Message != "hello" || runOpts.Provider != "openrouter" || runOpts.SessionID != "sess-1" || runOpts.ModelAlias != "flash" || runOpts.Model != "google/gemini-2.0-flash-001" || !runOpts.JSON {
		t.Fatalf("unexpected run opts: %+v", runOpts)
	}

	shellOpts, err := parseAgentCommandArgs([]string{"shell", "picoclaw", "--provider", "openrouter", "--session-id", "sess-2"})
	if err != nil {
		t.Fatalf("parseAgentCommandArgs(shell) error: %v", err)
	}
	if shellOpts.Action != "shell" || shellOpts.AgentID != "picoclaw" || shellOpts.Provider != "openrouter" || shellOpts.SessionID != "sess-2" {
		t.Fatalf("unexpected shell opts: %+v", shellOpts)
	}

	launcherOpts, err := parseAgentCommandArgs([]string{"launcher", "picoclaw", "--json"})
	if err != nil {
		t.Fatalf("parseAgentCommandArgs(launcher) error: %v", err)
	}
	if launcherOpts.Action != "launcher" || launcherOpts.AgentID != "picoclaw" || !launcherOpts.JSON {
		t.Fatalf("unexpected launcher opts: %+v", launcherOpts)
	}

	heartbeatOpts, err := parseAgentCommandArgs([]string{"heartbeat", "picoclaw", "--json"})
	if err != nil {
		t.Fatalf("parseAgentCommandArgs(heartbeat) error: %v", err)
	}
	if heartbeatOpts.Action != "heartbeat" || heartbeatOpts.AgentID != "picoclaw" || !heartbeatOpts.JSON {
		t.Fatalf("unexpected heartbeat opts: %+v", heartbeatOpts)
	}

	cronScheduleOpts, err := parseAgentCommandArgs([]string{"cron", "schedule", "picoclaw", "-m", "check launcher", "--provider", "openrouter", "--session-id", "cron-sess", "--json"})
	if err != nil {
		t.Fatalf("parseAgentCommandArgs(cron schedule) error: %v", err)
	}
	if cronScheduleOpts.Action != "cron-schedule" || cronScheduleOpts.AgentID != "picoclaw" || cronScheduleOpts.Message != "check launcher" || cronScheduleOpts.Provider != "openrouter" || cronScheduleOpts.SessionID != "cron-sess" || !cronScheduleOpts.JSON {
		t.Fatalf("unexpected cron schedule opts: %+v", cronScheduleOpts)
	}

	cronListOpts, err := parseAgentCommandArgs([]string{"cron", "list", "picoclaw", "--json"})
	if err != nil {
		t.Fatalf("parseAgentCommandArgs(cron list) error: %v", err)
	}
	if cronListOpts.Action != "cron-list" || cronListOpts.AgentID != "picoclaw" || !cronListOpts.JSON {
		t.Fatalf("unexpected cron list opts: %+v", cronListOpts)
	}

	cronCancelOpts, err := parseAgentCommandArgs([]string{"cron", "cancel", "picoclaw", "cron-1", "--json"})
	if err != nil {
		t.Fatalf("parseAgentCommandArgs(cron cancel) error: %v", err)
	}
	if cronCancelOpts.Action != "cron-cancel" || cronCancelOpts.AgentID != "picoclaw" || cronCancelOpts.CronJobID != "cron-1" || !cronCancelOpts.JSON {
		t.Fatalf("unexpected cron cancel opts: %+v", cronCancelOpts)
	}

	skillSearchOpts, err := parseAgentCommandArgs([]string{"skills", "search", "picoclaw", "--query", "workspace", "--json"})
	if err != nil {
		t.Fatalf("parseAgentCommandArgs(skills search) error: %v", err)
	}
	if skillSearchOpts.Action != "skills-search" || skillSearchOpts.AgentID != "picoclaw" || skillSearchOpts.Query != "workspace" || !skillSearchOpts.JSON {
		t.Fatalf("unexpected skills search opts: %+v", skillSearchOpts)
	}

	skillInstallOpts, err := parseAgentCommandArgs([]string{"skills", "install", "picoclaw", "workspace-inspection", "--json"})
	if err != nil {
		t.Fatalf("parseAgentCommandArgs(skills install) error: %v", err)
	}
	if skillInstallOpts.Action != "skills-install" || skillInstallOpts.AgentID != "picoclaw" || skillInstallOpts.SkillName != "workspace-inspection" || !skillInstallOpts.JSON {
		t.Fatalf("unexpected skills install opts: %+v", skillInstallOpts)
	}

	skillUninstallOpts, err := parseAgentCommandArgs([]string{"skills", "uninstall", "picoclaw", "workspace-inspection", "--json"})
	if err != nil {
		t.Fatalf("parseAgentCommandArgs(skills uninstall) error: %v", err)
	}
	if skillUninstallOpts.Action != "skills-uninstall" || skillUninstallOpts.AgentID != "picoclaw" || skillUninstallOpts.SkillName != "workspace-inspection" || !skillUninstallOpts.JSON {
		t.Fatalf("unexpected skills uninstall opts: %+v", skillUninstallOpts)
	}

	skillUpdateOpts, err := parseAgentCommandArgs([]string{"skills", "update", "picoclaw", "workspace-inspection", "--version", "v2.0.0", "--json"})
	if err != nil {
		t.Fatalf("parseAgentCommandArgs(skills update) error: %v", err)
	}
	if skillUpdateOpts.Action != "skills-update" || skillUpdateOpts.AgentID != "picoclaw" || skillUpdateOpts.SkillName != "workspace-inspection" || skillUpdateOpts.Version != "v2.0.0" || !skillUpdateOpts.JSON {
		t.Fatalf("unexpected skills update opts: %+v", skillUpdateOpts)
	}

	modelsOpts, err := parseAgentCommandArgs([]string{"models", "picoclaw", "--json"})
	if err != nil {
		t.Fatalf("parseAgentCommandArgs(models) error: %v", err)
	}
	if modelsOpts.Action != "models" || modelsOpts.AgentID != "picoclaw" || !modelsOpts.JSON {
		t.Fatalf("unexpected models opts: %+v", modelsOpts)
	}

	modelsSyncOpts, err := parseAgentCommandArgs([]string{"models", "sync", "picoclaw", "--json"})
	if err != nil {
		t.Fatalf("parseAgentCommandArgs(models sync) error: %v", err)
	}
	if modelsSyncOpts.Action != "models-sync" || modelsSyncOpts.AgentID != "picoclaw" || !modelsSyncOpts.JSON {
		t.Fatalf("unexpected models sync opts: %+v", modelsSyncOpts)
	}

	modelsDefaultOpts, err := parseAgentCommandArgs([]string{"models", "default", "picoclaw", "openrouter-safe", "--json"})
	if err != nil {
		t.Fatalf("parseAgentCommandArgs(models default) error: %v", err)
	}
	if modelsDefaultOpts.Action != "models-default" || modelsDefaultOpts.AgentID != "picoclaw" || modelsDefaultOpts.ProfileName != "openrouter-safe" || !modelsDefaultOpts.JSON {
		t.Fatalf("unexpected models default opts: %+v", modelsDefaultOpts)
	}

	modelsUpdateProfileOpts, err := parseAgentCommandArgs([]string{
		"models", "update-profile", "picoclaw", "openrouter-safe",
		"--model-alias", "flash-safe-v2",
		"--model", "anthropic/claude-sonnet-4.6",
		"--provider", "anthropic",
		"--base-url", "https://api.anthropic.com/v1",
		"--auth-method", "api_key",
		"--timeout-ms", "60000",
		"--retry-budget", "4",
		"--fallback-strategy", "round_robin",
		"--json",
	})
	if err != nil {
		t.Fatalf("parseAgentCommandArgs(models update-profile) error: %v", err)
	}
	if modelsUpdateProfileOpts.Action != "models-update-profile" ||
		modelsUpdateProfileOpts.AgentID != "picoclaw" ||
		modelsUpdateProfileOpts.ProfileName != "openrouter-safe" ||
		modelsUpdateProfileOpts.ModelAlias != "flash-safe-v2" ||
		modelsUpdateProfileOpts.Model != "anthropic/claude-sonnet-4.6" ||
		modelsUpdateProfileOpts.Provider != "anthropic" ||
		modelsUpdateProfileOpts.BaseURL != "https://api.anthropic.com/v1" ||
		modelsUpdateProfileOpts.AuthMethod != "api_key" ||
		modelsUpdateProfileOpts.TimeoutMs != 60000 ||
		modelsUpdateProfileOpts.RetryBudget != 4 ||
		modelsUpdateProfileOpts.FallbackStrategy != "round_robin" ||
		!modelsUpdateProfileOpts.JSON {
		t.Fatalf("unexpected models update-profile opts: %+v", modelsUpdateProfileOpts)
	}
}

func TestRunAgentCommand(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/agents/picoclaw/chat":
			if r.Method != http.MethodPost {
				http.NotFound(w, r)
				return
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["message"] != "hello launcher" {
				t.Fatalf("message=%v want hello launcher", body["message"])
			}
			if body["modelAlias"] != "flash" {
				t.Fatalf("modelAlias=%v want flash", body["modelAlias"])
			}
			if body["model"] != "google/gemini-2.0-flash-001" {
				t.Fatalf("model=%v want google/gemini-2.0-flash-001", body["model"])
			}
			_, _ = w.Write([]byte(`{"agentId":"picoclaw","sessionId":"sess-run","message":"pong"}`))
		case "/api/v1/agents/picoclaw/launcher":
			if r.Method != http.MethodGet {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(`{"result":"ok","agentId":"picoclaw","status":{"runtimeState":"running","installState":"installed","health":"healthy"},"heartbeat":{"state":"fresh","ageSeconds":4,"lastActivityAt":"2026-03-12T00:00:00Z"},"memory":{"contractId":"memory-alpha","contractDigest":"digest-1","syncState":"synced"},"providerReadiness":{"provider":"openrouter","ready":true,"authMode":"api_key"},"modelSurface":{"defaultProfile":"openrouter-fast","profiles":[{"profileName":"openrouter-fast","modelAlias":"flash","modelId":"google/gemini-2.0-flash-001","providerId":"openrouter","protocolFamily":"openai-compatible","timeoutMs":45000,"retryBudget":2,"fallbackStrategy":"ordered","primary":true},{"profileName":"openrouter-safe","modelAlias":"flash-safe","modelId":"deepseek/deepseek-chat-v3-0324","providerId":"openrouter","protocolFamily":"openai-compatible","primary":false}]},"lastModelRun":{"requestedAlias":"flash-safe","requestedModel":"deepseek/deepseek-chat-v3-0324","resolvedModel":"deepseek/deepseek-chat-v3-0324","fallbackGroup":"openrouter:flash","overrideHit":true,"fallbackHit":true,"lastRunAt":"2026-03-12T00:05:00Z"},"session":{"instanceId":"picoclaw-main","runtimeMode":"managed_gateway","updatedAt":"2026-03-12T00:00:00Z"},"cron":{"count":1,"jobs":[{"id":"cron-1","agentId":"picoclaw","prompt":"check launcher","nextRunAt":"2026-03-12T01:00:00Z","lastResult":"succeeded","lastRunAt":"2026-03-12T00:30:00Z"}]}}`))
		case "/api/v1/agents/picoclaw/cron":
			switch r.Method {
			case http.MethodGet:
				_, _ = w.Write([]byte(`{"jobs":[{"id":"cron-1","agentId":"picoclaw","prompt":"check launcher","lastResult":"succeeded","nextRunAt":"2026-03-12T01:00:00Z"}]}`))
			case http.MethodPost:
				_, _ = w.Write([]byte(`{"id":"cron-2","agentId":"picoclaw","prompt":"check launcher","lastResult":"scheduled","nextRunAt":"2026-03-12T01:00:00Z"}`))
			default:
				http.NotFound(w, r)
			}
		case "/api/v1/agents/picoclaw/cron/cron-1/cancel":
			if r.Method != http.MethodPost {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(`{"id":"cron-1","agentId":"picoclaw","prompt":"check launcher","lastResult":"cancelled","cancelledAt":"2026-03-12T00:40:00Z"}`))
		case "/api/v1/agents/picoclaw/skills/search":
			_, _ = w.Write([]byte(`{"skills":[{"name":"workspace-inspection","summary":"Inspect workspace state.","source":"catalog","version":"v1.2.3"}]}`))
		case "/api/v1/agents/picoclaw/skills/install":
			if r.Method != http.MethodPost {
				http.NotFound(w, r)
				return
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["name"] != "workspace-inspection" {
				t.Fatalf("skill install name=%v want workspace-inspection", body["name"])
			}
			_, _ = w.Write([]byte(`{"name":"workspace-inspection","summary":"Inspect workspace state.","source":"catalog","version":"v1.2.3"}`))
		case "/api/v1/agents/picoclaw/skills/update":
			if r.Method != http.MethodPost {
				http.NotFound(w, r)
				return
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["name"] != "workspace-inspection" {
				t.Fatalf("skill update name=%v want workspace-inspection", body["name"])
			}
			if body["version"] != "v2.0.0" {
				t.Fatalf("skill update version=%v want v2.0.0", body["version"])
			}
			_, _ = w.Write([]byte(`{"name":"workspace-inspection","summary":"Inspect workspace state.","source":"catalog","version":"v1.2.3","targetVersion":"v2.0.0"}`))
		case "/api/v1/agents/picoclaw/skills/uninstall":
			if r.Method != http.MethodPost {
				http.NotFound(w, r)
				return
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["name"] != "workspace-inspection" {
				t.Fatalf("skill uninstall name=%v want workspace-inspection", body["name"])
			}
			_, _ = w.Write([]byte(`{"name":"workspace-inspection","summary":"Inspect workspace state.","source":"catalog","version":"v1.2.3"}`))
		case "/api/v1/agents/picoclaw/models":
			if r.Method != http.MethodGet {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(`{"agentId":"picoclaw","instanceId":"picoclaw-main","configPath":"/tmp/picoclaw/config.json","modelSurface":{"defaultProfile":"openrouter-fast","profiles":[{"profileName":"openrouter-fast","modelAlias":"flash","modelId":"google/gemini-2.0-flash-001","providerId":"openrouter","protocolFamily":"openai-compatible","primary":true},{"profileName":"openrouter-safe","modelAlias":"flash","modelId":"deepseek/deepseek-chat-v3-0324","providerId":"openrouter","protocolFamily":"openai-compatible","primary":false}]}}`))
		case "/api/v1/agents/picoclaw/models/sync":
			if r.Method != http.MethodPost {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(`{"agentId":"picoclaw","instanceId":"picoclaw-main","configPath":"/tmp/picoclaw/config.json","synced":true,"modelSurface":{"defaultProfile":"openrouter-fast","profiles":[{"profileName":"openrouter-fast","modelAlias":"flash","modelId":"google/gemini-2.0-flash-001","providerId":"openrouter","protocolFamily":"openai-compatible","primary":true}]}}`))
		case "/api/v1/agents/picoclaw/models/default":
			if r.Method != http.MethodPost {
				http.NotFound(w, r)
				return
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["profileName"] != "openrouter-safe" {
				t.Fatalf("profileName=%v want openrouter-safe", body["profileName"])
			}
			_, _ = w.Write([]byte(`{"agentId":"picoclaw","instanceId":"picoclaw-main","configPath":"/tmp/picoclaw/config.json","modelSurface":{"defaultProfile":"openrouter-safe","profiles":[{"profileName":"openrouter-fast","modelAlias":"flash","modelId":"google/gemini-2.0-flash-001","providerId":"openrouter","protocolFamily":"openai-compatible","primary":true},{"profileName":"openrouter-safe","modelAlias":"flash-safe","modelId":"deepseek/deepseek-chat-v3-0324","providerId":"openrouter","protocolFamily":"openai-compatible","primary":false}]}}`))
		case "/api/v1/agents/picoclaw/models/profile":
			if r.Method != http.MethodPost {
				http.NotFound(w, r)
				return
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["profileName"] != "openrouter-safe" {
				t.Fatalf("profileName=%v want openrouter-safe", body["profileName"])
			}
			if body["modelAlias"] != "flash-safe-v2" {
				t.Fatalf("modelAlias=%v want flash-safe-v2", body["modelAlias"])
			}
			if body["modelId"] != "anthropic/claude-sonnet-4.6" {
				t.Fatalf("modelId=%v want anthropic/claude-sonnet-4.6", body["modelId"])
			}
			if body["providerId"] != "anthropic" {
				t.Fatalf("providerId=%v want anthropic", body["providerId"])
			}
			if body["baseUrl"] != "https://api.anthropic.com/v1" {
				t.Fatalf("baseUrl=%v want https://api.anthropic.com/v1", body["baseUrl"])
			}
			if body["authMethod"] != "api_key" {
				t.Fatalf("authMethod=%v want api_key", body["authMethod"])
			}
			if body["timeoutMs"] != float64(60000) {
				t.Fatalf("timeoutMs=%v want 60000", body["timeoutMs"])
			}
			if body["retryBudget"] != float64(4) {
				t.Fatalf("retryBudget=%v want 4", body["retryBudget"])
			}
			if body["fallbackStrategy"] != "round_robin" {
				t.Fatalf("fallbackStrategy=%v want round_robin", body["fallbackStrategy"])
			}
			_, _ = w.Write([]byte(`{"agentId":"picoclaw","instanceId":"picoclaw-main","configPath":"/tmp/picoclaw/config.json","modelSurface":{"defaultProfile":"openrouter-safe","profiles":[{"profileName":"openrouter-fast","modelAlias":"flash","modelId":"google/gemini-2.0-flash-001","providerId":"openrouter","protocolFamily":"openai-compatible","primary":true},{"profileName":"openrouter-safe","modelAlias":"flash-safe-v2","modelId":"anthropic/claude-sonnet-4.6","providerId":"anthropic","protocolFamily":"anthropic","baseUrl":"https://api.anthropic.com/v1","authMethod":"api_key","timeoutMs":60000,"retryBudget":4,"fallbackStrategy":"round_robin","primary":false}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer gateway.Close()

	setProbeEnvFromURL(t, "CARRIER_GATEWAY_HOST", "CARRIER_GATEWAY_PORT", gateway.URL)

	var out bytes.Buffer
	if err := runAgentCommand(strings.NewReader(""), &out, agentCommandOptions{Action: "run", AgentID: "picoclaw", Message: "hello launcher", ModelAlias: "flash", Model: "google/gemini-2.0-flash-001"}); err != nil {
		t.Fatalf("runAgentCommand(run) error: %v", err)
	}
	if !strings.Contains(out.String(), "pong") || !strings.Contains(out.String(), "sess-run") {
		t.Fatalf("run output=%s", out.String())
	}

	out.Reset()
	if err := runAgentCommand(strings.NewReader(""), &out, agentCommandOptions{Action: "launcher", AgentID: "picoclaw"}); err != nil {
		t.Fatalf("runAgentCommand(launcher) error: %v", err)
	}
	if !strings.Contains(out.String(), "fresh") || !strings.Contains(out.String(), "memory-alpha") || !strings.Contains(out.String(), "openrouter") || !strings.Contains(out.String(), "default=flash -> google/gemini-2.0-flash-001") || !strings.Contains(out.String(), "timeout=45000ms") || !strings.Contains(out.String(), "retry=2") || !strings.Contains(out.String(), "fallback=ordered") || !strings.Contains(out.String(), "last-model requested=flash-safe") || !strings.Contains(out.String(), "resolved=deepseek/deepseek-chat-v3-0324") || !strings.Contains(out.String(), "override=true") || !strings.Contains(out.String(), "fallback-hit=true") || !strings.Contains(out.String(), "last=2026-03-12T00:05:00Z") {
		t.Fatalf("launcher output=%s", out.String())
	}

	out.Reset()
	if err := runAgentCommand(strings.NewReader(""), &out, agentCommandOptions{Action: "heartbeat", AgentID: "picoclaw"}); err != nil {
		t.Fatalf("runAgentCommand(heartbeat) error: %v", err)
	}
	if !strings.Contains(out.String(), "fresh") || !strings.Contains(out.String(), "4s") {
		t.Fatalf("heartbeat output=%s", out.String())
	}

	out.Reset()
	if err := runAgentCommand(strings.NewReader(""), &out, agentCommandOptions{Action: "cron-schedule", AgentID: "picoclaw", Message: "check launcher", Provider: "openrouter", SessionID: "cron-sess"}); err != nil {
		t.Fatalf("runAgentCommand(cron schedule) error: %v", err)
	}
	if !strings.Contains(out.String(), "cron-2") || !strings.Contains(out.String(), "scheduled") {
		t.Fatalf("cron schedule output=%s", out.String())
	}

	out.Reset()
	if err := runAgentCommand(strings.NewReader(""), &out, agentCommandOptions{Action: "cron-list", AgentID: "picoclaw"}); err != nil {
		t.Fatalf("runAgentCommand(cron list) error: %v", err)
	}
	if !strings.Contains(out.String(), "cron-1") || !strings.Contains(out.String(), "check launcher") {
		t.Fatalf("cron list output=%s", out.String())
	}

	out.Reset()
	if err := runAgentCommand(strings.NewReader(""), &out, agentCommandOptions{Action: "cron-cancel", AgentID: "picoclaw", CronJobID: "cron-1"}); err != nil {
		t.Fatalf("runAgentCommand(cron cancel) error: %v", err)
	}
	if !strings.Contains(out.String(), "cron-1") || !strings.Contains(out.String(), "cancelled") {
		t.Fatalf("cron cancel output=%s", out.String())
	}

	out.Reset()
	if err := runAgentCommand(strings.NewReader(""), &out, agentCommandOptions{Action: "skills-search", AgentID: "picoclaw", Query: "workspace"}); err != nil {
		t.Fatalf("runAgentCommand(skills search) error: %v", err)
	}
	if !strings.Contains(out.String(), "workspace-inspection") || !strings.Contains(out.String(), "catalog v1.2.3") {
		t.Fatalf("skills search output=%s", out.String())
	}

	out.Reset()
	if err := runAgentCommand(strings.NewReader(""), &out, agentCommandOptions{Action: "skills-install", AgentID: "picoclaw", SkillName: "workspace-inspection"}); err != nil {
		t.Fatalf("runAgentCommand(skills install) error: %v", err)
	}
	if !strings.Contains(out.String(), "workspace-inspection") || !strings.Contains(out.String(), "catalog v1.2.3") {
		t.Fatalf("skills install output=%s", out.String())
	}

	out.Reset()
	if err := runAgentCommand(strings.NewReader(""), &out, agentCommandOptions{Action: "skills-update", AgentID: "picoclaw", SkillName: "workspace-inspection", Version: "v2.0.0"}); err != nil {
		t.Fatalf("runAgentCommand(skills update) error: %v", err)
	}
	if !strings.Contains(out.String(), "updated workspace-inspection") || !strings.Contains(out.String(), "target=v2.0.0") {
		t.Fatalf("skills update output=%s", out.String())
	}

	out.Reset()
	if err := runAgentCommand(strings.NewReader(""), &out, agentCommandOptions{Action: "skills-uninstall", AgentID: "picoclaw", SkillName: "workspace-inspection"}); err != nil {
		t.Fatalf("runAgentCommand(skills uninstall) error: %v", err)
	}
	if !strings.Contains(out.String(), "removed workspace-inspection") || !strings.Contains(out.String(), "catalog v1.2.3") {
		t.Fatalf("skills uninstall output=%s", out.String())
	}

	out.Reset()
	if err := runAgentCommand(strings.NewReader(""), &out, agentCommandOptions{Action: "models", AgentID: "picoclaw"}); err != nil {
		t.Fatalf("runAgentCommand(models) error: %v", err)
	}
	if !strings.Contains(out.String(), "default=flash -> google/gemini-2.0-flash-001") || !strings.Contains(out.String(), "profile=flash model=deepseek/deepseek-chat-v3-0324") {
		t.Fatalf("models output=%s", out.String())
	}

	out.Reset()
	if err := runAgentCommand(strings.NewReader(""), &out, agentCommandOptions{Action: "models-sync", AgentID: "picoclaw"}); err != nil {
		t.Fatalf("runAgentCommand(models sync) error: %v", err)
	}
	if !strings.Contains(out.String(), "synced=true") || !strings.Contains(out.String(), "default=flash -> google/gemini-2.0-flash-001") {
		t.Fatalf("models sync output=%s", out.String())
	}

	out.Reset()
	if err := runAgentCommand(strings.NewReader(""), &out, agentCommandOptions{Action: "models-default", AgentID: "picoclaw", ProfileName: "openrouter-safe"}); err != nil {
		t.Fatalf("runAgentCommand(models default) error: %v", err)
	}
	if !strings.Contains(out.String(), "default=flash-safe -> deepseek/deepseek-chat-v3-0324") {
		t.Fatalf("models default output=%s", out.String())
	}

	out.Reset()
	if err := runAgentCommand(strings.NewReader(""), &out, agentCommandOptions{
		Action:           "models-update-profile",
		AgentID:          "picoclaw",
		ProfileName:      "openrouter-safe",
		ModelAlias:       "flash-safe-v2",
		Model:            "anthropic/claude-sonnet-4.6",
		Provider:         "anthropic",
		BaseURL:          "https://api.anthropic.com/v1",
		AuthMethod:       "api_key",
		TimeoutMs:        60000,
		RetryBudget:      4,
		FallbackStrategy: "round_robin",
	}); err != nil {
		t.Fatalf("runAgentCommand(models update-profile) error: %v", err)
	}
	if !strings.Contains(out.String(), "profile=flash-safe-v2 model=anthropic/claude-sonnet-4.6") || !strings.Contains(out.String(), "fallback=round_robin") {
		t.Fatalf("models update-profile output=%s", out.String())
	}
}

func TestRunAgentShellCommand(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/agents/picoclaw/chat":
			_, _ = io.WriteString(w, `{"agentId":"picoclaw","sessionId":"sess-shell","message":"ack"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer gateway.Close()

	setProbeEnvFromURL(t, "CARRIER_GATEWAY_HOST", "CARRIER_GATEWAY_PORT", gateway.URL)

	var out bytes.Buffer
	in := strings.NewReader("hello\n/exit\n")
	if err := runAgentCommand(in, &out, agentCommandOptions{Action: "shell", AgentID: "picoclaw"}); err != nil {
		t.Fatalf("runAgentCommand(shell) error: %v", err)
	}
	if !strings.Contains(out.String(), "Interactive shell") || !strings.Contains(out.String(), "ack") {
		t.Fatalf("shell output=%s", out.String())
	}
}
