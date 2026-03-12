package gateway

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"carrier/shared/catalog"
	sharedconfig "carrier/shared/config"
	"carrier/shared/openclawcfg"
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
	ProfileName    string
	ModelAlias     string
	ModelID        string
	EnvVar         string
	ProviderID     string
	ProviderKey    string
	ProtocolFamily string
	BaseURL        string
	AuthMethod     string
	TimeoutMs      int
	RetryBudget    int
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
			ProfileName:    strings.TrimSpace(profile.ProfileName),
			ModelAlias:     strings.TrimSpace(profile.ModelAlias),
			ModelID:        strings.TrimSpace(profile.ModelID),
			ProviderID:     strings.TrimSpace(profile.ProviderID),
			ProviderKey:    strings.TrimSpace(profile.ProviderKey),
			ProtocolFamily: strings.TrimSpace(profile.ProtocolFamily),
			BaseURL:        strings.TrimSpace(profile.BaseURL),
			AuthMethod:     strings.TrimSpace(profile.AuthMethod),
			TimeoutMs:      profile.TimeoutMs,
			RetryBudget:    profile.RetryBudget,
			FallbackStrategy: strings.TrimSpace(profile.FallbackStrategy),
			FallbackGroup:  group,
			AliasGroupSize: groupSizes[group],
			Primary:        primary,
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
	providerKey, providerBaseURL, providerToken string,
	profiles []managedModelProfile,
	primaryProfile managedModelProfile,
	workspacePath string,
) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(renderer.ConfigFormat)) {
	case "toml":
		if strings.EqualFold(cfg.ID, "zeroclaw") {
			return renderZeroClawConfigTOML(channelID, channelToken, channelSetupPending, allowFrom, providerKey, providerToken, primaryProfile.ModelID, profiles), nil
		}
		return nil, fmt.Errorf("toml renderer is not supported for %s", cfg.ID)
	case "json":
		var payload map[string]interface{}
		if strings.EqualFold(cfg.ID, "openclaw") {
			payload = buildManagedOpenClawJSONConfigPayload(channelID, channelToken, channelSetupPending, allowFrom, provider, providerKey, providerBaseURL, providerToken, primaryProfile.ModelID, workspacePath)
		} else {
			payload = buildManagedPicoClawJSONConfigPayload(channelID, channelToken, channelSetupPending, allowFrom, provider, providerKey, providerToken, profiles, primaryProfile, workspacePath)
		}
		raw, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return nil, err
		}
		return append(raw, '\n'), nil
	default:
		return nil, fmt.Errorf("unsupported config format %q", renderer.ConfigFormat)
	}
}

func buildManagedPicoClawJSONConfigPayload(
	channelID, channelToken string,
	channelSetupPending bool,
	allowFrom []string,
	provider *LLMProvider,
	providerKey, providerToken string,
	profiles []managedModelProfile,
	primaryProfile managedModelProfile,
	workspacePath string,
) map[string]interface{} {
	modelList := make([]interface{}, 0, len(profiles))
	providerProfiles := map[string]interface{}{}
	providers := map[string]interface{}{}
	for _, profile := range profiles {
		modelItem := map[string]interface{}{
			"model_name":      profile.ProfileName,
			"model":           profile.ModelID,
			"protocol_family": profile.ProtocolFamily,
		}
		if strings.TrimSpace(profile.ModelAlias) != "" {
			modelItem["model_alias"] = profile.ModelAlias
		}
		if strings.TrimSpace(profile.BaseURL) != "" {
			modelItem["base_url"] = profile.BaseURL
		}
		if strings.TrimSpace(profile.AuthMethod) != "" {
			modelItem["auth_method"] = profile.AuthMethod
		}
		modelList = append(modelList, modelItem)

		profileEntry := map[string]interface{}{
			"provider":        profile.ProviderKey,
			"provider_id":     profile.ProviderID,
			"protocol_family": profile.ProtocolFamily,
			"model":           profile.ModelID,
			"credential_ref":  provider.ID,
		}
		if strings.TrimSpace(profile.ModelAlias) != "" {
			profileEntry["model_alias"] = profile.ModelAlias
		}
		if strings.TrimSpace(profile.BaseURL) != "" {
			profileEntry["base_url"] = profile.BaseURL
		}
		if strings.TrimSpace(profile.AuthMethod) != "" {
			profileEntry["auth_method"] = profile.AuthMethod
		}
		providerProfiles[profile.ProfileName] = profileEntry

		if _, ok := providers[profile.ProviderKey]; !ok {
			providerItem := map[string]interface{}{
				"credential_ref": provider.ID,
			}
			if strings.TrimSpace(profile.AuthMethod) != "" {
				providerItem["auth_method"] = profile.AuthMethod
			} else if providerToken != "" {
				providerItem["api_key"] = providerToken
			}
			providers[profile.ProviderKey] = providerItem
		}
	}

	channels := map[string]interface{}{}
	if strings.TrimSpace(channelID) != "" {
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
		channels[channelID] = channelConfig
	}

	return map[string]interface{}{
		"agents": map[string]interface{}{
			"defaults": map[string]interface{}{
				"workspace":             workspacePath,
				"provider":              providerKey,
				"model":                 deriveManagedModelName(primaryProfile.ModelID),
				"max_tokens":            8192,
				"temperature":           0.7,
				"max_tool_iterations":   20,
				"restrict_to_workspace": true,
			},
		},
		"model_list":        modelList,
		"provider_profiles": providerProfiles,
		"providers":         providers,
		"channels":          channels,
	}
}

