package main

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func buildHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestParseCatalogAndWebhooksCommandArgs(t *testing.T) {
	add, err := parseCatalogCommandArgs([]string{"add", "--manifest", "/tmp/m.toml"})
	if err != nil {
		t.Fatalf("parseCatalogCommandArgs(add) error: %v", err)
	}
	if add.Action != "add" || add.ManifestPath != "/tmp/m.toml" {
		t.Fatalf("unexpected catalog add opts: %+v", add)
	}
	if _, err := parseCatalogCommandArgs([]string{"add"}); err == nil {
		t.Fatalf("expected missing manifest error")
	}
	if _, err := parseCatalogCommandArgs([]string{"remove"}); err == nil {
		t.Fatalf("expected missing remove id error")
	}
	if _, err := parseCatalogCommandArgs([]string{"unsupported"}); err == nil {
		t.Fatalf("expected unsupported catalog action error")
	}

	if _, err := parseWebhooksCommandArgs([]string{"test"}); err != nil {
		t.Fatalf("parseWebhooksCommandArgs(test) error: %v", err)
	}
	if _, err := parseWebhooksCommandArgs([]string{"noop"}); err == nil {
		t.Fatalf("expected invalid webhooks action error")
	}
}

func TestParseSinceAndLogTimestampHelpers(t *testing.T) {
	if _, ok := parseSinceValue(""); ok {
		t.Fatalf("expected empty since parse failure")
	}
	if ts, ok := parseSinceValue("1700000000"); !ok || ts.Unix() != 1700000000 {
		t.Fatalf("expected unix timestamp parse success, got ts=%v ok=%v", ts, ok)
	}
	if _, ok := parseSinceValue("not-a-time"); ok {
		t.Fatalf("expected invalid since parse failure")
	}

	if _, ok := extractLogTimestamp(""); ok {
		t.Fatalf("expected empty log timestamp parse failure")
	}
	if ts, ok := extractLogTimestamp("2026-01-01T00:00:00Z [INFO] ok"); !ok || ts.Year() != 2026 {
		t.Fatalf("expected RFC3339 log timestamp parse success, got ts=%v ok=%v", ts, ok)
	}
}

func TestRunServiceCatalogAndWebhooksCommands(t *testing.T) {
	t.Run("run service command", func(t *testing.T) {
		var out bytes.Buffer
		if err := runServiceCommand(&out, serviceCommandOptions{Action: "status"}); err != nil {
			t.Fatalf("runServiceCommand error: %v", err)
		}
		if !strings.Contains(out.String(), "Windows Service") {
			t.Fatalf("unexpected runServiceCommand output: %q", out.String())
		}
	})

	t.Run("run catalog add/list/remove", func(t *testing.T) {
		customDir := t.TempDir()
		t.Setenv("CARRIER_CUSTOM_CATALOG_DIR", customDir)

		manifestPath := filepath.Join(t.TempDir(), "manifest.toml")
		if err := os.WriteFile(manifestPath, []byte("id = \"custom-agent\"\n"), 0o600); err != nil {
			t.Fatalf("write manifest: %v", err)
		}

		var out bytes.Buffer
		if err := runCatalogCommand(&out, catalogCommandOptions{Action: "add", ManifestPath: manifestPath}); err != nil {
			t.Fatalf("runCatalogCommand(add) error: %v", err)
		}
		out.Reset()
		if err := runCatalogCommand(&out, catalogCommandOptions{Action: "list"}); err != nil {
			t.Fatalf("runCatalogCommand(list) error: %v", err)
		}
		if !strings.Contains(out.String(), "custom-agent") {
			t.Fatalf("expected custom-agent in catalog list output: %q", out.String())
		}
		out.Reset()
		if err := runCatalogCommand(&out, catalogCommandOptions{Action: "remove", ID: "custom-agent"}); err != nil {
			t.Fatalf("runCatalogCommand(remove) error: %v", err)
		}
		if err := runCatalogCommand(io.Discard, catalogCommandOptions{Action: "unsupported"}); err == nil {
			t.Fatalf("expected unsupported catalog action error")
		}
	})

	t.Run("run webhooks command", func(t *testing.T) {
		successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer successServer.Close()
		t.Setenv("CARRIER_WEBHOOK_URL", successServer.URL)

		var out bytes.Buffer
		if err := runWebhooksCommand(&out, webhooksCommandOptions{Action: "test"}); err != nil {
			t.Fatalf("runWebhooksCommand success error: %v", err)
		}
		if !strings.Contains(strings.ToLower(out.String()), "successfully") {
			t.Fatalf("unexpected webhook success output: %q", out.String())
		}

		failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer failServer.Close()
		t.Setenv("CARRIER_WEBHOOK_URL", failServer.URL)
		if err := runWebhooksCommand(io.Discard, webhooksCommandOptions{Action: "test"}); err == nil {
			t.Fatalf("expected webhook non-2xx error")
		}
	})
}

