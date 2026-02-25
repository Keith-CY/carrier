package gateway

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type managedAgentConfig struct {
	ID             string
	ConfigDir      string
	RequiredEnvKey string
}

type managedOnboardResult struct {
	WorkspacePath string
	ConfigPath    string
	RecordPath    string
}

var managedAgents = map[string]managedAgentConfig{
	"picoclaw": {
		ID:        "picoclaw",
		ConfigDir: ".picoclaw",
	},
	"openclaw": {
		ID:             "openclaw",
		ConfigDir:      ".openclaw",
		RequiredEnvKey: "OPENAI_API_KEY",
	},
	"zeroclaw": {
		ID:        "zeroclaw",
		ConfigDir: ".zeroclaw",
	},
}

var openclawChannels = []picoclawChannel{
	{
		ID:         "telegram",
		Name:       "Telegram",
		TokenLabel: "Telegram bot token for OpenClaw",
	},
}

var zeroclawChannels = []picoclawChannel{
	{
		ID:         "telegram",
		Name:       "Telegram",
		TokenLabel: "Telegram bot token for ZeroClaw",
	},
}

func isManagedAgent(agentID string) bool {
	_, ok := managedAgentByID(agentID)
	return ok
}

func managedAgentByID(agentID string) (managedAgentConfig, bool) {
	cfg, ok := managedAgents[strings.ToLower(strings.TrimSpace(agentID))]
	return cfg, ok
}

func managedAgentDisplayName(agentID string) string {
	switch strings.ToLower(strings.TrimSpace(agentID)) {
	case "picoclaw":
		return "PicoClaw"
	case "openclaw":
		return "OpenClaw"
	case "zeroclaw":
		return "ZeroClaw"
	default:
		return strings.TrimSpace(agentID)
	}
}

func parseManagedChannel(agentID, input string) (picoclawChannel, bool) {
	id := strings.ToLower(strings.TrimSpace(input))
	if id == "" {
		return picoclawChannel{}, false
	}
	channels, ok := managedAgentChannels(agentID)
	if !ok {
		return picoclawChannel{}, false
	}
	for _, ch := range channels {
		if id == ch.ID {
			return ch, true
		}
	}
	return picoclawChannel{}, false
}

func renderManagedChannelPrompt(agentID string) string {
	channels, ok := managedAgentChannels(agentID)
	if !ok {
		return "Unsupported managed agent."
	}
	name := managedAgentDisplayName(agentID)
	lines := []string{
		fmt.Sprintf("Selected agent: **%s** (%s)", name, strings.ToLower(strings.TrimSpace(agentID))),
		"",
		fmt.Sprintf("**Step 2 — Choose a channel for %s**", name),
	}
	for _, ch := range channels {
		lines = append(lines, fmt.Sprintf("  • `%s` — %s", ch.ID, ch.Name))
	}
	lines = append(lines, "")
	lines = append(lines, "Reply with the channel ID (e.g. `/onboard telegram`).")
	return strings.Join(lines, "\n")
}

func renderManagedChannelTokenPrompt(agentID string, ch picoclawChannel) string {
	name := managedAgentDisplayName(agentID)
	return strings.Join([]string{
		fmt.Sprintf("✅ %s channel selected: **%s** (`%s`).", name, ch.Name, ch.ID),
		"",
		fmt.Sprintf("Paste %s.", ch.TokenLabel),
	}, "\n")
}

func managedAgentChannels(agentID string) ([]picoclawChannel, bool) {
	switch strings.ToLower(strings.TrimSpace(agentID)) {
	case "picoclaw":
		return picoclawChannels, true
	case "openclaw":
		return openclawChannels, true
	case "zeroclaw":
		return zeroclawChannels, true
	default:
		return nil, false
	}
}

