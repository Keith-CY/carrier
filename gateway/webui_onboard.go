package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultOnboardPairCodeTTLSeconds = 300

type webUIOnboardRequest struct {
	Channel         string `json:"channel"`
	ChannelToken    string `json:"channelToken"`
	ChannelSecret   string `json:"channelSecret"`
	ProviderID      string `json:"providerId"`
	ProviderToken   string `json:"providerToken"`
	ReuseCredential bool   `json:"reuseCredential"`
}

type onboardConfigFile struct {
	ConfigVersion int                    `json:"config_version"`
	Channels      []onboardConfigChannel `json:"channels"`
	ModelList     []onboardConfigModel   `json:"model_list"`
	DefaultModel  string                 `json:"default_model"`
	BaseAgent     onboardBaseAgentSpec   `json:"base_agent"`
	ConfiguredAt  string                 `json:"configured_at"`
}

type onboardBaseAgentSpec struct {
	Enabled           bool   `json:"enabled"`
	PublicMemoryID    string `json:"public_memory_id"`
	ActiveMemoryID    string `json:"active_memory_id"`
	SelfHealBackupDir string `json:"self_heal_backup_dir,omitempty"`
}

type onboardConfigChannel struct {
	ID            string `json:"id"`
	BotToken      string `json:"bot_token,omitempty"`
	WebhookSecret string `json:"webhook_secret,omitempty"`
	WebhookURL    string `json:"webhook_url,omitempty"`
	TransportMode string `json:"transport_mode,omitempty"`
	Enabled       bool   `json:"enabled"`
}

type onboardConfigModel struct {
	ModelName     string `json:"model_name"`
	Model         string `json:"model"`
	ProviderID    string `json:"provider_id"`
	AuthMode      string `json:"auth_mode,omitempty"`
	EnvVar        string `json:"env_var,omitempty"`
	CredentialRef string `json:"credential_ref,omitempty"`
}

type daemonPairCodeRecord struct {
	Code      string `json:"code"`
	ExpiresAt string `json:"expiresAt"`
}

