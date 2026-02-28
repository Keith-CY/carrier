package main

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"path/filepath"
	"strings"
	"testing"

	"carrier/configv2"
	"carrier/shared/openclawcfg"
)

func TestParseCarrierCommandRoutesRemote(t *testing.T) {
	cmd, args, err := parseCarrierCommand([]string{"carrier", "remote", "add", "openclaw"})
	if err != nil {
		t.Fatalf("parseCarrierCommand(remote) error: %v", err)
	}
	if cmd != "remote" {
		t.Fatalf("command = %q, want remote", cmd)
	}
	if len(args) != 2 || args[0] != "add" || args[1] != "openclaw" {
		t.Fatalf("args = %v, want [add openclaw]", args)
	}
}

func TestParseRemoteCommandArgsAddDefaultsAndValidation(t *testing.T) {
	opts, err := parseRemoteCommandArgs([]string{
		"add", "openclaw",
		"--host-id", "host-1",
		"--host", "127.0.0.1",
		"--port", "2222",
		"--user", "carrier",
		"--key-path", "/tmp/id_ed25519",
		"--sync-channel", "telegram",
		"--sync-provider", "openai-codex",
		"--check-retries", "3",
		"--check-retry-delay", "1",
		"--skip-reconnect-check",
	})
	if err != nil {
		t.Fatalf("parseRemoteCommandArgs(add) error: %v", err)
	}
	if opts.Action != "add" {
		t.Fatalf("action = %q, want add", opts.Action)
	}
	if opts.AgentID != "openclaw" {
		t.Fatalf("agent_id = %q, want openclaw", opts.AgentID)
	}
	if opts.InstallAgentID != "main" {
		t.Fatalf("install_agent_id = %q, want main", opts.InstallAgentID)
	}
	if opts.HostID != "host-1" || opts.HostAddr != "127.0.0.1" || opts.Port != 2222 || opts.User != "carrier" {
		t.Fatalf("parsed host options mismatch: %+v", opts)
	}
	if len(opts.SyncChannels) != 1 || opts.SyncChannels[0] != "telegram" {
		t.Fatalf("sync channels = %v, want [telegram]", opts.SyncChannels)
	}
	if len(opts.SyncProviders) != 1 || opts.SyncProviders[0] != "openai-codex" {
		t.Fatalf("sync providers = %v, want [openai-codex]", opts.SyncProviders)
	}
	if !opts.SkipReconnectCheck {
		t.Fatalf("skip reconnect should be true")
	}

	picoOpts, err := parseRemoteCommandArgs([]string{
		"add", "picoclaw",
		"--host-id", "h",
		"--host", "127.0.0.1",
		"--user", "carrier",
		"--key-path", "/tmp/id",
	})
	if err != nil {
		t.Fatalf("parseRemoteCommandArgs(picoclaw) error: %v", err)
	}
	if picoOpts.InstallAgentID != "picoclaw" {
		t.Fatalf("install_agent_id = %q, want picoclaw", picoOpts.InstallAgentID)
	}

	if _, err := parseRemoteCommandArgs([]string{"sync", "host-1", "main"}); err == nil {
		t.Fatal("expected unsupported action validation error")
	}
	if _, err := parseRemoteCommandArgs([]string{
		"add", "openclaw",
		"--host-id", "host-1",
		"--host", "127.0.0.1",
		"--user", "carrier",
		"--key-path", "/tmp/id",
		"--sync-channel", "unsupported",
	}); err == nil {
		t.Fatal("expected invalid sync-channel validation error")
	}
}

