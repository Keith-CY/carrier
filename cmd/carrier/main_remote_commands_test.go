package main

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

func TestParseCarrierCommandRoutesConfigCommands(t *testing.T) {
	cmd, args, err := parseCarrierCommand([]string{"carrier", "config", "backup"})
	if err != nil {
		t.Fatalf("parseCarrierCommand(config) error: %v", err)
	}
	if cmd != "config" || len(args) != 1 || args[0] != "backup" {
		t.Fatalf("unexpected parsed command=%q args=%v", cmd, args)
	}

	cmd, args, err = parseCarrierCommand([]string{"carrier", "remote-store", "restore", "--from", "/tmp/r.json"})
	if err != nil {
		t.Fatalf("parseCarrierCommand(remote-store) error: %v", err)
	}
	if cmd != "remote-store" || len(args) != 3 || args[0] != "restore" {
		t.Fatalf("unexpected parsed command=%q args=%v", cmd, args)
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
		"--isolation",
		"--skip-reconnect-check",
	})
	if err != nil {
		t.Fatalf("parseRemoteCommandArgs(add) error: %v", err)
	}
	if opts.Action != "add" {
		t.Fatalf("action = %q, want add", opts.Action)
	}
	if opts.AuthMode != "private_key" {
		t.Fatalf("auth mode = %q, want private_key", opts.AuthMode)
	}
	if !opts.AutoRollback {
		t.Fatalf("auto rollback = %v, want true", opts.AutoRollback)
	}
	if !opts.Isolation {
		t.Fatalf("isolation = %v, want true", opts.Isolation)
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
	if !picoOpts.Isolation {
		t.Fatalf("expected picoclaw remote add to default isolation=true, got %+v", picoOpts)
	}
	if _, err := parseRemoteCommandArgs([]string{
		"add", "openclaw",
		"--host-id", "h2",
		"--host", "127.0.0.1",
		"--user", "carrier",
	}); err == nil {
		t.Fatal("expected missing key auth validation error")
	} else if !strings.Contains(err.Error(), "--key-path or --key-ref is required") {
		t.Fatalf("unexpected no-key validation error: %v", err)
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

	sshConfigOpts, err := parseRemoteCommandArgs([]string{
		"add", "openclaw",
		"--host-id", "h2",
		"--auth-mode", "ssh_config",
		"--ssh-config-host", "prod-host",
		"--host", "prod-host",
	})
	if err != nil {
		t.Fatalf("parseRemoteCommandArgs(ssh_config) error: %v", err)
	}
	if sshConfigOpts.AuthMode != "ssh_config" {
		t.Fatalf("auth_mode = %q, want ssh_config", sshConfigOpts.AuthMode)
	}
	if !sshConfigOpts.Isolation {
		t.Fatalf("expected openclaw remote add to default isolation=true, got %+v", sshConfigOpts)
	}

	noIsolationOpts, err := parseRemoteCommandArgs([]string{
		"add", "openclaw",
		"--host-id", "h3",
		"--host", "127.0.0.1",
		"--user", "carrier",
		"--key-path", "/tmp/id",
		"--no-isolation",
	})
	if err != nil {
		t.Fatalf("parseRemoteCommandArgs(--no-isolation) error: %v", err)
	}
	if noIsolationOpts.Isolation {
		t.Fatalf("expected isolation=false with --no-isolation, got %+v", noIsolationOpts)
	}

	logsOpts, err := parseRemoteCommandArgs([]string{"logs", "host-1", "main", "--tail", "50"})
	if err != nil {
		t.Fatalf("parseRemoteCommandArgs(logs) error: %v", err)
	}
	if logsOpts.Action != "logs" || logsOpts.HostID != "host-1" || logsOpts.TargetAgentID != "main" || logsOpts.Tail != 50 {
		t.Fatalf("unexpected logs opts: %+v", logsOpts)
	}

	rollbackOpts, err := parseRemoteCommandArgs([]string{"rollback", "host-1", "main", "--commit", "abc123"})
	if err != nil {
		t.Fatalf("parseRemoteCommandArgs(rollback) error: %v", err)
	}
	if rollbackOpts.Commit != "abc123" {
		t.Fatalf("rollback commit = %q, want abc123", rollbackOpts.Commit)
	}

	keyImportOpts, err := parseRemoteCommandArgs([]string{"key", "import", "--file", "/tmp/id_ed25519"})
	if err != nil {
		t.Fatalf("parseRemoteCommandArgs(key import) error: %v", err)
	}
	if keyImportOpts.Action != "key-import" || keyImportOpts.KeyImportPath != "/tmp/id_ed25519" {
		t.Fatalf("unexpected key import opts: %+v", keyImportOpts)
	}

	keyGenerateOpts, err := parseRemoteCommandArgs([]string{"key", "generate", "--type", "rsa", "--output", "/tmp/id_rsa"})
	if err != nil {
		t.Fatalf("parseRemoteCommandArgs(key generate) error: %v", err)
	}
	if keyGenerateOpts.Action != "key-generate" || keyGenerateOpts.KeyType != "rsa" || keyGenerateOpts.KeyOutputPath != "/tmp/id_rsa" {
		t.Fatalf("unexpected key generate opts: %+v", keyGenerateOpts)
	}
}

func TestRunRemoteAddCommandWorkflowWithoutSync(t *testing.T) {
	var (
		upsertCalls  int
		checkCalls   int
		installCalls int
		listCalls    int
		installBody  string
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
			bodyRaw, _ := io.ReadAll(r.Body)
			installBody = strings.TrimSpace(string(bodyRaw))
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
		Isolation:          true,
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
	if !strings.Contains(installBody, `"isolation":true`) {
		t.Fatalf("expected install stream request to include isolation=true, got %s", installBody)
	}
	if !strings.Contains(out.String(), "Completed: OpenClaw remote install finished for host host-1.") {
		t.Fatalf("output missing success message: %s", out.String())
	}
}

func TestRunRemoteAddCommandAutomaticRollbackOnPostCheckFailure(t *testing.T) {
	var (
		checkCalls    int
		rollbackCalls int
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/remote/hosts":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":"ok"}`))
		case "/api/v1/remote/hosts/host-1/check":
			checkCalls++
			if checkCalls == 1 {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"result":"ok","check":{"sshOk":true,"openclawFound":true},"instances":[{"id":"host-1:main","agentId":"main"}],"pendingPullInstances":[],"pullConfirmationRequired":false}`))
				return
			}
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":{"code":"E_REMOTE_CHECK_FAILED","message":"post check failed"}}`))
		case "/api/v1/remote/hosts/host-1/instances/main/install/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"type\":\"start\"}\n\n"))
			_, _ = w.Write([]byte("data: {\"type\":\"result\",\"install\":{\"installed\":true}}\n\n"))
			_, _ = w.Write([]byte("data: {\"type\":\"finish\",\"finishReason\":\"stop\"}\n\n"))
		case "/api/v1/remote/hosts/host-1/instances/main/rollback":
			rollbackCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":"ok","rollback":{"rolledBack":true,"fromCommit":"a","newCommit":"b"}}`))
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
		TargetAgentID:      "main",
		HostID:             "host-1",
		HostName:           "host-1",
		HostAddr:           "127.0.0.1",
		Port:               2222,
		User:               "carrier",
		KeyPath:            "/tmp/id_ed25519",
		AuthMode:           "private_key",
		RuntimeMode:        "on_demand",
		CheckRetries:       0,
		CheckRetryDelaySec: 0,
		SkipReconnectCheck: true,
		AutoRollback:       true,
	})
	if err == nil {
		t.Fatal("expected post-check failure with rollback completion")
	}
	if !strings.Contains(err.Error(), "automatic rollback succeeded") {
		t.Fatalf("expected rollback success annotation, got %v", err)
	}
	if rollbackCalls != 1 {
		t.Fatalf("expected one rollback call, got %d", rollbackCalls)
	}
}