func TestRunConfigCommandsAndHelpers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, ".carrier", "config.v2.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{"config_version":2}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("CARRIER_CONFIG", configPath)

	backupPath := filepath.Join(t.TempDir(), "backup.json")
	if err := runConfigCommand(io.Discard, configCommandOptions{Action: "backup", OutputPath: backupPath}); err != nil {
		t.Fatalf("runConfigCommand(backup) error: %v", err)
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("expected backup file: %v", err)
	}

	restorePath := filepath.Join(t.TempDir(), "restore.json")
	if err := os.WriteFile(restorePath, []byte(`{"config_version":2}`), 0o600); err != nil {
		t.Fatalf("write restore file: %v", err)
	}
	if err := runConfigCommand(io.Discard, configCommandOptions{Action: "restore", FromPath: restorePath}); err != nil {
		t.Fatalf("runConfigCommand(restore) error: %v", err)
	}

	if err := runConfigCommand(io.Discard, configCommandOptions{
		Action: "set-tls",
		Value:  "true",
		Domain: "example.com",
	}); err != nil {
		t.Fatalf("runConfigCommand(set-tls) error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".carrier", "tls", "config.json")); err != nil {
		t.Fatalf("expected tls config file: %v", err)
	}

	if err := runConfigCommand(io.Discard, configCommandOptions{Action: "unsupported"}); err == nil {
		t.Fatalf("expected unsupported config action error")
	}

	if err := validateJSONBlob([]byte(""), "invalid"); err == nil {
		t.Fatalf("expected empty json blob error")
	}
	if err := validateConfigBackup([]byte(`{"config_version":1}`)); err == nil {
		t.Fatalf("expected config version mismatch error")
	}
}