func TestRunRemoteAddCommandWorkflowWithoutSync(t *testing.T) {
	var (
		upsertCalls  int
		checkCalls   int
		installCalls int
		listCalls    int
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/remote/hosts":
			if r.Method != http.MethodPost {
				http.NotFound(w, r)
				return
			}
			upsertCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":"ok"}`))
		case "/api/v1/remote/hosts/host-1/check":
			if r.Method != http.MethodPost {
				http.NotFound(w, r)
				return
			}
			checkCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":"ok","check":{"sshOk":true,"openclawFound":true},"instances":[{"id":"host-1:main","agentId":"main"}],"pendingPullInstances":[],"pullConfirmationRequired":false}`))
		case "/api/v1/remote/hosts/host-1/instances/main/install/stream":
			if r.Method != http.MethodPost {
				http.NotFound(w, r)
				return
			}
			installCalls++
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"type\":\"start\"}\n\n"))
			_, _ = w.Write([]byte("data: {\"type\":\"log\",\"stream\":\"stdout\",\"line\":\"installing\"}\n\n"))
			_, _ = w.Write([]byte("data: {\"type\":\"result\",\"install\":{\"installed\":true}}\n\n"))
			_, _ = w.Write([]byte("data: {\"type\":\"finish\",\"finishReason\":\"stop\"}\n\n"))
		case "/api/v1/remote/hosts/host-1/instances":
			if r.Method != http.MethodGet {
				http.NotFound(w, r)
				return
			}
			listCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":"ok","instances":[{"id":"host-1:main","agentId":"main"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	configureGatewayProbeEnvForTest(t, server.URL)

	var out bytes.Buffer
	err := runRemoteAddCommand(strings.NewReader(""), &out, remoteCommandOptions{
		Action:             "add",
		AgentID:            "openclaw",
		InstallAgentID:     "main",
		HostID:             "host-1",
		HostName:           "host-1",
		HostAddr:           "127.0.0.1",
		Port:               2222,
		User:               "carrier",
		KeyPath:            "/tmp/id_ed25519",
		RuntimeMode:        "on_demand",
		CheckRetries:       1,
		CheckRetryDelaySec: 0,
		SkipReconnectCheck: true,
	})
	if err != nil {
		t.Fatalf("runRemoteAddCommand(add) error: %v", err)
	}
	if upsertCalls != 1 || installCalls != 1 || checkCalls != 2 || listCalls != 1 {
		t.Fatalf("unexpected call counts: upsert=%d check=%d install=%d list=%d", upsertCalls, checkCalls, installCalls, listCalls)
	}
	if !strings.Contains(out.String(), "Completed: OpenClaw remote install finished for host host-1.") {
		t.Fatalf("output missing success message: %s", out.String())
	}
}

func TestBuildJSONRemoteConfigPatch(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
	if _, err := saveProviderCredential("openai", "sk-openai-1"); err != nil {
		t.Fatalf("saveProviderCredential(openai): %v", err)
	}
	cfg := &configv2.Config{
		Channels: []configv2.Channel{
			{
				ID:       "telegram",
				BotToken: "tg-1",
				AllowFrom: []string{
					"1001",
				},
				Enabled: true,
			},
		},
		DefaultModel: "gpt-5.3-codex",
		ModelList: []configv2.Model{
			{
				ModelName:     "gpt-5.3-codex",
				Model:         "openai/gpt-5.3-codex",
				ProviderID:    "openai-codex",
				CredentialRef: "openai-codex",
			},
			{
				ModelName:     "gpt-4.1",
				Model:         "openai/gpt-4.1",
				ProviderID:    "openai",
				CredentialRef: "openai",
			},
		},
	}

	patch, err := buildJSONRemoteConfigPatch(remoteCommandOptions{
		SyncChannels:      []string{"telegram"},
		SyncProviders:     []string{"openai"},
		TelegramAllowFrom: []string{"2002"},
	}, cfg)
	if err != nil {
		t.Fatalf("buildJSONRemoteConfigPatch error: %v", err)
	}
	channels, _ := patch["channels"].(map[string]interface{})
	telegram, _ := channels["telegram"].(map[string]interface{})
	if strings.TrimSpace(anyToString(telegram["token"])) != "tg-1" {
		t.Fatalf("telegram token mismatch: %+v", telegram)
	}
	defaults := patch["agents"].(map[string]interface{})["defaults"].(map[string]interface{})
	if strings.TrimSpace(anyToString(defaults["provider"])) != "openai" {
		t.Fatalf("defaults provider mismatch: %+v", defaults)
	}
	providers := patch["providers"].(map[string]interface{})
	openai := providers["openai"].(map[string]interface{})
	if strings.TrimSpace(anyToString(openai["api_key"])) != "sk-openai-1" {
		t.Fatalf("provider api key mismatch: %+v", openai)
	}
}

