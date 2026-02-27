package gateway

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type managedAgentConfig struct {
	ID             string
	ConfigDir      string
	ConfigFile     string
	RequiredEnvKey string
}

type managedOnboardResult struct {
	WorkspacePath string
	ConfigPath    string
	RecordPath    string
	RendererID    string
	ConfigFormat  string
	AgentVersion  string
	VersionSource string
}

var managedAgents = map[string]managedAgentConfig{
	"picoclaw": {
		ID:         "picoclaw",
		ConfigDir:  ".picoclaw",
		ConfigFile: "config.json",
	},
	"openclaw": {
		ID:             "openclaw",
		ConfigDir:      ".openclaw",
		ConfigFile:     "config.json",
		RequiredEnvKey: "OPENAI_API_KEY",
	},
	"zeroclaw": {
		ID:         "zeroclaw",
		ConfigDir:  ".zeroclaw",
		ConfigFile: "config.toml",
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
	lines = append(lines, "Bot token input is skipped in chat onboarding; configure it later in Web UI.")
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
	channelToken := strings.TrimSpace(sess.ChannelToken)
	channelSetupPending := sess.ChannelSetupPending
	if !channelSetupPending && channelToken == "" {
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

	renderer, err := resolveManagedRenderer(cfg.ID)
	if err != nil {
		return nil, fmt.Errorf("select %s config renderer: %w", cfg.ID, err)
	}

	instanceID := strings.TrimSpace(sess.InstanceID)
	if instanceID == "" {
		instanceID = cfg.ID
	}
	workspacePath := strings.TrimSpace(sess.WorkspacePath)
	if workspacePath == "" {
		workspacePath = filepath.Join(home, cfg.ConfigDir, "instances", instanceID, "workspace")
	}
	configPath := resolveManagedConfigPath(home, cfg, renderer)
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
	providerToken := pickProviderToken(provider, sess.EnvVars)
	if strings.EqualFold(provider.ID, "openai-codex") && strings.EqualFold(cfg.ID, "picoclaw") {
		accountID := extractOpenAIAccountID(providerToken)
		if err := savePicoclawAuthCredential(home, "openai", providerToken, accountID); err != nil {
			return nil, fmt.Errorf("write picoclaw auth store: %w", err)
		}
	}

	chatID := actorChatID(actor)
	allowFrom := []string{}
	if chatID != "" {
		allowFrom = append(allowFrom, chatID)
	}

	configRaw, err := renderManagedConfigBytes(
		renderer,
		cfg,
		sess.SelectedChannel,
		channelToken,
		channelSetupPending,
		allowFrom,
		provider,
		providerKey,
		providerToken,
		modelID,
		modelName,
		workspacePath,
	)
	if err != nil {
		return nil, fmt.Errorf("render %s config (%s): %w", cfg.ID, renderer.RendererID, err)
	}
	if err := os.WriteFile(configPath, configRaw, 0o600); err != nil {
		return nil, fmt.Errorf("write %s config: %w", cfg.ID, err)
	}

	record := map[string]interface{}{
		"instance_id":           instanceID,
		"agent_id":              cfg.ID,
		"workspace_path":        workspacePath,
		"config_path":           configPath,
		"channel":               sess.SelectedChannel,
		"channel_setup_pending": channelSetupPending,
		"provider":              provider.ID,
		"renderer_id":           renderer.RendererID,
		"config_format":         renderer.ConfigFormat,
		"agent_version":         renderer.AgentVersion,
		"version_source":        renderer.VersionSource,
		"compat_repository":     renderer.Repository,
		"compat_fingerprint":    renderer.ExpectedFingerprint,
		"compat_version_range":  renderer.VersionRange,
		"updated_at":            time.Now().UTC().Format(time.RFC3339Nano),
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
		RendererID:    renderer.RendererID,
		ConfigFormat:  renderer.ConfigFormat,
		AgentVersion:  renderer.AgentVersion,
		VersionSource: renderer.VersionSource,
	}, nil
}

func resolveManagedConfigPath(home string, cfg managedAgentConfig, renderer managedRendererSelection) string {
	configPath := strings.TrimSpace(renderer.ConfigPath)
	if configPath == "" {
		configPath = filepath.Join(home, cfg.ConfigDir, cfg.ConfigFile)
	}
	if strings.HasPrefix(configPath, "~/") {
		return filepath.Join(home, strings.TrimPrefix(configPath, "~/"))
	}
	if filepath.IsAbs(configPath) {
		return filepath.Clean(configPath)
	}
	return filepath.Join(home, cfg.ConfigDir, configPath)
}

func renderManagedConfigBytes(
	renderer managedRendererSelection,
	cfg managedAgentConfig,
	channelID, channelToken string,
	channelSetupPending bool,
	allowFrom []string,
	provider *LLMProvider,
	providerKey, providerToken, modelID, modelName, workspacePath string,
) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(renderer.ConfigFormat)) {
	case "toml":
		if strings.EqualFold(cfg.ID, "zeroclaw") {
			return renderZeroClawConfigTOML(channelID, channelToken, channelSetupPending, allowFrom, providerKey, providerToken, modelID), nil
		}
		return nil, fmt.Errorf("toml renderer is not supported for %s", cfg.ID)
	case "json":
		payload := buildManagedJSONConfigPayload(channelID, channelToken, channelSetupPending, allowFrom, provider, providerKey, providerToken, modelID, modelName, workspacePath)
		raw, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return nil, err
		}
		return append(raw, '\n'), nil
	default:
		return nil, fmt.Errorf("unsupported config format %q", renderer.ConfigFormat)
	}
}

