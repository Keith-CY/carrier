package gateway

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseErrorErrorMethod(t *testing.T) {
	pe := &ParseError{Err: "boom"}
	if pe.Error() != "boom" {
		t.Fatalf("unexpected ParseError.Error(): %q", pe.Error())
	}
}

func TestGatewayEnvHelpers(t *testing.T) {
	t.Setenv("CARRIER_TEST_ENV_OR_DEFAULT", "")
	if got := envOrDefault("CARRIER_TEST_ENV_OR_DEFAULT", "fallback"); got != "fallback" {
		t.Fatalf("envOrDefault fallback failed: %q", got)
	}

	t.Setenv("CARRIER_TEST_PARSE_INT", "abc")
	if got := parseEnvInt("CARRIER_TEST_PARSE_INT", 7); got != 7 {
		t.Fatalf("parseEnvInt fallback failed: %d", got)
	}
}

func TestRunUsesInjectedStartFunction(t *testing.T) {
	orig := startGatewayFn
	t.Cleanup(func() { startGatewayFn = orig })
	startGatewayFn = func(_ *GatewayConfig) error {
		return errors.New("sentinel-run-error")
	}

	err := Run()
	if err == nil || !strings.Contains(err.Error(), "sentinel-run-error") {
		t.Fatalf("expected injected Run error, got %v", err)
	}
}

func TestStartGatewayRejectsNonLoopbackWithoutToken(t *testing.T) {
	cfg := &GatewayConfig{
		Hostname: "0.0.0.0",
		Port:     8787,
	}
	err := StartGateway(cfg)
	if err == nil || !strings.Contains(err.Error(), "CARRIER_GATEWAY_API_TOKEN must be set") {
		t.Fatalf("expected non-loopback auth error, got %v", err)
	}
}

func TestLoadGatewayConfigFromEnvRespectsOverrides(t *testing.T) {
	dataDir := t.TempDir()
	artifactRoot := filepath.Join(dataDir, "artifacts")

	t.Setenv("CARRIER_GATEWAY_PORT", "9988")
	t.Setenv("CARRIER_GATEWAY_HOST", "0.0.0.0")
	t.Setenv("CARRIER_GATEWAY_API_TOKEN", "gw-token")
	t.Setenv("CARRIER_DAEMON_BASE_URL", "http://127.0.0.1:9191/")
	t.Setenv("CARRIER_DAEMON_TIMEOUT_MS", "2500")
	t.Setenv("CARRIER_TELEGRAM_TRANSPORT_MODE", "PoLLing")
	t.Setenv("CARRIER_TELEGRAM_POLLING_TIMEOUT_SEC", "45")
	t.Setenv("CARRIER_TELEGRAM_API_BASE_URL", "https://tg.example")
	t.Setenv("CARRIER_MAX_COMMAND_BODY_BYTES", "12345")
	t.Setenv("CARRIER_WORKER_LEASE_STALE_AFTER_SEC", "90")
	t.Setenv("CARRIER_WORKER_HEARTBEAT_TIMEOUT_SEC", "15")
	t.Setenv("CARRIER_RATE_LIMIT_PER_SESSION", "13")
	t.Setenv("CARRIER_RATE_LIMIT_GLOBAL", "37")
	t.Setenv("CARRIER_RATE_LIMIT_WINDOW_MS", "2000")
	t.Setenv("CARRIER_INTEGRATION_CALLBACK_POLL_INTERVAL_SEC", "7")
	t.Setenv("SESSION_DATA_DIR", "")
	t.Setenv("ARTIFACT_ROOT", artifactRoot)

	cfg := LoadGatewayConfigFromEnv()
	if cfg.Port != 9988 || cfg.Hostname != "0.0.0.0" {
		t.Fatalf("unexpected network config: %+v", cfg)
	}
	if cfg.APIToken != "gw-token" {
		t.Fatalf("unexpected api token: %q", cfg.APIToken)
	}
	if cfg.DaemonBaseURL != "http://127.0.0.1:9191" {
		t.Fatalf("unexpected daemon base url: %q", cfg.DaemonBaseURL)
	}
	if cfg.DaemonTimeout != 2500*time.Millisecond {
		t.Fatalf("unexpected daemon timeout: %v", cfg.DaemonTimeout)
	}
	if cfg.TelegramTransportMode != "polling" || cfg.TelegramPollingTimeout != 45 || cfg.TelegramAPIBaseURL != "https://tg.example" {
		t.Fatalf("unexpected telegram config: mode=%q timeout=%d api=%q", cfg.TelegramTransportMode, cfg.TelegramPollingTimeout, cfg.TelegramAPIBaseURL)
	}
	if cfg.MaxCommandBodyBytes != 12345 || cfg.RateLimitPerSession != 13 || cfg.RateLimitGlobal != 37 || cfg.RateLimitWindow != 2*time.Second {
		t.Fatalf("unexpected limits: body=%d perSession=%d global=%d window=%v", cfg.MaxCommandBodyBytes, cfg.RateLimitPerSession, cfg.RateLimitGlobal, cfg.RateLimitWindow)
	}
	if cfg.IntegrationCallbackPollInterval != 7*time.Second {
		t.Fatalf("unexpected integration callback poll interval: %v", cfg.IntegrationCallbackPollInterval)
	}
	if cfg.WorkerLeaseStaleAfter != 90*time.Second || cfg.WorkerHeartbeatTimeout != 15*time.Second {
		t.Fatalf("unexpected worker stale thresholds: stale=%v heartbeat=%v", cfg.WorkerLeaseStaleAfter, cfg.WorkerHeartbeatTimeout)
	}
	if cfg.DataDir != artifactRoot || cfg.ArtifactRoot != artifactRoot {
		t.Fatalf("unexpected data/artifact roots: data=%q artifact=%q", cfg.DataDir, cfg.ArtifactRoot)
	}
}

