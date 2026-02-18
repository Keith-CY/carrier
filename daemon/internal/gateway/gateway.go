package gateway

import (
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// GatewayConfig holds all configuration for the gateway server.
type GatewayConfig struct {
	// Network
	Port     int
	Hostname string

	// Auth
	APIToken string // CARRIER_GATEWAY_API_TOKEN

	// Daemon connection
	DaemonBaseURL string // CARRIER_DAEMON_BASE_URL (default http://127.0.0.1:9090)
	DaemonToken   string // CARRIER_SERVER_API_TOKEN
	DaemonTimeout time.Duration

	// Provider verification
	DiscordPublicKey        string // CARRIER_DISCORD_PUBLIC_KEY
	FeishuVerificationToken string // CARRIER_FEISHU_VERIFICATION_TOKEN
	TelegramWebhookSecret   string // CARRIER_TELEGRAM_WEBHOOK_SECRET

	// Limits
	MaxCommandBodyBytes int // CARRIER_MAX_COMMAND_BODY_BYTES (default 64KB)

	// Rate limits
	RateLimitPerSession int           // CARRIER_RATE_LIMIT_PER_SESSION (default 30)
	RateLimitGlobal     int           // CARRIER_RATE_LIMIT_GLOBAL (default 200)
	RateLimitWindow     time.Duration // CARRIER_RATE_LIMIT_WINDOW_MS (default 60s)

	// Data
	DataDir      string // SESSION_DATA_DIR or ARTIFACT_ROOT
	ArtifactRoot string // ARTIFACT_ROOT
}

// LoadGatewayConfigFromEnv loads GatewayConfig from environment variables.
func LoadGatewayConfigFromEnv() *GatewayConfig {
	cfg := &GatewayConfig{
		Port:                parseEnvInt("CARRIER_GATEWAY_PORT", 8787),
		Hostname:            envOrDefault("CARRIER_GATEWAY_HOST", "127.0.0.1"),
		APIToken:            strings.TrimSpace(os.Getenv("CARRIER_GATEWAY_API_TOKEN")),
		DaemonBaseURL:       strings.TrimRight(strings.TrimSpace(envOrDefault("CARRIER_DAEMON_BASE_URL", "http://127.0.0.1:9090")), "/"),
		DaemonToken:         strings.TrimSpace(os.Getenv("CARRIER_SERVER_API_TOKEN")),
		DaemonTimeout:       time.Duration(parseEnvInt("CARRIER_DAEMON_TIMEOUT_MS", 30000)) * time.Millisecond,
		DiscordPublicKey:    strings.TrimSpace(os.Getenv("CARRIER_DISCORD_PUBLIC_KEY")),
		FeishuVerificationToken: strings.TrimSpace(os.Getenv("CARRIER_FEISHU_VERIFICATION_TOKEN")),
		TelegramWebhookSecret:   strings.TrimSpace(os.Getenv("CARRIER_TELEGRAM_WEBHOOK_SECRET")),
		MaxCommandBodyBytes: parseEnvInt("CARRIER_MAX_COMMAND_BODY_BYTES", defaultMaxCommandBodyBytes),
		RateLimitPerSession: parseEnvInt("CARRIER_RATE_LIMIT_PER_SESSION", 30),
		RateLimitGlobal:     parseEnvInt("CARRIER_RATE_LIMIT_GLOBAL", 200),
		RateLimitWindow:     time.Duration(parseEnvInt("CARRIER_RATE_LIMIT_WINDOW_MS", 60000)) * time.Millisecond,
	}

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

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("gateway listen %s: %w", addr, err)
	}
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
