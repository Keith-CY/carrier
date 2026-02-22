package gateway

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type zeroclawManagedOnboardResult struct {
	WorkspacePath string
	ConfigPath    string
	RecordPath    string
}

var zeroclawChannels = []picoclawChannel{
	{
		ID:         "telegram",
		Name:       "Telegram",
		TokenLabel: "Telegram bot token for ZeroClaw",
	},
}

func isZeroclawAgent(agentID string) bool {
	return strings.EqualFold(strings.TrimSpace(agentID), "zeroclaw")
}

func parseZeroclawChannel(input string) (picoclawChannel, bool) {
	id := strings.ToLower(strings.TrimSpace(input))
	for _, ch := range zeroclawChannels {
		if id == ch.ID {
			return ch, true
		}
	}
	return picoclawChannel{}, false
}

func renderZeroclawChannelPrompt() string {
	lines := []string{
		"Selected agent: **ZeroClaw** (zeroclaw)",
		"",
		"**Step 2 — Choose a channel for ZeroClaw**",
	}
	for _, ch := range zeroclawChannels {
		lines = append(lines, fmt.Sprintf("  • `%s` — %s", ch.ID, ch.Name))
	}
	lines = append(lines, "")
	lines = append(lines, "Reply with the channel ID (e.g. `/onboard telegram`).")
	return strings.Join(lines, "\n")
}

func renderZeroclawChannelTokenPrompt(ch picoclawChannel) string {
	return strings.Join([]string{
		fmt.Sprintf("✅ ZeroClaw channel selected: **%s** (`%s`).", ch.Name, ch.ID),
		"",
		fmt.Sprintf("Paste %s.", ch.TokenLabel),
	}, "\n")
}

func prepareZeroclawManagedOnboard(sess *OnboardSession, actor string) (*zeroclawManagedOnboardResult, error) {
	if sess == nil {
		return nil, fmt.Errorf("nil onboarding session")
	}
	if !isZeroclawAgent(sess.SelectedAgent) {
		return nil, fmt.Errorf("managed onboarding is only supported for zeroclaw")
	}
	if strings.TrimSpace(sess.SelectedChannel) == "" {
		return nil, fmt.Errorf("zeroclaw channel is required")
	}
	if strings.TrimSpace(sess.ChannelToken) == "" {
		return nil, fmt.Errorf("zeroclaw channel token is required")
	}
	if strings.TrimSpace(sess.SelectedProvider) == "" {
		return nil, fmt.Errorf("zeroclaw provider is required")
	}
	provider := GetLLMProvider(sess.SelectedProvider)
	if provider == nil {
		return nil, fmt.Errorf("unknown provider %q", sess.SelectedProvider)
	}

	if sess.EnvVars == nil {
		sess.EnvVars = map[string]string{}
	}
	if strings.TrimSpace(sess.EnvVars["ZEROCLAW_API_KEY"]) == "" {
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
		instanceID = "zeroclaw"
	}
	workspacePath := strings.TrimSpace(sess.WorkspacePath)
	if workspacePath == "" {
		workspacePath = filepath.Join(home, ".zeroclaw", "instances", instanceID, "workspace")
	}
	configPath := filepath.Join(home, ".zeroclaw", "config.json")
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
		return nil, fmt.Errorf("backup existing zeroclaw config: %w", err)
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
	providerKey = mapCarrierProviderToPicoclawProvider(providerKey)

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
	} else if token != "" {
		providerItem["api_key"] = token
	}

	chatID := actorChatID(actor)
	allowFrom := []string{}
	if chatID != "" {
		allowFrom = append(allowFrom, chatID)
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
		"channels": map[string]interface{}{
			sess.SelectedChannel: map[string]interface{}{
				"enabled":    true,
				"token":      sess.ChannelToken,
				"allow_from": allowFrom,
			},
		},
	}

	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal zeroclaw config: %w", err)
	}
	if err := os.WriteFile(configPath, append(raw, '\n'), 0o600); err != nil {
		return nil, fmt.Errorf("write zeroclaw config: %w", err)
	}

	record := map[string]interface{}{
		"instance_id":    instanceID,
		"agent_id":       "zeroclaw",
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

	return &zeroclawManagedOnboardResult{
		WorkspacePath: workspacePath,
		ConfigPath:    configPath,
		RecordPath:    recordPath,
	}, nil
}
