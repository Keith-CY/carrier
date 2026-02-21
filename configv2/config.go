package configv2

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	CurrentVersion = 2
)

type Config struct {
	ConfigVersion int           `json:"config_version"`
	Channels      []Channel     `json:"channels"`
	ModelList     []Model       `json:"model_list"`
	DefaultModel  string        `json:"default_model"`
	BaseAgent     BaseAgentSpec `json:"base_agent"`
	ConfiguredAt  string        `json:"configured_at"`
}

type BaseAgentSpec struct {
	Enabled           bool   `json:"enabled"`
	PublicMemoryID    string `json:"public_memory_id"`
	ActiveMemoryID    string `json:"active_memory_id"`
	SelfHealBackupDir string `json:"self_heal_backup_dir,omitempty"`
}

type Channel struct {
	ID            string `json:"id"`
	BotToken      string `json:"bot_token,omitempty"`
	WebhookSecret string `json:"webhook_secret,omitempty"`
	WebhookURL    string `json:"webhook_url,omitempty"`
	TransportMode string `json:"transport_mode,omitempty"` // auto|webhook|polling
	Enabled       bool   `json:"enabled"`
}

type Model struct {
	ModelName     string `json:"model_name"`
	Model         string `json:"model"`
	ProviderID    string `json:"provider_id"`
	AuthMode      string `json:"auth_mode,omitempty"`
	EnvVar        string `json:"env_var,omitempty"`
	CredentialRef string `json:"credential_ref,omitempty"` // provider credential key
}

func DefaultPath() (string, error) {
	if path := strings.TrimSpace(os.Getenv("CARRIER_CONFIG")); path != "" {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".carrier", "config.v2.json"), nil
}

func Load() (*Config, string, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{ConfigVersion: CurrentVersion}, path, nil
		}
		return nil, "", fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, "", fmt.Errorf("parse config: %w", err)
	}
	if cfg.ConfigVersion != CurrentVersion {
		return nil, "", fmt.Errorf("unsupported config_version=%d (expected %d). Please run `carrier onboard` again", cfg.ConfigVersion, CurrentVersion)
	}
	return &cfg, path, nil
}

func Save(cfg *Config) (string, error) {
	if cfg == nil {
		return "", errors.New("nil config")
	}
	cfg.ConfigVersion = CurrentVersion
	path, err := DefaultPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("create config directory: %w", err)
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("write config: %w", err)
	}
	return path, nil
}

func setEnvIfUnset(key, value string) error {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return nil
	}
	if _, exists := os.LookupEnv(key); exists {
		return nil
	}
	return os.Setenv(key, value)
}

func ApplyGatewayEnvironment(cfg *Config) error {
	if cfg == nil {
		return nil
	}

	// Channel settings.
	for _, ch := range cfg.Channels {
		if !ch.Enabled {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(ch.ID)) {
		case "telegram":
			if err := setEnvIfUnset("CARRIER_TELEGRAM_BOT_TOKEN", ch.BotToken); err != nil {
				return err
			}
			if err := setEnvIfUnset("CARRIER_TELEGRAM_WEBHOOK_SECRET", ch.WebhookSecret); err != nil {
				return err
			}
			if err := setEnvIfUnset("CARRIER_TELEGRAM_WEBHOOK_URL", ch.WebhookURL); err != nil {
				return err
			}
			mode := strings.ToLower(strings.TrimSpace(ch.TransportMode))
			if err := setEnvIfUnset("CARRIER_TELEGRAM_TRANSPORT_MODE", mode); err != nil {
				return err
			}
		case "discord":
			if err := setEnvIfUnset("CARRIER_DISCORD_BOT_TOKEN", ch.BotToken); err != nil {
				return err
			}
			if err := setEnvIfUnset("CARRIER_DISCORD_PUBLIC_KEY", ch.WebhookSecret); err != nil {
				return err
			}
		case "feishu":
			if err := setEnvIfUnset("CARRIER_FEISHU_APP_TOKEN", ch.BotToken); err != nil {
				return err
			}
			if err := setEnvIfUnset("CARRIER_FEISHU_VERIFICATION_TOKEN", ch.WebhookSecret); err != nil {
				return err
			}
		}
	}

	// Provider credentials (by credential reference).
	for _, m := range cfg.ModelList {
		if strings.TrimSpace(m.EnvVar) == "" || strings.TrimSpace(m.CredentialRef) == "" {
			continue
		}
		if value, ok := loadCredential(m.CredentialRef); ok {
			if err := setEnvIfUnset(m.EnvVar, value); err != nil {
				return err
			}
		}
	}

	// Expose default model context for components that need runtime inference.
	defaultModel := pickDefaultModel(cfg)
	if defaultModel != nil {
		if err := setEnvIfUnset("CARRIER_DEFAULT_MODEL_NAME", defaultModel.ModelName); err != nil {
			return err
		}
		if err := setEnvIfUnset("CARRIER_DEFAULT_MODEL_ID", defaultModel.Model); err != nil {
			return err
		}
		if err := setEnvIfUnset("CARRIER_DEFAULT_PROVIDER_ID", defaultModel.ProviderID); err != nil {
			return err
		}
		if err := setEnvIfUnset("CARRIER_DEFAULT_PROVIDER_ENV", defaultModel.EnvVar); err != nil {
			return err
		}
	}
	return nil
}

func pickDefaultModel(cfg *Config) *Model {
	if cfg == nil || len(cfg.ModelList) == 0 {
		return nil
	}
	defaultName := strings.TrimSpace(cfg.DefaultModel)
	if defaultName != "" {
		for i := range cfg.ModelList {
			if strings.EqualFold(strings.TrimSpace(cfg.ModelList[i].ModelName), defaultName) {
				return &cfg.ModelList[i]
			}
		}
	}
	return &cfg.ModelList[0]
}

func loadCredential(providerID string) (string, bool) {
	service := "carrier.provider." + strings.TrimSpace(providerID)
	if v, ok := loadCredentialFromKeychain(service); ok {
		return v, true
	}
	if v, ok := loadCredentialFromFile(providerID); ok {
		return v, true
	}
	return "", false
}

func loadCredentialFromKeychain(service string) (string, bool) {
	if runtime.GOOS != "darwin" {
		return "", false
	}
	if strings.TrimSpace(os.Getenv("CARRIER_DISABLE_KEYCHAIN")) == "1" {
		return "", false
	}
	if _, err := exec.LookPath("security"); err != nil {
		return "", false
	}
	cmd := exec.Command("security", "find-generic-password", "-a", "carrier", "-s", service, "-w")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	value := strings.TrimSpace(string(out))
	if value == "" {
		return "", false
	}
	return value, true
}

func loadCredentialFromFile(providerID string) (string, bool) {
	path := strings.TrimSpace(os.Getenv("CARRIER_CREDENTIAL_STORE"))
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false
		}
		path = filepath.Join(home, ".carrier", "credentials.json")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var payload struct {
		Providers map[string]string `json:"providers"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", false
	}
	value := strings.TrimSpace(payload.Providers[providerID])
	if value == "" {
		return "", false
	}
	return value, true
}