func TestBuildOpenClawRemoteConfigPatch(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
	if _, err := saveProviderCredential("openai", "sk-openai-1"); err != nil {
		t.Fatalf("saveProviderCredential(openai): %v", err)
	}
	cfg := &configv2.Config{
		Channels: []configv2.Channel{
			{
				ID:       "telegram",
				BotToken: "tg-1",
				AllowFrom: []string{
					"1001",
				},
				Enabled: true,
			},
		},
		DefaultModel: "gpt-5.3-codex",
		ModelList: []configv2.Model{
			{
				ModelName:     "gpt-4.1",
				Model:         "openai/gpt-4.1",
				ProviderID:    "openai",
				CredentialRef: "openai",
			},
		},
	}

	patch, err := buildOpenClawRemoteConfigPatch(remoteCommandOptions{
		SyncChannels:      []string{"telegram"},
		SyncProviders:     []string{"openai"},
		TelegramAllowFrom: []string{"2002"},
	}, cfg)
	if err != nil {
		t.Fatalf("buildOpenClawRemoteConfigPatch error: %v", err)
	}
	channels, _ := patch["channels"].(map[string]interface{})
	telegram, _ := channels["telegram"].(map[string]interface{})
	if strings.TrimSpace(anyToString(telegram["botToken"])) != "tg-1" {
		t.Fatalf("telegram botToken mismatch: %+v", telegram)
	}
	rawAllowFrom, _ := telegram["allowFrom"].([]string)
	if len(rawAllowFrom) != 1 || strings.TrimSpace(rawAllowFrom[0]) != "2002" {
		t.Fatalf("telegram allowFrom mismatch: %+v", telegram["allowFrom"])
	}
	models, _ := patch["models"].(map[string]interface{})
	providers, _ := models["providers"].(map[string]interface{})
	openai, _ := providers["openai"].(map[string]interface{})
	apiKeyRef, _ := openai["apiKey"].(map[string]interface{})
	if strings.TrimSpace(anyToString(apiKeyRef["provider"])) != "carrier_file" {
		t.Fatalf("expected api key ref provider carrier_file, got %+v", apiKeyRef)
	}
	secretsPatch, _ := patch[openclawcfg.CarrierSecretFilePatchKey].(map[string]interface{})
	secretProviders, _ := secretsPatch["providers"].(map[string]interface{})
	secretOpenAI, _ := secretProviders["openai"].(map[string]interface{})
	if strings.TrimSpace(anyToString(secretOpenAI["apiKey"])) != "sk-openai-1" {
		t.Fatalf("expected secret payload api key sk-openai-1, got %+v", secretOpenAI)
	}
}

func TestBuildZeroClawRemoteConfigPatchRejectsProviderOnlySync(t *testing.T) {
	cfg := &configv2.Config{
		Channels: []configv2.Channel{
			{
				ID:       "telegram",
				BotToken: "tg-1",
				Enabled:  true,
			},
		},
		ModelList: []configv2.Model{
			{
				ModelName:     "gpt-4.1",
				Model:         "openai/gpt-4.1",
				ProviderID:    "openai",
				CredentialRef: "openai",
			},
		},
	}

	_, err := buildZeroClawRemoteConfigPatch(remoteCommandOptions{
		SyncProviders: []string{"openai"},
	}, cfg)
	if err == nil {
		t.Fatal("expected provider-only zeroclaw sync to fail")
	}
	if !strings.Contains(err.Error(), "requires both --sync-channel and --sync-provider") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildZeroClawRemoteConfigPatchRejectsChannelOnlySync(t *testing.T) {
	cfg := &configv2.Config{
		Channels: []configv2.Channel{
			{
				ID:       "telegram",
				BotToken: "tg-1",
				Enabled:  true,
			},
		},
	}

	_, err := buildZeroClawRemoteConfigPatch(remoteCommandOptions{
		SyncChannels: []string{"telegram"},
	}, cfg)
	if err == nil {
		t.Fatal("expected channel-only zeroclaw sync to fail")
	}
	if !strings.Contains(err.Error(), "requires both --sync-channel and --sync-provider") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func configureGatewayProbeEnvForTest(t *testing.T, serverURL string) {
	t.Helper()
	parsed, err := neturl.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	host, port, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatalf("split host/port: %v", err)
	}
	t.Setenv("CARRIER_GATEWAY_HOST", host)
	t.Setenv("CARRIER_GATEWAY_PORT", port)
}
