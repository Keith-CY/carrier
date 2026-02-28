package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"carrier/configv2"
)

func TestE2ECarrierBinaryOnboardOpenAIAPIKey(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}

	t.Setenv("HOME", home)
	t.Setenv("CARRIER_CONFIG", filepath.Join(tmp, "config.v2.json"))
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	t.Setenv("CARRIER_TELEGRAM_BOT_TOKEN", "")
	t.Setenv("CARRIER_DISCORD_BOT_TOKEN", "")
	t.Setenv("CARRIER_DISCORD_PUBLIC_KEY", "")

	pairCode := "pair-0123456789abcdef0123456789abcdef"
	pairExpiresAt := time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339Nano)
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/api/v1/pairing/codes" && r.Method == http.MethodGet:
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"codes": []map[string]string{
					{
						"code":      pairCode,
						"expiresAt": pairExpiresAt,
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer daemon.Close()

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer gateway.Close()

	setProbeEnvFromURL(t, "CARRIER_SERVER_HOST", "CARRIER_SERVER_PORT", daemon.URL)
	setProbeEnvFromURL(t, "CARRIER_GATEWAY_HOST", "CARRIER_GATEWAY_PORT", gateway.URL)

	bin := buildCarrierBinary(t)
	stdout, stderr, err := runCarrierBinary(t, bin, "telegram\ntg-test-token\nopenai\nsk-test-openai\n", "onboard")
	if err != nil {
		t.Fatalf("carrier onboard failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	cfg, _, err := configv2.Load()
	if err != nil {
		t.Fatalf("configv2.Load: %v", err)
	}
	if cfg.DefaultModel != "openai-default" {
		t.Fatalf("default model = %q, want %q", cfg.DefaultModel, "openai-default")
	}
	if len(cfg.ModelList) != 1 {
		t.Fatalf("model list size = %d, want 1", len(cfg.ModelList))
	}
	if got := cfg.ModelList[0].ProviderID; got != "openai" {
		t.Fatalf("provider_id = %q, want %q", got, "openai")
	}
	if got := cfg.ModelList[0].Model; got != "openai/gpt-5.2" {
		t.Fatalf("model = %q, want %q", got, "openai/gpt-5.2")
	}

	credential, _, ok, err := loadProviderCredential("openai")
	if err != nil {
		t.Fatalf("loadProviderCredential(openai): %v", err)
	}
	if !ok {
		t.Fatal("expected saved openai credential")
	}
	if credential != "sk-test-openai" {
		t.Fatalf("saved credential = %q, want %q", credential, "sk-test-openai")
	}

	if !strings.Contains(stdout, "Carrier TUI Onboard") {
		t.Fatalf("stdout missing onboard header: %q", stdout)
	}
	if !strings.Contains(stdout, "Provider override selected: OpenAI (openai)") {
		t.Fatalf("stdout missing provider override confirmation: %q", stdout)
	}
	if !strings.Contains(stdout, "PAIR_CODE: "+pairCode) {
		t.Fatalf("stdout missing pair code: %q", stdout)
	}
}

func TestE2ECarrierBinaryAddOpenClawReusesPairedUserAndProviderCredential(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}

	t.Setenv("HOME", home)
	t.Setenv("CARRIER_CONFIG", filepath.Join(tmp, "config.v2.json"))
	t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	const carrierTelegramToken = "tg-existing-token"
	t.Setenv("CARRIER_TELEGRAM_BOT_TOKEN", carrierTelegramToken)

	const pairedChatID = "123456789"
	const openaiToken = "sk-openai-reused"
	const openclawTelegramToken = "tg-openclaw-dedicated"

	if _, err := saveProviderCredential("openai", openaiToken); err != nil {
		t.Fatalf("saveProviderCredential(openai): %v", err)
	}

	instancesPath, err := managedInstancesPath()
	if err != nil {
		t.Fatalf("managedInstancesPath: %v", err)
	}
	if err := saveManagedInstances(instancesPath, []managedAgentInstance{
		{
			ID:           "openclaw-seed",
			Name:         "openclaw",
			Type:         "openclaw",
			AgentID:      "openclaw",
			GatewayURL:   "http://127.0.0.1:8787",
			Channel:      "telegram",
			Provider:     "openai",
			PairedChatID: pairedChatID,
			PairRequired: false,
			RuntimeState: "running",
			CreatedAt:    time.Date(2026, 2, 21, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
			UpdatedAt:    time.Date(2026, 2, 22, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		},
	}); err != nil {
		t.Fatalf("saveManagedInstances: %v", err)
	}

	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/api/v1/agents/openclaw/logs" && r.Method == http.MethodGet:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"lines":["[openclaw] preparing","[openclaw] running"]}`))
		case r.URL.Path == "/api/v1/agents/openclaw/install" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		case r.URL.Path == "/api/v1/agents/openclaw/start" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer daemon.Close()
	setProbeEnvFromURL(t, "CARRIER_SERVER_HOST", "CARRIER_SERVER_PORT", daemon.URL)

	bin := buildCarrierBinary(t)
	stdout, stderr, err := runCarrierBinary(t, bin, openclawTelegramToken+"\n", "add", "openclaw")
	if err != nil {
		t.Fatalf("carrier add openclaw failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	instances, _, err := loadManagedInstances()
	if err != nil {
		t.Fatalf("loadManagedInstances: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("instances size = %d, want 1", len(instances))
	}
	updated := instances[0]
	if updated.ID != "openclaw-seed" {
		t.Fatalf("instance id = %q, want %q", updated.ID, "openclaw-seed")
	}
	if updated.PairedChatID != pairedChatID {
		t.Fatalf("paired chat id = %q, want %q", updated.PairedChatID, pairedChatID)
	}
	if updated.PairRequired {
		t.Fatal("pair_required should be false when paired chat id is reused")
	}
	if updated.Provider != "openai" {
		t.Fatalf("provider = %q, want %q", updated.Provider, "openai")
	}
	if updated.Channel != "telegram" {
		t.Fatalf("channel = %q, want %q", updated.Channel, "telegram")
	}
	if strings.TrimSpace(updated.ConfigPath) == "" {
		t.Fatal("config path should not be empty")
	}
	if strings.TrimSpace(updated.RecordPath) == "" {
		t.Fatal("record path should not be empty")
	}

	rawConfig, err := os.ReadFile(updated.ConfigPath)
	if err != nil {
		t.Fatalf("read openclaw config: %v", err)
	}
	var cfgPayload map[string]interface{}
	if err := json.Unmarshal(rawConfig, &cfgPayload); err != nil {
		t.Fatalf("parse openclaw config: %v", err)
	}
	models, _ := cfgPayload["models"].(map[string]interface{})
	providers, _ := models["providers"].(map[string]interface{})
	openaiProvider, _ := providers["openai"].(map[string]interface{})
	apiKeyRef, _ := openaiProvider["apiKey"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(apiKeyRef["provider"])); got != "carrier_file" {
		t.Fatalf("models.providers.openai.apiKey.provider = %q, want carrier_file", got)
	}
	channels, _ := cfgPayload["channels"].(map[string]interface{})
	telegram, _ := channels["telegram"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(telegram["botToken"])); got != openclawTelegramToken {
		t.Fatalf("channels.telegram.botToken = %q, want %q", got, openclawTelegramToken)
	}
	if strings.TrimSpace(anyToString(telegram["botToken"])) == carrierTelegramToken {
		t.Fatalf("channels.telegram.botToken should not reuse carrier token %q", carrierTelegramToken)
	}
	rawAllowFrom, _ := telegram["allowFrom"].([]interface{})
	allowFrom := make([]string, 0, len(rawAllowFrom))
	for _, item := range rawAllowFrom {
		allowFrom = append(allowFrom, strings.TrimSpace(anyToString(item)))
	}
	if len(allowFrom) != 1 || allowFrom[0] != pairedChatID {
		t.Fatalf("channels.telegram.allowFrom = %v, want [%s]", allowFrom, pairedChatID)
	}

	secretsPath := filepath.Join(home, ".openclaw", "carrier-secrets.json")
	secretsRaw, err := os.ReadFile(secretsPath)
	if err != nil {
		t.Fatalf("read openclaw carrier secrets: %v", err)
	}
	var secretsPayload map[string]interface{}
	if err := json.Unmarshal(secretsRaw, &secretsPayload); err != nil {
		t.Fatalf("parse openclaw carrier secrets: %v", err)
	}
	secretProviders, _ := secretsPayload["providers"].(map[string]interface{})
	secretOpenAI, _ := secretProviders["openai"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(secretOpenAI["apiKey"])); got != openaiToken {
		t.Fatalf("carrier secrets providers.openai.apiKey = %q, want %q", got, openaiToken)
	}

	var recordPayload struct {
		PairedChatID string `json:"paired_chat_id"`
		Provider     string `json:"provider"`
		Channel      string `json:"channel"`
	}
	rawRecord, err := os.ReadFile(updated.RecordPath)
	if err != nil {
		t.Fatalf("read carrier record: %v", err)
	}
	if err := json.Unmarshal(rawRecord, &recordPayload); err != nil {
		t.Fatalf("parse carrier record: %v", err)
	}
	if recordPayload.PairedChatID != pairedChatID {
		t.Fatalf("record paired_chat_id = %q, want %q", recordPayload.PairedChatID, pairedChatID)
	}
	if recordPayload.Provider != "openai" {
		t.Fatalf("record provider = %q, want %q", recordPayload.Provider, "openai")
	}
	if recordPayload.Channel != "telegram" {
		t.Fatalf("record channel = %q, want %q", recordPayload.Channel, "telegram")
	}

	if !strings.Contains(stdout, "Reused paired Telegram user id from latest managed instance: "+pairedChatID) {
		t.Fatalf("stdout missing paired chat reuse message: %q", stdout)
	}
	if !strings.Contains(stdout, "Reuse saved OpenAI credential") && !strings.Contains(stdout, "Reusing saved OpenAI credential") {
		t.Fatalf("stdout missing provider credential reuse prompt/message: %q", stdout)
	}
	if !strings.Contains(stdout, "Token reuse is disabled for OpenClaw") {
		t.Fatalf("stdout missing token reuse policy message: %q", stdout)
	}
}

func TestE2ECarrierBinaryAddOpenClawIsolationSendsInstallAndStartIsolationPayload(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}

	t.Setenv("HOME", home)
	t.Setenv("CARRIER_CONFIG", filepath.Join(tmp, "config.v2.json"))
	t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	t.Setenv("CARRIER_TELEGRAM_BOT_TOKEN", "")

	if _, err := saveProviderCredential("openai", "sk-openai-reused"); err != nil {
		t.Fatalf("saveProviderCredential(openai): %v", err)
	}

	var installBody string
	var startBody string
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/api/v1/agents/openclaw/logs" && r.Method == http.MethodGet:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"lines":[]}`))
		case r.URL.Path == "/api/v1/agents/openclaw/install" && r.Method == http.MethodPost:
			raw, _ := io.ReadAll(r.Body)
			installBody = strings.TrimSpace(string(raw))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		case r.URL.Path == "/api/v1/agents/openclaw/start" && r.Method == http.MethodPost:
			raw, _ := io.ReadAll(r.Body)
			startBody = strings.TrimSpace(string(raw))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer daemon.Close()
	setProbeEnvFromURL(t, "CARRIER_SERVER_HOST", "CARRIER_SERVER_PORT", daemon.URL)

	bin := buildCarrierBinary(t)
	stdout, stderr, err := runCarrierBinary(t, bin, "tg-openclaw-dedicated\n", "add", "openclaw", "--isolation")
	if err != nil {
		t.Fatalf("carrier add openclaw --isolation failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	if !strings.Contains(installBody, `"isolation":true`) {
		t.Fatalf("expected install payload to include isolation=true, got %q", installBody)
	}
	if !strings.Contains(startBody, `"isolation":true`) {
		t.Fatalf("expected start payload to include isolation=true, got %q", startBody)
	}
}

func TestE2ECarrierBinaryAddManagedAgentsIsolationSendsInstallAndStartIsolationPayload(t *testing.T) {
	agentIDs := []string{"picoclaw", "zeroclaw"}
	for _, agentID := range agentIDs {
		t.Run(agentID, func(t *testing.T) {
			tmp := t.TempDir()
			home := filepath.Join(tmp, "home")
			if err := os.MkdirAll(home, 0o700); err != nil {
				t.Fatalf("mkdir home: %v", err)
			}

			t.Setenv("HOME", home)
			t.Setenv("CARRIER_CONFIG", filepath.Join(tmp, "config.v2.json"))
			t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(tmp, "instances.json"))
			t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
			t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
			t.Setenv("CARRIER_TELEGRAM_BOT_TOKEN", "")

			if _, err := saveProviderCredential("openai", "sk-openai-reused"); err != nil {
				t.Fatalf("saveProviderCredential(openai): %v", err)
			}
			if _, err := saveProviderCredential("openai-codex", "codex-token-reused"); err != nil {
				t.Fatalf("saveProviderCredential(openai-codex): %v", err)
			}

			var installBody string
			var startBody string
			logsPath := fmt.Sprintf("/api/v1/agents/%s/logs", agentID)
			installPath := fmt.Sprintf("/api/v1/agents/%s/install", agentID)
			startPath := fmt.Sprintf("/api/v1/agents/%s/start", agentID)
			daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/healthz":
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"status":"ok"}`))
				case r.URL.Path == logsPath && r.Method == http.MethodGet:
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"lines":[]}`))
				case r.URL.Path == installPath && r.Method == http.MethodPost:
					raw, _ := io.ReadAll(r.Body)
					installBody = strings.TrimSpace(string(raw))
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"ok":true}`))
				case r.URL.Path == startPath && r.Method == http.MethodPost:
					raw, _ := io.ReadAll(r.Body)
					startBody = strings.TrimSpace(string(raw))
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"ok":true}`))
				default:
					http.NotFound(w, r)
				}
			}))
			defer daemon.Close()
			setProbeEnvFromURL(t, "CARRIER_SERVER_HOST", "CARRIER_SERVER_PORT", daemon.URL)

			bin := buildCarrierBinary(t)
			stdout, stderr, err := runCarrierBinary(t, bin, "tg-"+agentID+"-dedicated\n", "add", agentID, "--isolation")
			if err != nil {
				t.Fatalf("carrier add %s --isolation failed: %v\nstdout:\n%s\nstderr:\n%s", agentID, err, stdout, stderr)
			}

			if !strings.Contains(installBody, `"isolation":true`) {
				t.Fatalf("expected install payload to include isolation=true, got %q", installBody)
			}
			if !strings.Contains(startBody, `"isolation":true`) {
				t.Fatalf("expected start payload to include isolation=true, got %q", startBody)
			}
		})
	}
}