func handleWebUIOnboard(w http.ResponseWriter, r *http.Request, requestID string, daemon *DaemonClient) {
	var req webUIOnboardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
		return
	}

	channelID, webUIOnly, err := normalizeOnboardChannel(req.Channel)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
		return
	}
	channelToken := strings.TrimSpace(req.ChannelToken)
	channelSecret := strings.TrimSpace(req.ChannelSecret)
	if !webUIOnly {
		if channelToken == "" {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "channelToken is required"))
			return
		}
		if channelID == "discord" && channelSecret == "" {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "Discord requires channelSecret (public key)"))
			return
		}
	}

	providerID := strings.ToLower(strings.TrimSpace(req.ProviderID))
	if providerID == "" {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_PROVIDER_NOT_FOUND", "providerId is required"))
		return
	}
	provider := GetLLMProvider(providerID)
	if provider == nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_PROVIDER_NOT_FOUND", "providerId is invalid"))
		return
	}

	providerToken, tokenFromInput, err := resolveOnboardProviderCredential(provider, req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_AUTH_INPUT", err.Error()))
		return
	}
	if tokenFromInput && strings.TrimSpace(providerToken) != "" {
		if _, err := saveProviderCredential(provider.ID, providerToken); err != nil {
			writeInternalGatewayError(w, http.StatusBadRequest, "E_AUTH_INPUT", "failed to persist provider credential", "save provider credential", err)
			return
		}
	}

	cfg := buildOnboardConfig(channelID, webUIOnly, channelToken, channelSecret, provider, providerToken)
	cfgPath, err := saveOnboardConfigFile(cfg)
	if err != nil {
		writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to save onboarding config", "save onboarding config", err)
		return
	}
	if err := applyOnboardConfigEnvironment(cfg); err != nil {
		writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to apply onboarding environment", "apply onboarding environment", err)
		return
	}

	pairRequired := !webUIOnly
	pairCode := ""
	pairCodeExpiresAt := ""
	if pairRequired {
		pairCode, pairCodeExpiresAt, err = ensureDaemonPairCode(r.Context(), daemon, requestID)
		if err != nil {
			writeInternalGatewayError(w, http.StatusBadGateway, "E_COMMAND_FAILED", "failed to issue pair code", "issue daemon pair code", err)
			return
		}
	}

	resp := map[string]interface{}{
		"requestId": requestID,
		"result":    "ok",
		"message":   "onboarding configuration saved",
		"onboard": map[string]interface{}{
			"channel":           channelID,
			"providerId":        provider.ID,
			"webuiOnly":         webUIOnly,
			"pairRequired":      pairRequired,
			"pairCode":          pairCode,
			"pairCodeExpiresAt": pairCodeExpiresAt,
			"configPath":        cfgPath,
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

func normalizeOnboardChannel(raw string) (string, bool, error) {
	channelID := strings.ToLower(strings.TrimSpace(raw))
	switch channelID {
	case "", "skip", "none", "webui":
		return "", true, nil
	case "telegram", "discord":
		return channelID, false, nil
	default:
		return "", false, fmt.Errorf("unsupported channel %q; expected telegram, discord, or skip", raw)
	}
}

func resolveOnboardProviderCredential(provider *LLMProvider, req webUIOnboardRequest) (string, bool, error) {
	if provider == nil {
		return "", false, errors.New("provider is required")
	}
	if provider.AuthMode == AuthModeNone {
		return "", false, nil
	}

	token := strings.TrimSpace(req.ProviderToken)
	tokenFromInput := token != ""
	if token == "" && req.ReuseCredential {
		value, _, ok, err := loadProviderCredential(provider.ID)
		if err != nil {
			return "", false, fmt.Errorf("failed to load saved credential for %s: %w", provider.Name, err)
		}
		if ok {
			token = strings.TrimSpace(value)
		}
	}
	if token == "" && strings.TrimSpace(provider.EnvVar) != "" {
		token = strings.TrimSpace(os.Getenv(provider.EnvVar))
	}
	if token == "" {
		return "", false, fmt.Errorf("provider %s requires credential", provider.ID)
	}
	return token, tokenFromInput, nil
}

func buildOnboardConfig(
	channelID string,
	webUIOnly bool,
	channelToken string,
	channelSecret string,
	provider *LLMProvider,
	providerToken string,
) *onboardConfigFile {
	modelName := strings.TrimSpace(provider.ID) + "-default"
	modelID := strings.TrimSpace(provider.ExampleModel)
	if modelID == "" {
		modelID = provider.ID + "/default"
	}
	model := onboardConfigModel{
		ModelName:  modelName,
		Model:      modelID,
		ProviderID: provider.ID,
		AuthMode:   string(provider.AuthMode),
		EnvVar:     strings.TrimSpace(provider.EnvVar),
	}
	if strings.TrimSpace(providerToken) != "" && strings.TrimSpace(provider.EnvVar) != "" {
		model.CredentialRef = provider.ID
	}

	channels := []onboardConfigChannel{}
	if !webUIOnly && channelID != "" {
		ch := onboardConfigChannel{
			ID:            channelID,
			BotToken:      strings.TrimSpace(channelToken),
			WebhookSecret: strings.TrimSpace(channelSecret),
			Enabled:       true,
		}
		if channelID == "telegram" {
			ch.TransportMode = "auto"
		}
		channels = append(channels, ch)
	}

	return &onboardConfigFile{
		ConfigVersion: 2,
		Channels:      channels,
		ModelList:     []onboardConfigModel{model},
		DefaultModel:  modelName,
		BaseAgent: onboardBaseAgentSpec{
			Enabled:           true,
			PublicMemoryID:    "carrier.base.public.v1",
			ActiveMemoryID:    "carrier.base.active.v1",
			SelfHealBackupDir: "base-agent-memory-backups",
		},
		ConfiguredAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func saveOnboardConfigFile(cfg *onboardConfigFile) (string, error) {
	if cfg == nil {
		return "", errors.New("nil config")
	}
	path, err := onboardConfigPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
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

func onboardConfigPath() (string, error) {
	if path := strings.TrimSpace(os.Getenv("CARRIER_CONFIG")); path != "" {
		return path, nil
	}
	if path := strings.TrimSpace(os.Getenv("CARRIER_ONBOARD_CONFIG")); path != "" {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".carrier", "config.v2.json"), nil
}

func applyOnboardConfigEnvironment(cfg *onboardConfigFile) error {
	if cfg == nil {
		return nil
	}
	for _, ch := range cfg.Channels {
		switch strings.ToLower(strings.TrimSpace(ch.ID)) {
		case "telegram":
			if err := setOnboardEnv("CARRIER_TELEGRAM_BOT_TOKEN", ch.BotToken); err != nil {
				return err
			}
			if err := setOnboardEnv("CARRIER_TELEGRAM_WEBHOOK_SECRET", ch.WebhookSecret); err != nil {
				return err
			}
			if err := setOnboardEnv("CARRIER_TELEGRAM_TRANSPORT_MODE", ch.TransportMode); err != nil {
				return err
			}
		case "discord":
			if err := setOnboardEnv("CARRIER_DISCORD_BOT_TOKEN", ch.BotToken); err != nil {
				return err
			}
			if err := setOnboardEnv("CARRIER_DISCORD_PUBLIC_KEY", ch.WebhookSecret); err != nil {
				return err
			}
		}
	}
	for _, m := range cfg.ModelList {
		envName := strings.TrimSpace(m.EnvVar)
		credRef := strings.TrimSpace(m.CredentialRef)
		if envName == "" || credRef == "" {
			continue
		}
		value, _, ok, err := loadProviderCredential(credRef)
		if err != nil || !ok || strings.TrimSpace(value) == "" {
			continue
		}
		if err := setOnboardEnv(envName, value); err != nil {
			return err
		}
		if strings.EqualFold(strings.TrimSpace(m.ProviderID), "openai-codex") {
			if err := setOnboardEnv("OPENAI_API_KEY", value); err != nil {
				return err
			}
		}
	}
	return nil
}

func setOnboardEnv(key, value string) error {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return nil
	}
	if err := os.Setenv(key, value); err != nil {
		return fmt.Errorf("set %s: %w", key, err)
	}
	return nil
}

func ensureDaemonPairCode(ctx context.Context, daemon *DaemonClient, requestID string) (string, string, error) {
	if daemon == nil {
		return "", "", errors.New("daemon client is not available")
	}
	actor := "webui:onboard"
	raw, err := daemon.request(ctx, http.MethodGet, "/api/v1/pairing/codes", nil, actor, requestID)
	if err != nil {
		return "", "", err
	}
	var listed struct {
		Codes []daemonPairCodeRecord `json:"codes"`
	}
	if err := json.Unmarshal(raw, &listed); err != nil {
		return "", "", fmt.Errorf("decode pairing code list: %w", err)
	}
	for _, rec := range listed.Codes {
		if strings.TrimSpace(rec.Code) != "" {
			return strings.TrimSpace(rec.Code), strings.TrimSpace(rec.ExpiresAt), nil
		}
	}
	body := map[string]int{"ttlSeconds": defaultOnboardPairCodeTTLSeconds}
	raw, err = daemon.request(ctx, http.MethodPost, "/api/v1/pairing/codes", body, actor, requestID)
	if err != nil {
		return "", "", err
	}
	var issued daemonPairCodeRecord
	if err := json.Unmarshal(raw, &issued); err != nil {
		return "", "", fmt.Errorf("decode issued pairing code: %w", err)
	}
	if strings.TrimSpace(issued.Code) == "" {
		return "", "", errors.New("daemon returned empty pairing code")
	}
	return strings.TrimSpace(issued.Code), strings.TrimSpace(issued.ExpiresAt), nil
}