func buildManagedJSONConfigPayload(
	channelID, channelToken string,
	channelSetupPending bool,
	allowFrom []string,
	provider *LLMProvider,
	providerKey, providerToken, modelID, modelName, workspacePath string,
) map[string]interface{} {
	modelItem := map[string]interface{}{
		"model_name": modelName,
		"model":      modelID,
	}
	providerItem := map[string]interface{}{
		"credential_ref": provider.ID,
	}
	if strings.EqualFold(provider.ID, "openai-codex") {
		modelItem["auth_method"] = "oauth"
		providerItem["auth_method"] = "oauth"
	} else if providerToken != "" {
		providerItem["api_key"] = providerToken
	}

	channelConfig := map[string]interface{}{
		"enabled":    true,
		"allow_from": allowFrom,
	}
	if channelSetupPending {
		channelConfig["enabled"] = false
		channelConfig["setup_pending"] = true
	} else {
		channelConfig["token"] = channelToken
	}
	channels := map[string]interface{}{
		channelID: channelConfig,
	}

	return map[string]interface{}{
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
}

func renderZeroClawConfigTOML(
	channelID, channelToken string,
	channelSetupPending bool,
	allowFrom []string,
	providerKey, providerToken, modelID string,
) []byte {
	if strings.TrimSpace(channelID) == "" {
		channelID = "telegram"
	}
	if strings.TrimSpace(providerKey) == "" {
		providerKey = "openrouter"
	}
	if strings.TrimSpace(modelID) == "" {
		modelID = "anthropic/claude-sonnet-4-6"
	}
	if strings.TrimSpace(providerToken) == "" {
		providerToken = ""
	}

	allowedUsers := "[]"
	if len(allowFrom) > 0 {
		quoted := make([]string, 0, len(allowFrom))
		for _, raw := range allowFrom {
			id := strings.TrimSpace(raw)
			if id == "" {
				continue
			}
			quoted = append(quoted, strconv.Quote(id))
		}
		if len(quoted) > 0 {
			allowedUsers = "[" + strings.Join(quoted, ", ") + "]"
		}
	}

	lines := []string{
		"# Generated by Carrier managed onboarding",
		"# Edit manually if you need advanced ZeroClaw settings.",
		"",
		fmt.Sprintf("api_key = %s", strconv.Quote(providerToken)),
		fmt.Sprintf("default_provider = %s", strconv.Quote(providerKey)),
		fmt.Sprintf("default_model = %s", strconv.Quote(modelID)),
		"default_temperature = 0.7",
		"",
		"[agent]",
		"max_tool_iterations = 20",
		"",
		fmt.Sprintf("[channels_config.%s]", channelID),
		fmt.Sprintf("bot_token = %s", strconv.Quote(channelToken)),
		fmt.Sprintf("allowed_users = %s", allowedUsers),
		"mention_only = false",
	}
	if channelSetupPending {
		lines = append(lines,
			"",
			"# channel setup is pending; configure channel token in Web UI before enabling channel start",
		)
	}
	return []byte(strings.Join(lines, "\n") + "\n")
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
