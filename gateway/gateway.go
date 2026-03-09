package gateway

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var startGatewayFn = StartGateway

// GatewayConfig holds all configuration for the gateway server.
type GatewayConfig struct {
	// Network
	Port     int
	Hostname string

	// Auth
	APIToken   string                   // CARRIER_GATEWAY_API_TOKEN
	RoleTokens map[string]GatewayRole   // CARRIER_GATEWAY_ROLE_TOKENS (role:token,role:token)

	// Daemon connection
	DaemonBaseURL string // CARRIER_DAEMON_BASE_URL (default http://127.0.0.1:9090)
	DaemonToken   string // CARRIER_SERVER_API_TOKEN
	DaemonTimeout time.Duration

	// Provider verification
	DiscordPublicKey        string // CARRIER_DISCORD_PUBLIC_KEY
	FeishuVerificationToken string // CARRIER_FEISHU_VERIFICATION_TOKEN
	TelegramWebhookSecret   string // CARRIER_TELEGRAM_WEBHOOK_SECRET
	TelegramBotToken        string // CARRIER_TELEGRAM_BOT_TOKEN
	TelegramTransportMode   string // CARRIER_TELEGRAM_TRANSPORT_MODE: auto|webhook|polling
	TelegramWebhookURL      string // CARRIER_TELEGRAM_WEBHOOK_URL
	TelegramPollingTimeout  int    // CARRIER_TELEGRAM_POLLING_TIMEOUT_SEC
	TelegramAPIBaseURL      string // CARRIER_TELEGRAM_API_BASE_URL

	// Limits
	MaxCommandBodyBytes    int           // CARRIER_MAX_COMMAND_BODY_BYTES (default 64KB)
	WorkerLeaseStaleAfter  time.Duration // CARRIER_WORKER_LEASE_STALE_AFTER_SEC (default 10m)
	WorkerHeartbeatTimeout time.Duration // CARRIER_WORKER_HEARTBEAT_TIMEOUT_SEC (default 2m)

	// Rate limits
	RateLimitPerSession int           // CARRIER_RATE_LIMIT_PER_SESSION (default 30)
	RateLimitGlobal     int           // CARRIER_RATE_LIMIT_GLOBAL (default 200)
	RateLimitWindow     time.Duration // CARRIER_RATE_LIMIT_WINDOW_MS (default 60s)

	// Data
	DataDir      string // SESSION_DATA_DIR or ARTIFACT_ROOT
	ArtifactRoot string // ARTIFACT_ROOT

	// Feature flags
	RemoteControlPlaneEnabled bool // CARRIER_REMOTE_CONTROL_PLANE_ENABLED
	RemoteChatEnabled         bool // CARRIER_REMOTE_CHAT_ENABLED
	ProviderBindingEnabled    bool // CARRIER_PROVIDER_BINDING_ENABLED

	// Remote alerting
	RemoteAlertWebhookURL string        // CARRIER_REMOTE_ALERT_WEBHOOK_URL
	RemoteAlertInterval   time.Duration // CARRIER_REMOTE_ALERT_INTERVAL_SEC
	RemoteAlertCooldown   time.Duration // CARRIER_REMOTE_ALERT_COOLDOWN_SEC
}

