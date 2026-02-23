package main

import (
	"bytes"
	"encoding/json"
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
	stdout, stderr, err := runCarrierBinary(t, bin, "tg-test-token\nopenai\nsk-test-openai\n", "onboard")
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

	var cfgPayload struct {
		Providers map[string]struct {
			APIKey string `json:"api_key"`
		} `json:"providers"`
		Channels map[string]struct {
			Token     string   `json:"token"`
			AllowFrom []string `json:"allow_from"`
		} `json:"channels"`
	}
	rawConfig, err := os.ReadFile(updated.ConfigPath)
	if err != nil {
		t.Fatalf("read openclaw config: %v", err)
	}
	if err := json.Unmarshal(rawConfig, &cfgPayload); err != nil {
		t.Fatalf("parse openclaw config: %v", err)
	}
	if got := cfgPayload.Providers["openai"].APIKey; got != openaiToken {
		t.Fatalf("providers.openai.api_key = %q, want %q", got, openaiToken)
	}
	if got := cfgPayload.Channels["telegram"].Token; got != openclawTelegramToken {
		t.Fatalf("channels.telegram.token = %q, want %q", got, openclawTelegramToken)
	}
	if cfgPayload.Channels["telegram"].Token == carrierTelegramToken {
		t.Fatalf("channels.telegram.token should not reuse carrier token %q", carrierTelegramToken)
	}
	allowFrom := cfgPayload.Channels["telegram"].AllowFrom
	if len(allowFrom) != 1 || allowFrom[0] != pairedChatID {
		t.Fatalf("channels.telegram.allow_from = %v, want [%s]", allowFrom, pairedChatID)
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
	if !strings.Contains(stdout, "Reusing saved OpenAI credential") {
		t.Fatalf("stdout missing provider credential reuse message: %q", stdout)
	}
	if !strings.Contains(stdout, "Token reuse is disabled for OpenClaw") {
		t.Fatalf("stdout missing token reuse policy message: %q", stdout)
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
	cmd.Dir = "."
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build carrier binary: %v\n%s", err, string(raw))
	}
	return path
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
