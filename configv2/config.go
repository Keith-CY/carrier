package configv2

import (
	"carrier/daemon/credentialstore"
	"carrier/shared/catalog"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	CurrentVersion = 2
)

type Config struct {
	ConfigVersion int           `json:"config_version"`
	Channels      []Channel     `json:"channels,omitempty"`
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
	ID            string   `json:"id"`
	BotToken      string   `json:"bot_token,omitempty"`
	WebhookSecret string   `json:"webhook_secret,omitempty"`
	WebhookURL    string   `json:"webhook_url,omitempty"`
	TransportMode string   `json:"transport_mode,omitempty"` // auto|webhook|polling
	AllowFrom     []string `json:"allow_from,omitempty"`
	Enabled       bool     `json:"enabled"`
}

type Model struct {
	ModelName     string `json:"model_name"`
	Model         string `json:"model"`
	ProviderID    string `json:"provider_id"`
	AuthMode      string `json:"auth_mode,omitempty"`
	EnvVar        string `json:"env_var,omitempty"`
	CredentialRef string `json:"credential_ref,omitempty"` // provider credential key
	BaseURL       string `json:"base_url,omitempty"`
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
	if err := validateConfig(&cfg); err != nil {
		return nil, "", err
	}
	return &cfg, path, nil
}

func Save(cfg *Config) (string, error) {
	if cfg == nil {
		return "", errors.New("nil config")
	}
	if err := validateConfig(cfg); err != nil {
		return "", err
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

func validateConfig(cfg *Config) error {
	if cfg == nil {
		return errors.New("nil config")
	}
	for _, ch := range cfg.Channels {
		channelID := strings.ToLower(strings.TrimSpace(ch.ID))
		if channelID == "" {
			continue
		}
		if !catalog.IsSupportedChannel(channelID) {
			return fmt.Errorf("unsupported channel id %q", ch.ID)
		}
	}
	for _, model := range cfg.ModelList {
		providerID := strings.ToLower(strings.TrimSpace(model.ProviderID))
		if providerID == "" {
			continue
		}
		if !catalog.IsSupportedProvider(providerID) {
			return fmt.Errorf("unsupported provider id %q", model.ProviderID)
		}
		if baseURL := strings.TrimSpace(model.BaseURL); baseURL != "" {
			if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
				return fmt.Errorf("model %q base_url must start with http:// or https://", model.ModelName)
			}
		}
	}
	return nil
}

// ResolveDefaultModel returns the selected default model entry in model_list.
// Selection order:
// 1) model_name matching default_model (case-insensitive)
// 2) first model entry as fallback
func ResolveDefaultModel(cfg *Config) (*Model, error) {
	if cfg == nil {
		return nil, errors.New("nil config")
	}
	if len(cfg.ModelList) == 0 {
		return nil, errors.New("empty model_list")
	}

	pick := cfg.ModelList[0]
	defaultName := strings.TrimSpace(cfg.DefaultModel)
	if defaultName != "" {
		for _, m := range cfg.ModelList {
			if strings.EqualFold(strings.TrimSpace(m.ModelName), defaultName) {
				pick = m
				break
			}
		}
	}
	return &pick, nil
}

func setEnvIfUnset(key, value string) error {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return nil
	}
	if existing, exists := os.LookupEnv(key); exists && strings.TrimSpace(existing) != "" {
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
		if strings.TrimSpace(m.EnvVar) != "" && strings.TrimSpace(m.CredentialRef) != "" {
			value, _, ok, err := credentialstore.LoadProviderCredential(m.CredentialRef)
			if err == nil && ok && strings.TrimSpace(value) != "" {
				if err := setEnvIfUnset(m.EnvVar, value); err != nil {
					return err
				}
			}
		}
		if strings.TrimSpace(m.BaseURL) != "" {
			provider := catalog.GetProvider(m.ProviderID)
			if provider != nil && strings.TrimSpace(provider.BaseURLEnv) != "" {
				if err := setEnvIfUnset(provider.BaseURLEnv, m.BaseURL); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