// LoadGatewayConfigFromEnv loads GatewayConfig from environment variables.
func LoadGatewayConfigFromEnv() *GatewayConfig {
	cfg := &GatewayConfig{
		Port:                    parseEnvInt("CARRIER_GATEWAY_PORT", 8787),
		Hostname:                envOrDefault("CARRIER_GATEWAY_HOST", "127.0.0.1"),
		APIToken:                strings.TrimSpace(os.Getenv("CARRIER_GATEWAY_API_TOKEN")),
		RoleTokens:              parseGatewayRoleTokens(strings.TrimSpace(os.Getenv("CARRIER_GATEWAY_ROLE_TOKENS"))),
		DaemonBaseURL:           strings.TrimRight(strings.TrimSpace(envOrDefault("CARRIER_DAEMON_BASE_URL", "http://127.0.0.1:9090")), "/"),
		DaemonToken:             strings.TrimSpace(os.Getenv("CARRIER_SERVER_API_TOKEN")),
		DaemonTimeout:           time.Duration(parseEnvInt("CARRIER_DAEMON_TIMEOUT_MS", int(defaultDaemonTimeout/time.Millisecond))) * time.Millisecond,
		DiscordPublicKey:        strings.TrimSpace(os.Getenv("CARRIER_DISCORD_PUBLIC_KEY")),
		FeishuVerificationToken: strings.TrimSpace(os.Getenv("CARRIER_FEISHU_VERIFICATION_TOKEN")),
		TelegramWebhookSecret:   strings.TrimSpace(os.Getenv("CARRIER_TELEGRAM_WEBHOOK_SECRET")),
		TelegramBotToken:        strings.TrimSpace(os.Getenv("CARRIER_TELEGRAM_BOT_TOKEN")),
		TelegramTransportMode:   strings.ToLower(strings.TrimSpace(envOrDefault("CARRIER_TELEGRAM_TRANSPORT_MODE", "auto"))),
		TelegramWebhookURL:      strings.TrimSpace(os.Getenv("CARRIER_TELEGRAM_WEBHOOK_URL")),
		TelegramPollingTimeout:  parseEnvInt("CARRIER_TELEGRAM_POLLING_TIMEOUT_SEC", 30),
		TelegramAPIBaseURL:      strings.TrimSpace(envOrDefault("CARRIER_TELEGRAM_API_BASE_URL", "https://api.telegram.org")),
		MaxCommandBodyBytes:     parseEnvInt("CARRIER_MAX_COMMAND_BODY_BYTES", defaultMaxCommandBodyBytes),
		WorkerLeaseStaleAfter:   time.Duration(parseEnvInt("CARRIER_WORKER_LEASE_STALE_AFTER_SEC", 600)) * time.Second,
		WorkerHeartbeatTimeout:  time.Duration(parseEnvInt("CARRIER_WORKER_HEARTBEAT_TIMEOUT_SEC", 120)) * time.Second,
		RateLimitPerSession:     parseEnvInt("CARRIER_RATE_LIMIT_PER_SESSION", 30),
		RateLimitGlobal:         parseEnvInt("CARRIER_RATE_LIMIT_GLOBAL", 200),
		RateLimitWindow:         time.Duration(parseEnvInt("CARRIER_RATE_LIMIT_WINDOW_MS", 60000)) * time.Millisecond,
		RemoteControlPlaneEnabled: parseEnvBool(
			"CARRIER_REMOTE_CONTROL_PLANE_ENABLED",
			true,
		),
		RemoteChatEnabled: parseEnvBool(
			"CARRIER_REMOTE_CHAT_ENABLED",
			true,
		),
		ProviderBindingEnabled: parseEnvBool(
			"CARRIER_PROVIDER_BINDING_ENABLED",
			true,
		),
		RemoteAlertWebhookURL: strings.TrimSpace(os.Getenv("CARRIER_REMOTE_ALERT_WEBHOOK_URL")),
		RemoteAlertInterval:   time.Duration(parseEnvInt("CARRIER_REMOTE_ALERT_INTERVAL_SEC", 30)) * time.Second,
		RemoteAlertCooldown:   time.Duration(parseEnvInt("CARRIER_REMOTE_ALERT_COOLDOWN_SEC", 300)) * time.Second,
	}
	normalizeGatewayConfigFeatureFlags(cfg)

	// Determine data dir
	dataDir := strings.TrimSpace(os.Getenv("SESSION_DATA_DIR"))
	if dataDir == "" {
		dataDir = strings.TrimSpace(os.Getenv("ARTIFACT_ROOT"))
	}
	if dataDir == "" {
		cwd, _ := os.Getwd()
		dataDir = cwd
	}
	cfg.DataDir = dataDir

	// Artifact root
	artifactRoot := strings.TrimSpace(os.Getenv("ARTIFACT_ROOT"))
	if artifactRoot == "" {
		artifactRoot = filepath.Join(dataDir, "artifacts")
	}
	cfg.ArtifactRoot = artifactRoot

	return cfg
}