func TestParseKeysAndRunLogsCommand(t *testing.T) {
	if _, err := parseKeysCommandArgs([]string{"generate", "--name", "mykey"}); err != nil {
		t.Fatalf("parseKeysCommandArgs(generate) error: %v", err)
	}
	if _, err := parseKeysCommandArgs([]string{"list"}); err != nil {
		t.Fatalf("parseKeysCommandArgs(list) error: %v", err)
	}
	if _, err := parseKeysCommandArgs([]string{"delete", "mykey"}); err != nil {
		t.Fatalf("parseKeysCommandArgs(delete) error: %v", err)
	}
	if _, err := parseKeysCommandArgs([]string{"unknown"}); err == nil {
		t.Fatalf("expected unknown keys action error")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	path, err := managedInstancesPath()
	if err != nil {
		t.Fatalf("managedInstancesPath error: %v", err)
	}
	if err := saveManagedInstances(path, []managedAgentInstance{
		{
			ID:        "inst-1",
			Name:      "inst-1",
			Type:      "openclaw",
			AgentID:   "agent-a",
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		},
	}); err != nil {
		t.Fatalf("saveManagedInstances error: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agents/agent-a/logs" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"lines":["2026-01-01T00:00:00Z [INFO] started","2026-01-01T00:10:00Z [WARN] retry"]}`))
	}))
	defer server.Close()
	u, _ := neturl.Parse(server.URL)
	host, port, splitErr := net.SplitHostPort(u.Host)
	if splitErr != nil {
		t.Fatalf("split host/port: %v", splitErr)
	}
	t.Setenv("CARRIER_SERVER_HOST", host)
	t.Setenv("CARRIER_SERVER_PORT", port)
	t.Setenv("CARRIER_SERVER_API_TOKEN", "daemon-token")

	var out bytes.Buffer
	if err := runLogsCommand(&out, logsCommandOptions{
		AgentID: "inst-1",
		Level:   "WARN",
	}); err != nil {
		t.Fatalf("runLogsCommand error: %v", err)
	}
	if !strings.Contains(out.String(), "[WARN]") || strings.Contains(out.String(), "[INFO]") {
		t.Fatalf("unexpected filtered log output: %q", out.String())
	}

	exportPath := filepath.Join(t.TempDir(), "logs.txt")
	if err := runLogsCommand(io.Discard, logsCommandOptions{
		AgentID: "inst-1",
		Export:  exportPath,
	}); err != nil {
		t.Fatalf("runLogsCommand export error: %v", err)
	}
	if _, err := os.Stat(exportPath); err != nil {
		t.Fatalf("expected exported log file: %v", err)
	}
}

func TestHealthBootstrapAndPairCodeHelpers(t *testing.T) {
	t.Setenv("CARRIER_GATEWAY_HOST", "")
	t.Setenv("CARRIER_GATEWAY_PORT", "")
	if !strings.Contains(gatewayProbeBaseURL(), "127.0.0.1:8787") {
		t.Fatalf("unexpected default gateway probe base url: %q", gatewayProbeBaseURL())
	}
	t.Setenv("CARRIER_SERVER_HOST", "")
	t.Setenv("CARRIER_SERVER_PORT", "")
	if !strings.Contains(daemonProbeBaseURL(), "127.0.0.1:9090") {
		t.Fatalf("unexpected default daemon probe base url: %q", daemonProbeBaseURL())
	}

	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "ok")
			return
		}
		if r.URL.Path == "/api/v1/pairing/codes" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"codes":[{"code":"PAIR-123","expiresAt":"2026-01-01T00:00:00Z"}]}`)
			return
		}
		if r.URL.Path == "/api/v1/pairing/codes" && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"code":"PAIR-456","expiresAt":"2026-01-01T00:00:00Z"}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer healthServer.Close()

	if !checkGatewayHealth(healthServer.URL) {
		t.Fatalf("expected healthy gateway probe")
	}
	if !checkDaemonHealth(healthServer.URL) {
		t.Fatalf("expected healthy daemon probe")
	}

	origDaemonProbe := daemonHealthProbe
	origGatewayProbe := gatewayHealthProbe
	t.Cleanup(func() {
		daemonHealthProbe = origDaemonProbe
		gatewayHealthProbe = origGatewayProbe
	})
	daemonHealthProbe = func(string) bool { return true }
	gatewayHealthProbe = func(string) bool { return true }
	if err := waitForDaemonHealthy("http://x", 10*time.Millisecond); err != nil {
		t.Fatalf("waitForDaemonHealthy success error: %v", err)
	}
	if err := waitForGatewayHealthy("http://x", 10*time.Millisecond); err != nil {
		t.Fatalf("waitForGatewayHealthy success error: %v", err)
	}
	daemonHealthProbe = func(string) bool { return false }
	if err := waitForDaemonHealthy("http://x", 1*time.Millisecond); err == nil {
		t.Fatalf("expected waitForDaemonHealthy timeout error")
	}

	code, expiresAt, err := fetchDaemonPairCode(healthServer.URL)
	if err != nil {
		t.Fatalf("fetchDaemonPairCode list-existing error: %v", err)
	}
	if code != "PAIR-123" || expiresAt == "" {
		t.Fatalf("unexpected pair code response: code=%q expiresAt=%q", code, expiresAt)
	}
	if _, _, err := fetchDaemonPairCode(""); err == nil {
		t.Fatalf("expected empty daemon base URL error")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CARRIER_BOOTSTRAP_VERBOSE", "true")
	if !bootstrapVerboseEnabled() {
		t.Fatalf("expected bootstrap verbose to be enabled")
	}
	t.Setenv("CARRIER_BOOTSTRAP_RUN_DIR", filepath.Join(home, "run"))
	t.Setenv("CARRIER_BOOTSTRAP_LOG_DIR", filepath.Join(home, "logs"))

	if _, err := bootstrapRunDir(); err != nil {
		t.Fatalf("bootstrapRunDir error: %v", err)
	}
	if _, err := bootstrapLogDir(); err != nil {
		t.Fatalf("bootstrapLogDir error: %v", err)
	}
	pidPath, err := bootstrapPIDPath("daemon")
	if err != nil {
		t.Fatalf("bootstrapPIDPath error: %v", err)
	}
	if mustBootstrapPIDPath("daemon") == "" {
		t.Fatalf("expected mustBootstrapPIDPath to return non-empty path")
	}
	if err := writeBootstrapPIDFile("daemon", 1234); err != nil {
		t.Fatalf("writeBootstrapPIDFile error: %v", err)
	}
	if raw, err := os.ReadFile(pidPath); err != nil || !strings.Contains(string(raw), "1234") {
		t.Fatalf("unexpected pid file content raw=%q err=%v", string(raw), err)
	}
	if _, _, err := openBootstrapLogFile("daemon"); err != nil {
		t.Fatalf("openBootstrapLogFile error: %v", err)
	}
}