func prepareManagedOnboard(agentID string, sess *OnboardSession, actor string) (*managedOnboardResult, error) {
	cfg, ok := managedAgentByID(agentID)
	if !ok {
		return nil, fmt.Errorf("managed onboarding is only supported for picoclaw/openclaw/zeroclaw")
	}
	if sess == nil {
		return nil, fmt.Errorf("nil onboarding session")
	}
	if !strings.EqualFold(strings.TrimSpace(sess.SelectedAgent), cfg.ID) {
		return nil, fmt.Errorf("managed onboarding is only supported for %s", cfg.ID)
	}
	if strings.TrimSpace(sess.SelectedChannel) == "" {
		return nil, fmt.Errorf("%s channel is required", cfg.ID)
	}
	if strings.TrimSpace(sess.ChannelToken) == "" {
		return nil, fmt.Errorf("%s channel token is required", cfg.ID)
	}
	if strings.TrimSpace(sess.SelectedProvider) == "" {
		return nil, fmt.Errorf("%s provider is required", cfg.ID)
	}
	provider := GetLLMProvider(sess.SelectedProvider)
	if provider == nil {
		return nil, fmt.Errorf("unknown provider %q", sess.SelectedProvider)
	}

	if sess.EnvVars == nil {
		sess.EnvVars = map[string]string{}
	}
	if cfg.RequiredEnvKey != "" && strings.TrimSpace(sess.EnvVars[cfg.RequiredEnvKey]) == "" {
		if token := pickProviderToken(provider, sess.EnvVars); token != "" {
			sess.EnvVars[cfg.RequiredEnvKey] = token
		}
	}
	if cfg.RequiredEnvKey != "" && strings.TrimSpace(sess.EnvVars[cfg.RequiredEnvKey]) == "" {
		return nil, fmt.Errorf("%s requires %s", cfg.ID, cfg.RequiredEnvKey)
	}
	if strings.EqualFold(cfg.ID, "zeroclaw") && strings.TrimSpace(sess.EnvVars["ZEROCLAW_API_KEY"]) == "" {
		if token := pickProviderToken(provider, sess.EnvVars); token != "" {
			sess.EnvVars["ZEROCLAW_API_KEY"] = token
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	instanceID := strings.TrimSpace(sess.InstanceID)
	if instanceID == "" {
		instanceID = cfg.ID
	}
	workspacePath := strings.TrimSpace(sess.WorkspacePath)
	if workspacePath == "" {
		workspacePath = filepath.Join(home, cfg.ConfigDir, "instances", instanceID, "workspace")
	}
	configPath := filepath.Join(home, cfg.ConfigDir, "config.json")
	recordPath := filepath.Join(home, ".carrier", "agents", instanceID+".json")

	if err := os.MkdirAll(workspacePath, 0o700); err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o700); err != nil {
		return nil, fmt.Errorf("create record dir: %w", err)
	}

	if err := backupIfExists(configPath); err != nil {
		return nil, fmt.Errorf("backup existing %s config: %w", cfg.ID, err)
	}

	modelID := strings.TrimSpace(provider.ExampleModel)
	if modelID == "" {
		modelID = provider.ID + "/default"
	}
	if strings.EqualFold(provider.ID, "openai-codex") {
		if _, name, ok := strings.Cut(modelID, "/"); ok && strings.TrimSpace(name) != "" {
			modelID = "openai/" + strings.TrimSpace(name)
		} else {
			modelID = "openai/gpt-5.3-codex"
		}
	}
	modelName := modelID
	if _, name, ok := strings.Cut(modelID, "/"); ok && strings.TrimSpace(name) != "" {
		modelName = strings.TrimSpace(name)
	}
	providerKey := provider.ID
	if vendor, _, ok := strings.Cut(modelID, "/"); ok && strings.TrimSpace(vendor) != "" {
		providerKey = strings.TrimSpace(vendor)
	}
	providerKey = mapCarrierProviderToManagedProvider(providerKey)
	modelItem := map[string]interface{}{
		"model_name": modelName,
		"model":      modelID,
	}
	providerItem := map[string]interface{}{
		"credential_ref": provider.ID,
	}
	token := pickProviderToken(provider, sess.EnvVars)
	if strings.EqualFold(provider.ID, "openai-codex") {
		modelItem["auth_method"] = "oauth"
		providerItem["auth_method"] = "oauth"
		if strings.EqualFold(cfg.ID, "picoclaw") {
			accountID := extractOpenAIAccountID(token)
			if err := savePicoclawAuthCredential(home, "openai", token, accountID); err != nil {
				return nil, fmt.Errorf("write picoclaw auth store: %w", err)
			}
		}
	} else if token != "" {
		providerItem["api_key"] = token
	}

	chatID := actorChatID(actor)
	allowFrom := []string{}
	if chatID != "" {
		allowFrom = append(allowFrom, chatID)
	}
	channels := map[string]interface{}{
		sess.SelectedChannel: map[string]interface{}{
			"enabled":    true,
			"token":      sess.ChannelToken,
			"allow_from": allowFrom,
		},
	}

	payload := map[string]interface{}{
		"agents": map[string]interface{}{
			"defaults": map[string]interface{}{
				"workspace":             workspacePath,
				"provider":              providerKey,
				"model":                 modelName,
				"max_tokens":            8192,
				"temperature":           0.7,
				"max_tool_iterations":   20,
				"restrict_to_workspace": true,
			},
		},
		"model_list": []interface{}{modelItem},
		"providers": map[string]interface{}{
			providerKey: providerItem,
		},
		"channels": channels,
	}

	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal %s config: %w", cfg.ID, err)
	}
	if err := os.WriteFile(configPath, append(raw, '\n'), 0o600); err != nil {
		return nil, fmt.Errorf("write %s config: %w", cfg.ID, err)
	}

	record := map[string]interface{}{
		"instance_id":    instanceID,
		"agent_id":       cfg.ID,
		"workspace_path": workspacePath,
		"config_path":    configPath,
		"channel":        sess.SelectedChannel,
		"provider":       provider.ID,
		"updated_at":     time.Now().UTC().Format(time.RFC3339Nano),
	}
	recordRaw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal carrier record: %w", err)
	}
	if err := os.WriteFile(recordPath, append(recordRaw, '\n'), 0o600); err != nil {
		return nil, fmt.Errorf("write carrier record: %w", err)
	}

	return &managedOnboardResult{
		WorkspacePath: workspacePath,
		ConfigPath:    configPath,
		RecordPath:    recordPath,
	}, nil
}

func mapCarrierProviderToManagedProvider(providerID string) string {
	switch strings.ToLower(strings.TrimSpace(providerID)) {
	case "openai-codex":
		return "openai"
	case "openai-compatible", "vllm", "openai-v1":
		return "openai"
	default:
		return strings.TrimSpace(providerID)
	}
}
