// Package config provides configuration file support for the carrier daemon.
//
// Configuration is loaded from a JSON file (config.json) and can be overridden
// by environment variables with the CARRIER_ prefix.
//
// Supported environment variables:
//   - CARRIER_SERVER_HOST       — server listen host (default "127.0.0.1")
//   - CARRIER_SERVER_PORT       — server listen port (default 9090)
//   - CARRIER_LOG_LEVEL         — logging level: debug, info, warn, error (default "info")
//   - CARRIER_LOG_FORMAT        — logging format: text, json (default "text")
//   - CARRIER_CRASH_THRESHOLD   — crash-loop restart threshold (default 3)
//   - CARRIER_CRASH_WINDOW      — crash-loop window e.g. "5m" (default "5m")
//   - CARRIER_CRASH_COOLDOWN    — crash-loop cooldown e.g. "5m" (default "5m")
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds the daemon configuration.
type Config struct {
	Server    ServerConfig    `json:"server"`
	Log       LogConfig       `json:"log"`
	Lifecycle LifecycleConfig `json:"lifecycle"`
}

// ServerConfig holds the network server settings.
type ServerConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// LogConfig holds the logging settings.
type LogConfig struct {
	Level  string `json:"level"`
	Format string `json:"format"`
}

// LifecycleConfig holds crash-loop detection settings.
type LifecycleConfig struct {
	CrashThreshold int    `json:"crash_threshold"`
	CrashWindow    string `json:"crash_window"`
	CrashCooldown  string `json:"crash_cooldown"`
}

// Default returns a Config with sensible defaults.
func Default() Config {
	return Config{
		Server: ServerConfig{
			Host: "127.0.0.1",
			Port: 9090,
		},
		Log: LogConfig{
			Level:  "info",
			Format: "text",
		},
		Lifecycle: LifecycleConfig{
			CrashThreshold: 3,
			CrashWindow:    "5m",
			CrashCooldown:  "5m",
		},
	}
}

// Load reads configuration from the given JSON file path. If the file does
// not exist, defaults are returned. Environment variables with the CARRIER_
// prefix override file values.
func Load(path string) (Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return cfg, fmt.Errorf("config: read %s: %w", path, err)
		}
		// File not found — use defaults, still apply env overrides.
	} else {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("config: parse %s: %w", path, err)
		}
	}

	applyEnvOverrides(&cfg)
	return cfg, nil
}

// CrashWindowDuration parses the CrashWindow string as a time.Duration.
func (c *Config) CrashWindowDuration() (time.Duration, error) {
	return time.ParseDuration(c.Lifecycle.CrashWindow)
}

// CrashCooldownDuration parses the CrashCooldown string as a time.Duration.
func (c *Config) CrashCooldownDuration() (time.Duration, error) {
	return time.ParseDuration(c.Lifecycle.CrashCooldown)
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("CARRIER_SERVER_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("CARRIER_SERVER_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			cfg.Server.Port = p
		}
	}
	if v := os.Getenv("CARRIER_LOG_LEVEL"); v != "" {
		cfg.Log.Level = v
	}
	if v := os.Getenv("CARRIER_LOG_FORMAT"); v != "" {
		cfg.Log.Format = v
	}
	if v := os.Getenv("CARRIER_CRASH_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Lifecycle.CrashThreshold = n
		}
	}
	if v := os.Getenv("CARRIER_CRASH_WINDOW"); v != "" {
		cfg.Lifecycle.CrashWindow = v
	}
	if v := os.Getenv("CARRIER_CRASH_COOLDOWN"); v != "" {
		cfg.Lifecycle.CrashCooldown = v
	}
}