func TestOpenAICodexDeviceCodeHelpers(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/api/accounts/deviceauth/usercode":
				return buildHTTPResponse(http.StatusOK, `{"device_auth_id":"dev-1","user_code":"CODE-1","interval":"0"}`), nil
			case "/api/accounts/deviceauth/token":
				return buildHTTPResponse(http.StatusOK, `{"authorization_code":"auth-1","code_verifier":"verifier-1"}`), nil
			case "/oauth/token":
				return buildHTTPResponse(http.StatusOK, `{"access_token":"token-1"}`), nil
			default:
				return buildHTTPResponse(http.StatusNotFound, `{"error":"not found"}`), nil
			}
		}),
	}

	deviceID, userCode, interval, err := requestOpenAICodexUserCode(client)
	if err != nil {
		t.Fatalf("requestOpenAICodexUserCode error: %v", err)
	}
	if deviceID != "dev-1" || userCode != "CODE-1" || interval != openAICodexDefaultPollSeconds {
		t.Fatalf("unexpected user code response: deviceID=%q userCode=%q interval=%d", deviceID, userCode, interval)
	}

	authCode, verifier, err := pollOpenAICodexAuthorization(client, deviceID, userCode, interval, 2*time.Second)
	if err != nil {
		t.Fatalf("pollOpenAICodexAuthorization error: %v", err)
	}
	if authCode != "auth-1" || verifier != "verifier-1" {
		t.Fatalf("unexpected authorization response: authCode=%q verifier=%q", authCode, verifier)
	}

	token, err := exchangeOpenAICodexToken(client, authCode, verifier)
	if err != nil {
		t.Fatalf("exchangeOpenAICodexToken error: %v", err)
	}
	if token != "token-1" {
		t.Fatalf("unexpected exchanged token: %q", token)
	}

	if _, _, err := postJSON(client, "://bad-url", map[string]string{"x": "y"}); err == nil {
		t.Fatalf("expected postJSON invalid URL build error")
	}

	timeoutClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/api/accounts/deviceauth/token" {
				return buildHTTPResponse(http.StatusForbidden, `{"error":"pending"}`), nil
			}
			return buildHTTPResponse(http.StatusOK, `{"ok":true}`), nil
		}),
	}
	if _, _, err := pollOpenAICodexAuthorization(timeoutClient, "dev-x", "code-x", 0, 10*time.Millisecond); err == nil {
		t.Fatalf("expected pollOpenAICodexAuthorization timeout error")
	}

	origTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/accounts/deviceauth/usercode":
			return buildHTTPResponse(http.StatusOK, `{"device_auth_id":"dev-2","user_code":"CODE-2","interval":"0"}`), nil
		case "/api/accounts/deviceauth/token":
			return buildHTTPResponse(http.StatusOK, `{"authorization_code":"auth-2","code_verifier":"verifier-2"}`), nil
		case "/oauth/token":
			return buildHTTPResponse(http.StatusOK, `{"access_token":"token-2"}`), nil
		default:
			return buildHTTPResponse(http.StatusNotFound, `{"error":"not found"}`), nil
		}
	})
	t.Cleanup(func() { http.DefaultTransport = origTransport })

	var out bytes.Buffer
	token2, err := performOpenAICodexDeviceCodeFlow(&out)
	if err != nil {
		t.Fatalf("performOpenAICodexDeviceCodeFlow error: %v", err)
	}
	if token2 != "token-2" {
		t.Fatalf("unexpected flow token=%q", token2)
	}
	if !strings.Contains(out.String(), "authorization completed") {
		t.Fatalf("expected completion message in output: %q", out.String())
	}
}

func TestMiscEnvAndPromptHelpers(t *testing.T) {
	if err := applyEnvVars(map[string]string{"TEST_CARRIER_COVERAGE_KEY": "v"}); err != nil {
		t.Fatalf("applyEnvVars error: %v", err)
	}
	if got := os.Getenv("TEST_CARRIER_COVERAGE_KEY"); got != "v" {
		t.Fatalf("unexpected env value=%q", got)
	}

	merged := mergeEnvVars(
		map[string]string{"A": "1", "B": "2"},
		map[string]string{"B": "3", "C": "4"},
	)
	if merged["A"] != "1" || merged["B"] != "3" || merged["C"] != "4" {
		t.Fatalf("unexpected merged env vars: %v", merged)
	}

	reader := bufio.NewReader(strings.NewReader("yes\n"))
	yes, err := promptYesNo(reader, io.Discard, "q", false)
	if err != nil || !yes {
		t.Fatalf("promptYesNo expected yes=true err=nil got yes=%v err=%v", yes, err)
	}
	reader = bufio.NewReader(strings.NewReader("value\n"))
	value, err := promptInput(reader, io.Discard, "label", true)
	if err != nil || value != "value" {
		t.Fatalf("promptInput unexpected value=%q err=%v", value, err)
	}
	reader = bufio.NewReader(strings.NewReader(""))
	if _, err := promptInput(reader, io.Discard, "label", true); err == nil {
		t.Fatalf("expected interrupted promptInput error")
	}
}