func TestLoadGatewayConfigFromEnvDerivesArtifactRootFromSessionDataDir(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("SESSION_DATA_DIR", dataDir)
	t.Setenv("ARTIFACT_ROOT", "")

	cfg := LoadGatewayConfigFromEnv()
	want := filepath.Join(dataDir, "artifacts")
	if cfg.DataDir != dataDir || cfg.ArtifactRoot != want {
		t.Fatalf("unexpected data/artifact roots: data=%q artifact=%q wantArtifact=%q", cfg.DataDir, cfg.ArtifactRoot, want)
	}
}

func TestNormalizeGatewayConfigFeatureFlags(t *testing.T) {
	cfg := &GatewayConfig{
		RemoteControlPlaneEnabled: false,
		RemoteChatEnabled:         true,
		ProviderBindingEnabled:    true,
	}
	normalizeGatewayConfigFeatureFlags(cfg)
	if cfg.RemoteChatEnabled || cfg.ProviderBindingEnabled {
		t.Fatalf("expected dependent flags disabled when remote control plane is off: %+v", cfg)
	}
}

func TestLoadGatewayConfigFromEnvNormalizesFeatureDependencies(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_CONTROL_PLANE_ENABLED", "false")
	t.Setenv("CARRIER_REMOTE_CHAT_ENABLED", "true")
	t.Setenv("CARRIER_PROVIDER_BINDING_ENABLED", "true")

	cfg := LoadGatewayConfigFromEnv()
	if cfg.RemoteControlPlaneEnabled {
		t.Fatalf("expected remote control plane disabled, got %+v", cfg)
	}
	if cfg.RemoteChatEnabled || cfg.ProviderBindingEnabled {
		t.Fatalf("expected dependent feature flags normalized to false, got %+v", cfg)
	}
}

func TestStartGatewayReturnsListenErrorForInvalidAddress(t *testing.T) {
	tmp := t.TempDir()
	artifactRoot := filepath.Join(tmp, "artifacts")
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		t.Fatalf("mkdir artifact root: %v", err)
	}

	cfg := &GatewayConfig{
		Hostname:               "bad host",
		Port:                   8787,
		APIToken:               "token",
		DaemonBaseURL:          "http://127.0.0.1:9090",
		DaemonTimeout:          time.Second,
		TelegramAPIBaseURL:     "https://api.telegram.org",
		TelegramPollingTimeout: 30,
		DataDir:                tmp,
		ArtifactRoot:           artifactRoot,
		RateLimitPerSession:    10,
		RateLimitGlobal:        20,
		RateLimitWindow:        time.Second,
	}

	err := StartGateway(cfg)
	if err == nil || !strings.Contains(err.Error(), "gateway listen") {
		t.Fatalf("expected listen error, got %v", err)
	}
}

func TestDownloadStorePeriodicCleanupAndStop(t *testing.T) {
	store := NewDownloadStore(t.TempDir(), nil)
	store.StartPeriodicCleanup()
	store.Stop()
}

func TestValidateArtifactRoot(t *testing.T) {
	if _, err := ValidateArtifactRoot("/"); err == nil {
		t.Fatal("expected root path to be rejected")
	}
	if _, err := ValidateArtifactRoot("/etc/test"); err == nil {
		t.Fatal("expected blocked system path to be rejected")
	}

	okPath, err := os.MkdirTemp(".", "artifact-root-*")
	if err != nil {
		t.Fatalf("create local temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(okPath) })

	okPath = filepath.Join(okPath, "artifacts")
	resolved, err := ValidateArtifactRoot(okPath)
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if resolved == "" {
		t.Fatal("expected resolved artifact root")
	}
}

func TestWriteInternalGatewayError(t *testing.T) {
	w := httptest.NewRecorder()
	writeInternalGatewayError(w, http.StatusBadGateway, "E_GATEWAY", "gateway failed", "test-context", errors.New("sensitive OPENAI_API_KEY=abc"))
	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected status 502, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "gateway failed") {
		t.Fatalf("unexpected response body: %s", w.Body.String())
	}
}

func TestSetWebUIHandlerFactory(t *testing.T) {
	SetWebUIHandlerFactory(nil)
	got := webUIHandler()
	if got == nil {
		t.Fatal("expected non-nil default webui handler")
	}

	SetWebUIHandlerFactory(func() http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
	})

	rec := httptest.NewRecorder()
	got.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	// got currently references default handler; fetch latest and verify custom factory.
	rec = httptest.NewRecorder()
	webUIHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected custom webui handler status 204, got %d", rec.Code)
	}
}

func TestCleanupManagedInstanceFiles(t *testing.T) {
	tmp := t.TempDir()
	record := filepath.Join(tmp, "record.json")
	config := filepath.Join(tmp, "config.json")
	workspace := filepath.Join(tmp, "workspace")

	if err := os.WriteFile(record, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "tmp.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := cleanupManagedInstanceFiles(managedAgentInstance{
		RecordPath: record,
		ConfigPath: config,
		Workspace:  workspace,
		UpdatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("cleanupManagedInstanceFiles returned error: %v", err)
	}
}
