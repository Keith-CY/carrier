package gateway

import (
	"testing"
)

func TestLoadGatewayConfigFromEnv_Defaults(t *testing.T) {
	// Clear relevant env vars
	for _, key := range []string{
		"CARRIER_GATEWAY_PORT", "CARRIER_GATEWAY_HOST", "CARRIER_GATEWAY_API_TOKEN",
		"CARRIER_DAEMON_BASE_URL", "CARRIER_SERVER_API_TOKEN", "CARRIER_DAEMON_TIMEOUT_MS",
		"CARRIER_DISCORD_PUBLIC_KEY", "CARRIER_FEISHU_VERIFICATION_TOKEN",
		"CARRIER_TELEGRAM_WEBHOOK_SECRET", "CARRIER_MAX_COMMAND_BODY_BYTES",
		"CARRIER_RATE_LIMIT_PER_SESSION", "CARRIER_RATE_LIMIT_GLOBAL",
		"CARRIER_RATE_LIMIT_WINDOW_MS", "SESSION_DATA_DIR", "ARTIFACT_ROOT",
	} {
		t.Setenv(key, "")
	}

	cfg := LoadGatewayConfigFromEnv()
	if cfg.Port != 8787 {
		t.Errorf("expected port 8787, got %d", cfg.Port)
	}
	if cfg.Hostname != "127.0.0.1" {
		t.Errorf("expected hostname 127.0.0.1, got %q", cfg.Hostname)
	}
	if cfg.DaemonBaseURL != "http://127.0.0.1:9090" {
		t.Errorf("expected default daemon URL, got %q", cfg.DaemonBaseURL)
	}
	if cfg.MaxCommandBodyBytes != defaultMaxCommandBodyBytes {
		t.Errorf("expected default max body bytes, got %d", cfg.MaxCommandBodyBytes)
	}
}

func TestLoadGatewayConfigFromEnv_Custom(t *testing.T) {
	t.Setenv("CARRIER_GATEWAY_PORT", "9999")
	t.Setenv("CARRIER_GATEWAY_HOST", "0.0.0.0")
	t.Setenv("CARRIER_GATEWAY_API_TOKEN", "mytoken")
	t.Setenv("CARRIER_DAEMON_BASE_URL", "http://localhost:1234/")
	t.Setenv("SESSION_DATA_DIR", "/tmp/test-sessions")

	cfg := LoadGatewayConfigFromEnv()
	if cfg.Port != 9999 {
		t.Errorf("expected port 9999, got %d", cfg.Port)
	}
	if cfg.Hostname != "0.0.0.0" {
		t.Errorf("expected hostname 0.0.0.0, got %q", cfg.Hostname)
	}
	if cfg.APIToken != "mytoken" {
		t.Errorf("expected token mytoken, got %q", cfg.APIToken)
	}
	// Trailing slash should be trimmed
	if cfg.DaemonBaseURL != "http://localhost:1234" {
		t.Errorf("expected trimmed daemon URL, got %q", cfg.DaemonBaseURL)
	}
	if cfg.DataDir != "/tmp/test-sessions" {
		t.Errorf("expected /tmp/test-sessions, got %q", cfg.DataDir)
	}
}

func TestEnvOrDefault(t *testing.T) {
	t.Setenv("TEST_ENV_KEY", "")
	if got := envOrDefault("TEST_ENV_KEY", "fallback"); got != "fallback" {
		t.Errorf("expected fallback, got %q", got)
	}
	t.Setenv("TEST_ENV_KEY", "value")
	if got := envOrDefault("TEST_ENV_KEY", "fallback"); got != "value" {
		t.Errorf("expected value, got %q", got)
	}
}

func TestParseEnvInt(t *testing.T) {
	t.Setenv("TEST_INT_KEY", "")
	if got := parseEnvInt("TEST_INT_KEY", 42); got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
	t.Setenv("TEST_INT_KEY", "100")
	if got := parseEnvInt("TEST_INT_KEY", 42); got != 100 {
		t.Errorf("expected 100, got %d", got)
	}
	t.Setenv("TEST_INT_KEY", "abc")
	if got := parseEnvInt("TEST_INT_KEY", 42); got != 42 {
		t.Errorf("expected 42 for invalid, got %d", got)
	}
	t.Setenv("TEST_INT_KEY", "-1")
	if got := parseEnvInt("TEST_INT_KEY", 42); got != 42 {
		t.Errorf("expected 42 for negative, got %d", got)
	}
}

func TestParseError_Error(t *testing.T) {
	pe := &ParseError{RequestID: "r1", Err: "test error"}
	if pe.Error() != "test error" {
		t.Errorf("unexpected error string: %q", pe.Error())
	}
}