func TestRunRemoteKeyImportCommand(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(keyPath, []byte("-----BEGIN TEST KEY-----\nabc\n-----END TEST KEY-----\n"), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	var (
		uploadedKey string
		handlerErr  error
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/remote/keys":
			if err := r.ParseMultipartForm(2 << 20); err != nil {
				handlerErr = err
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			file, _, err := r.FormFile("file")
			if err != nil {
				handlerErr = err
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			defer file.Close()
			raw, err := io.ReadAll(file)
			if err != nil {
				handlerErr = err
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			uploadedKey = string(raw)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":"ok","key":{"keyRef":"k-123","fingerprint":"SHA256:test","sizeBytes":64}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	configureGatewayProbeEnvForTest(t, server.URL)

	var out bytes.Buffer
	if err := runRemoteKeyImportCommand(&out, remoteCommandOptions{
		Action:        "key-import",
		KeyImportPath: keyPath,
	}); err != nil {
		t.Fatalf("runRemoteKeyImportCommand() error: %v", err)
	}
	if handlerErr != nil {
		t.Fatalf("handler error: %v", handlerErr)
	}
	if !strings.Contains(uploadedKey, "BEGIN TEST KEY") {
		t.Fatalf("expected uploaded key content, got %q", uploadedKey)
	}
	if !strings.Contains(out.String(), "keyRef=k-123") {
		t.Fatalf("expected output to include keyRef, got %s", out.String())
	}
}

func TestParseConfigAndRemoteStoreCommandArgs(t *testing.T) {
	cfgBackup, err := parseConfigCommandArgs([]string{"backup", "--output", "/tmp/config-backup.json"})
	if err != nil {
		t.Fatalf("parseConfigCommandArgs(backup) error: %v", err)
	}
	if cfgBackup.Action != "backup" || cfgBackup.OutputPath != "/tmp/config-backup.json" {
		t.Fatalf("unexpected config backup opts: %+v", cfgBackup)
	}
	if _, err := parseConfigCommandArgs([]string{"restore"}); err == nil {
		t.Fatal("expected missing --from error")
	}

	storeRestore, err := parseRemoteStoreCommandArgs([]string{"restore", "--from", "/tmp/remote-store.json"})
	if err != nil {
		t.Fatalf("parseRemoteStoreCommandArgs(restore) error: %v", err)
	}
	if storeRestore.Action != "restore" || storeRestore.FromPath != "/tmp/remote-store.json" {
		t.Fatalf("unexpected remote-store opts: %+v", storeRestore)
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

func TestRemoteBatchInstallParallel(t *testing.T) {
	var (
		currentConcurrent int32
		maxConcurrent     int32
		mu                sync.Mutex
		seenHosts         = map[string]bool{}
	)
	origHealthProbe := gatewayHealthProbe
	origHostsLister := remoteHostsLister
	origInstallStreamer := remoteInstallStreamer
	t.Cleanup(func() {
		gatewayHealthProbe = origHealthProbe
		remoteHostsLister = origHostsLister
		remoteInstallStreamer = origInstallStreamer
	})
	gatewayHealthProbe = func(string) bool { return true }
	remoteHostsLister = func() ([]remoteHostSummary, error) {
		return []remoteHostSummary{{ID: "h1"}, {ID: "h2"}, {ID: "h3"}}, nil
	}
	remoteInstallStreamer = func(_ io.Writer, hostID, _ string, _ bool) error {
		mu.Lock()
		seenHosts[hostID] = true
		mu.Unlock()
		n := atomic.AddInt32(&currentConcurrent, 1)
		for {
			prev := atomic.LoadInt32(&maxConcurrent)
			if n <= prev || atomic.CompareAndSwapInt32(&maxConcurrent, prev, n) {
				break
			}
		}
		time.Sleep(40 * time.Millisecond)
		atomic.AddInt32(&currentConcurrent, -1)
		return nil
	}

	var out bytes.Buffer
	err := runRemoteBatchInstall(&out, remoteCommandOptions{
		Action:         "install",
		AgentID:        "openclaw",
		InstallAgentID: "main",
		All:            true,
		Concurrency:    2,
	})
	if err != nil {
		t.Fatalf("runRemoteBatchInstall error: %v", err)
	}
	if maxConcurrent > 2 {
		t.Fatalf("max concurrent installs = %d, want <= 2", maxConcurrent)
	}
	if maxConcurrent < 2 {
		t.Fatalf("expected parallel installs, max concurrent=%d", maxConcurrent)
	}
	if len(seenHosts) != 3 {
		t.Fatalf("seenHosts=%v, want 3 hosts", seenHosts)
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
