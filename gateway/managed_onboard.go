package gateway

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"carrier/shared/catalog"
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
	ModelSurface  managedAgentModelSurface
	RendererID    string
	ConfigFormat  string
	AgentVersion  string
	VersionSource string
}

type managedModelProfile struct {
	ProfileName      string
	ModelAlias       string
	ModelID          string
	EnvVar           string
	ProviderID       string
	ProviderKey      string
	ProtocolFamily   string
	BaseURL          string
	AuthMethod       string
	TimeoutMs        int
	RetryBudget      int
	FallbackStrategy string
}

func buildManagedModelSurface(profiles []managedModelProfile) managedAgentModelSurface {
	surface := managedAgentModelSurface{
		Profiles: make([]managedAgentModelProfile, 0, len(profiles)),
	}
	if len(profiles) == 0 {
		return surface
	}
	surface.DefaultProfile = strings.TrimSpace(profiles[0].ProfileName)
	groupSizes := map[string]int{}
	groupPrimaries := map[string]bool{}
	for _, profile := range profiles {
		group := managedModelFallbackGroup(profile)
		if group == "" {
			continue
		}
		groupSizes[group]++
	}
	for i, profile := range profiles {
		group := managedModelFallbackGroup(profile)
		primary := i == 0
		if group != "" && !groupPrimaries[group] {
			primary = true
			groupPrimaries[group] = true
		}
		surface.Profiles = append(surface.Profiles, managedAgentModelProfile{
			ProfileName:      strings.TrimSpace(profile.ProfileName),
			ModelAlias:       strings.TrimSpace(profile.ModelAlias),
			ModelID:          strings.TrimSpace(profile.ModelID),
			ProviderID:       strings.TrimSpace(profile.ProviderID),
			ProviderKey:      strings.TrimSpace(profile.ProviderKey),
			ProtocolFamily:   strings.TrimSpace(profile.ProtocolFamily),
			BaseURL:          strings.TrimSpace(profile.BaseURL),
			AuthMethod:       strings.TrimSpace(profile.AuthMethod),
			TimeoutMs:        profile.TimeoutMs,
			RetryBudget:      profile.RetryBudget,
			FallbackStrategy: strings.TrimSpace(profile.FallbackStrategy),
			FallbackGroup:    group,
			AliasGroupSize:   groupSizes[group],
			Primary:          primary,
		})
	}
	return surface
}

func managedModelFallbackGroup(profile managedModelProfile) string {
	alias := strings.ToLower(strings.TrimSpace(profile.ModelAlias))
	if alias == "" {
		return ""
	}
	provider := strings.ToLower(strings.TrimSpace(profile.ProviderID))
	if provider == "" {
		provider = strings.ToLower(strings.TrimSpace(profile.ProviderKey))
	}
	if provider == "" {
		return alias
	}
	return provider + ":" + alias
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
		ConfigFile:     "openclaw.json",
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
	selectedChannel := strings.TrimSpace(sess.SelectedChannel)
	channelToken := strings.TrimSpace(sess.ChannelToken)
	// Channel setup is pending if the session says so, or if a channel is selected without a token.
	// WebUI-only mode (no channel selected) is not a "pending" state - it's a deliberate choice.
	channelSetupPending := sess.ChannelSetupPending || (selectedChannel != "" && channelToken == "")
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

	profiles := resolveManagedModelProfiles(provider)
	primaryProfile := profiles[0]
	providerKey := primaryProfile.ProviderKey
	providerBaseURL := primaryProfile.BaseURL
	providerToken := pickProviderToken(provider, sess.EnvVars)
	if catalog.IsOpenAICodexProviderID(provider.ID) && strings.EqualFold(cfg.ID, "picoclaw") {
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
		selectedChannel,
		channelToken,
		channelSetupPending,
		allowFrom,
		provider,
		providerKey,
		providerBaseURL,
		providerToken,
		profiles,
		primaryProfile,
		workspacePath,
	)
	if err != nil {
		return nil, fmt.Errorf("render %s config (%s): %w", cfg.ID, renderer.RendererID, err)
	}
	if err := os.WriteFile(configPath, configRaw, 0o600); err != nil {
		return nil, fmt.Errorf("write %s config: %w", cfg.ID, err)
	}
	if strings.EqualFold(cfg.ID, "openclaw") && strings.TrimSpace(providerToken) != "" {
		secretsPath := filepath.Join(home, ".openclaw", "carrier-secrets.json")
		secrets := map[string]interface{}{
			"providers": map[string]interface{}{
				providerKey: map[string]interface{}{
					"apiKey": providerToken,
				},
			},
		}
		secretsRaw, marshalErr := json.MarshalIndent(secrets, "", "  ")
		if marshalErr != nil {
			return nil, fmt.Errorf("marshal openclaw carrier secrets: %w", marshalErr)
		}
		if err := os.WriteFile(secretsPath, append(secretsRaw, '\n'), 0o600); err != nil {
			return nil, fmt.Errorf("write openclaw carrier secrets: %w", err)
		}
	}

	record := map[string]interface{}{
		"instance_id":           instanceID,
		"agent_id":              cfg.ID,
		"workspace_path":        workspacePath,
		"config_path":           configPath,
		"channel":               selectedChannel,
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
		ModelSurface:  buildManagedModelSurface(profiles),
		RendererID:    renderer.RendererID,
		ConfigFormat:  renderer.ConfigFormat,
		AgentVersion:  renderer.AgentVersion,
		VersionSource: renderer.VersionSource,
	}, nil
}