// StartGateway initializes all stores and starts the gateway HTTP server.
// It blocks until the server errors (call in a goroutine for concurrent operation).
func StartGateway(cfg *GatewayConfig) error {
	if cfg == nil {
		cfg = LoadGatewayConfigFromEnv()
	}
	normalizeGatewayConfigFeatureFlags(cfg)

	// Security check
	if cfg.APIToken == "" && !isLoopbackGateway(cfg.Hostname) {
		return fmt.Errorf("CARRIER_GATEWAY_API_TOKEN must be set when binding gateway to non-loopback host %q", cfg.Hostname)
	}
	if cfg.APIToken == "" {
		cfg.Hostname = "127.0.0.1"
		log.Println("[gateway] no API token configured; forcing loopback-only bind (127.0.0.1)")
	}

	// Warn if Discord key not set
	if cfg.DiscordPublicKey == "" {
		log.Println("[gateway] CARRIER_DISCORD_PUBLIC_KEY is not set — all Discord webhook requests will be rejected")
	}

	// Initialize daemon client
	daemon := NewDaemonClient(cfg.DaemonBaseURL, cfg.DaemonToken, cfg.DaemonTimeout)

	// Initialize session store
	sessionPath := filepath.Join(cfg.DataDir, "sessions.json")
	sessions := NewSessionStore(sessionPath, 0, nil)
	sessions.StartPeriodicCleanup()

	// Initialize download store
	downloads := NewDownloadStore(cfg.ArtifactRoot, nil)
	downloads.StartPeriodicCleanup()

	// Initialize rate limiter
	var rl *GatewayRateLimiter
	if cfg.RateLimitPerSession > 0 || cfg.RateLimitGlobal > 0 {
		rl = NewGatewayRateLimiter(cfg.RateLimitPerSession, cfg.RateLimitGlobal, cfg.RateLimitWindow, nil)
	}

	// Initialize onboard store
	onboard := NewOnboardStore()

	// Initialize setup store
	setup := NewSetupStore()

	// Build and start the HTTP server
	mux := buildGatewayMux(cfg, daemon, sessions, downloads, rl, onboard, setup)
	addr := fmt.Sprintf("%s:%d", cfg.Hostname, cfg.Port)
	server := newGatewayHTTPServer(addr, mux)

	transportCtx, cancelTransport := context.WithCancel(context.Background())
	defer cancelTransport()

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("gateway listen %s: %w", addr, err)
	}

	if err := startTelegramTransport(transportCtx, cfg, daemon, sessions, downloads, rl, onboard); err != nil {
		return err
	}
	startRemoteAlertWatchdog(transportCtx, cfg)

	log.Printf("[gateway] listening on http://%s", addr)
	return server.Serve(ln)
}

func envOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func parseEnvInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func parseEnvBool(key string, fallback bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if raw == "" {
		return fallback
	}
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func parseGatewayRoleTokens(raw string) map[string]GatewayRole {
	out := map[string]GatewayRole{}
	for _, entry := range strings.Split(strings.TrimSpace(raw), ",") {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		var roleRaw, token string
		if left, right, ok := strings.Cut(trimmed, ":"); ok {
			roleRaw = left
			token = right
		} else if left, right, ok := strings.Cut(trimmed, "="); ok {
			roleRaw = left
			token = right
		} else {
			continue
		}
		role := normalizeGatewayRole(roleRaw)
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		out[token] = role
	}
	return out
}

// Run starts gateway using environment-based configuration.
func Run() error {
	return startGatewayFn(nil)
}