func buildCarrierBinary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "carrier-e2e")
	if runtime.GOOS == "windows" {
		path += ".exe"
	}

	cmd := exec.Command("go", "build", "-o", path, ".")
	cmd.Env = os.Environ()
	cacheRoot := filepath.Join(os.TempDir(), "carrier-test-gocache")
	modCacheRoot := filepath.Join(os.TempDir(), "carrier-test-gomodcache")
	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		t.Fatalf("mkdir gocache root: %v", err)
	}
	if err := os.MkdirAll(modCacheRoot, 0o755); err != nil {
		t.Fatalf("mkdir gomodcache root: %v", err)
	}
	cmd.Env = setEnvEntry(cmd.Env, "GOCACHE", cacheRoot)
	cmd.Env = setEnvEntry(cmd.Env, "GOMODCACHE", modCacheRoot)
	cmd.Dir = "."
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build carrier binary: %v\n%s", err, string(raw))
	}
	return path
}

func setEnvEntry(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			continue
		}
		out = append(out, entry)
	}
	return append(out, prefix+value)
}

func runCarrierBinary(t *testing.T, binaryPath, stdin string, args ...string) (string, string, error) {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	cmd.Env = os.Environ()
	cmd.Stdin = strings.NewReader(stdin)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func setProbeEnvFromURL(t *testing.T, hostKey, portKey, rawURL string) {
	t.Helper()
	parsed, err := neturl.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		t.Fatalf("parse url %q: %v", rawURL, err)
	}
	host, port, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatalf("split host/port %q: %v", parsed.Host, err)
	}
	host = strings.TrimSpace(host)
	if host == "" {
		host = "127.0.0.1"
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	t.Setenv(hostKey, host)
	t.Setenv(portKey, port)
}