func buildManagedOpenClawJSONConfigPayload(
	channelID, channelToken string,
	channelSetupPending bool,
	allowFrom []string,
	provider *LLMProvider,
	providerKey, providerBaseURL, providerToken, modelID, workspacePath string,
) map[string]interface{} {
	return openclawcfg.BuildManagedConfigPayload(openclawcfg.ManagedPayloadParams{
		ChannelID:           channelID,
		ChannelToken:        channelToken,
		ChannelSetupPending: channelSetupPending,
		AllowFrom:           allowFrom,
		ProviderID:          provider.ID,
		ProviderKey:         providerKey,
		ProviderBaseURL:     providerBaseURL,
		IncludeAPIKeyRef:    strings.TrimSpace(providerToken) != "",
		ModelID:             modelID,
		WorkspacePath:       workspacePath,
	})
}

func renderZeroClawConfigTOML(
	channelID, channelToken string,
	channelSetupPending bool,
	allowFrom []string,
	providerKey, providerToken, modelID string,
	profiles []managedModelProfile,
) []byte {
	if strings.TrimSpace(providerKey) == "" {
		providerKey = "openai"
	}
	if strings.TrimSpace(modelID) == "" {
		modelID = "anthropic/claude-sonnet-4-6"
	}
	modelID = sharedconfig.NormalizeModelForProvider(providerKey, modelID)
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
	}
	if len(profiles) > 0 {
		lines = append(lines, "", "# Managed provider profiles")
		for _, profile := range profiles {
			lines = append(lines,
				"",
				fmt.Sprintf("[provider_profiles.%s]", sanitizeManagedProfileSectionName(profile.ProfileName)),
				fmt.Sprintf("protocol_family = %s", strconv.Quote(profile.ProtocolFamily)),
				fmt.Sprintf("provider = %s", strconv.Quote(profile.ProviderKey)),
				fmt.Sprintf("provider_id = %s", strconv.Quote(profile.ProviderID)),
			)
			if strings.TrimSpace(profile.ModelAlias) != "" {
				lines = append(lines, fmt.Sprintf("model_alias = %s", strconv.Quote(profile.ModelAlias)))
			}
			lines = append(lines, fmt.Sprintf("model = %s", strconv.Quote(sharedconfig.NormalizeModelForProvider(profile.ProviderKey, profile.ModelID))))
			if strings.TrimSpace(profile.BaseURL) != "" {
				lines = append(lines, fmt.Sprintf("base_url = %s", strconv.Quote(profile.BaseURL)))
			}
			if strings.TrimSpace(profile.EnvVar) != "" {
				lines = append(lines, fmt.Sprintf("credential_env = %s", strconv.Quote(profile.EnvVar)))
			}
		}
	}
	if strings.TrimSpace(channelID) == "" {
		lines = append(lines,
			"",
			"# No chat channel configured (WebUI-only mode)",
		)
		return []byte(strings.Join(lines, "\n") + "\n")
	}

	lines = append(lines,
		"",
		fmt.Sprintf("[channels_config.%s]", channelID),
		fmt.Sprintf("bot_token = %s", strconv.Quote(channelToken)),
		fmt.Sprintf("allowed_users = %s", allowedUsers),
		"mention_only = false",
	)
	if channelSetupPending {
		lines = append(lines,
			"",
			"# channel setup is pending; configure channel token in Web UI before enabling channel start",
		)
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

func resolveManagedModelProfiles(provider *LLMProvider) []managedModelProfile {
	if provider == nil {
		return []managedModelProfile{buildFallbackManagedModelProfile(nil)}
	}
	configuredProfiles, err := sharedconfig.LoadCarrierModelProfilesForProvider(provider.ID)
	if err == nil && len(configuredProfiles) > 0 {
		profiles := make([]managedModelProfile, 0, len(configuredProfiles))
		for _, configured := range configuredProfiles {
			profiles = append(profiles, buildManagedModelProfile(provider, configured))
		}
		return profiles
	}
	return []managedModelProfile{buildFallbackManagedModelProfile(provider)}
}

func buildManagedModelProfile(provider *LLMProvider, configured sharedconfig.CarrierDefaultModel) managedModelProfile {
	modelID := normalizeManagedModelID(configured.ProviderID, configured.ModelID)
	providerKey := deriveManagedProviderKey(configured.ProviderID, modelID)
	baseURL := strings.TrimSpace(configured.BaseURL)
	if baseURL == "" {
		baseURL = resolveManagedProviderBaseURL(provider, providerKey)
	}
	authMethod := ""
	if catalog.IsOpenAICodexProviderID(configured.ProviderID) {
		authMethod = "oauth"
	}
	return managedModelProfile{
		ProfileName:    firstNonEmptyProfile(strings.TrimSpace(configured.ModelName), deriveManagedModelName(modelID)),
		ModelAlias:     strings.TrimSpace(configured.ModelAlias),
		ModelID:        modelID,
		EnvVar:         strings.TrimSpace(configured.EnvVar),
		ProviderID:     strings.TrimSpace(configured.ProviderID),
		ProviderKey:    providerKey,
		ProtocolFamily: firstNonEmptyProfile(strings.TrimSpace(configured.ProtocolFamily), catalog.ProtocolFamilyForProvider(configured.ProviderID)),
		BaseURL:        baseURL,
		AuthMethod:     authMethod,
	}
}

func buildFallbackManagedModelProfile(provider *LLMProvider) managedModelProfile {
	providerID := ""
	modelID := ""
	envVar := ""
	if provider != nil {
		providerID = strings.TrimSpace(provider.ID)
		modelID = strings.TrimSpace(provider.ExampleModel)
		envVar = strings.TrimSpace(provider.EnvVar)
	}
	if modelID == "" {
		modelID = providerID + "/default"
	}
	modelID = normalizeManagedModelID(providerID, modelID)
	providerKey := deriveManagedProviderKey(providerID, modelID)
	authMethod := ""
	if catalog.IsOpenAICodexProviderID(providerID) {
		authMethod = "oauth"
	}
	return managedModelProfile{
		ProfileName:    deriveManagedModelName(modelID),
		ModelID:        modelID,
		EnvVar:         envVar,
		ProviderID:     providerID,
		ProviderKey:    providerKey,
		ProtocolFamily: catalog.ProtocolFamilyForProvider(providerID),
		BaseURL:        resolveManagedProviderBaseURL(provider, providerKey),
		AuthMethod:     authMethod,
	}
}

func normalizeManagedModelID(providerID, modelID string) string {
	modelID = strings.TrimSpace(modelID)
	if catalog.IsOpenAICodexProviderID(providerID) {
		if _, name, ok := strings.Cut(modelID, "/"); ok && strings.TrimSpace(name) != "" {
			return "openai/" + strings.TrimSpace(name)
		}
		return "openai/gpt-5.3-codex"
	}
	return modelID
}

func deriveManagedProviderKey(providerID, modelID string) string {
	providerKey := strings.TrimSpace(providerID)
	if vendor, _, ok := strings.Cut(strings.TrimSpace(modelID), "/"); ok && strings.TrimSpace(vendor) != "" {
		providerKey = strings.TrimSpace(vendor)
	}
	return mapCarrierProviderToManagedProvider(providerKey)
}

func deriveManagedModelName(modelID string) string {
	modelName := strings.TrimSpace(modelID)
	if _, name, ok := strings.Cut(modelName, "/"); ok && strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	if modelName == "" {
		return "default"
	}
	return modelName
}

func sanitizeManagedProfileSectionName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "default"
	}
	var b strings.Builder
	for _, r := range raw {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return strings.Trim(strings.ToLower(b.String()), "_")
}

func firstNonEmptyProfile(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func mapCarrierProviderToManagedProvider(providerID string) string {
	return catalog.MapToManagedProvider(providerID)
}

func resolveManagedProviderBaseURL(provider *LLMProvider, providerKey string) string {
	if provider == nil {
		return ""
	}
	if base := strings.TrimSpace(provider.DefaultBase); base != "" {
		return base
	}
	return catalog.ResolveProviderBaseURL(provider.ID, providerKey, "")
}
