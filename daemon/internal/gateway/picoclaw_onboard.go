package gateway

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type picoclawChannel struct {
	ID         string
	Name       string
	TokenLabel string
}

type picoclawManagedOnboardResult struct {
	WorkspacePath string
	ConfigPath    string
	RecordPath    string
}

var (
	picoclawPairCodePattern = regexp.MustCompile(`\bpair-[a-f0-9]{32}\b`)
	picoclawPairedPattern   = regexp.MustCompile(`(?i)\bpaired\s+telegram:([0-9]+)\b`)
	picoclawChannels        = []picoclawChannel{
		{
			ID:         "telegram",
			Name:       "Telegram",
			TokenLabel: "Telegram bot token for PicoClaw",
		},
	}
)

func isPicoclawAgent(agentID string) bool {
	return strings.EqualFold(strings.TrimSpace(agentID), "picoclaw")
}

func parsePicoclawChannel(input string) (picoclawChannel, bool) {
	id := strings.ToLower(strings.TrimSpace(input))
	for _, ch := range picoclawChannels {
		if id == ch.ID {
			return ch, true
		}
	}
	return picoclawChannel{}, false
}

func renderPicoclawChannelPrompt() string {
	lines := []string{
		"Selected agent: **PicoClaw** (picoclaw)",
		"",
		"**Step 2 — Choose a channel for PicoClaw**",
	}
	for _, ch := range picoclawChannels {
		lines = append(lines, fmt.Sprintf("  • `%s` — %s", ch.ID, ch.Name))
	}
	lines = append(lines, "")
	lines = append(lines, "Reply with the channel ID (e.g. `/onboard telegram`).")
	return strings.Join(lines, "\n")
}

func renderPicoclawChannelTokenPrompt(ch picoclawChannel) string {
	return strings.Join([]string{
		fmt.Sprintf("✅ PicoClaw channel selected: **%s** (`%s`).", ch.Name, ch.ID),
		"",
		fmt.Sprintf("Paste %s.", ch.TokenLabel),
	}, "\n")
}

func extractPairCode(lines []string) string {
	for _, line := range lines {
		code := strings.TrimSpace(picoclawPairCodePattern.FindString(line))
		if code != "" {
			return code
		}
	}
	return ""
}

func extractPairedTelegramChatID(lines []string) string {
	for _, line := range lines {
		matches := picoclawPairedPattern.FindStringSubmatch(line)
		if len(matches) < 2 {
			continue
		}
		chatID := strings.TrimSpace(matches[1])
		if chatID != "" {
			return chatID
		}
	}
	return ""
}

func preparePicoclawManagedOnboard(sess *OnboardSession, actor string) (*picoclawManagedOnboardResult, error) {
	if sess == nil {
		return nil, fmt.Errorf("nil onboarding session")
	}
	if !isPicoclawAgent(sess.SelectedAgent) {
		return nil, fmt.Errorf("managed onboarding is only supported for picoclaw")
	}
	if strings.TrimSpace(sess.SelectedChannel) == "" {
		return nil, fmt.Errorf("picoclaw channel is required")
	}
	if strings.TrimSpace(sess.ChannelToken) == "" {
		return nil, fmt.Errorf("picoclaw channel token is required")
	}
	if strings.TrimSpace(sess.SelectedProvider) == "" {
		return nil, fmt.Errorf("picoclaw provider is required")
	}
	provider := GetLLMProvider(sess.SelectedProvider)
	if provider == nil {
		return nil, fmt.Errorf("unknown provider %q", sess.SelectedProvider)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	instanceID := strings.TrimSpace(sess.InstanceID)
	if instanceID == "" {
		instanceID = "picoclaw"
	}
	workspacePath := strings.TrimSpace(sess.WorkspacePath)
	if workspacePath == "" {
		workspacePath = filepath.Join(home, ".picoclaw", "instances", instanceID, "workspace")
	}
	configPath := filepath.Join(home, ".picoclaw", "config.json")
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
		return nil, fmt.Errorf("backup existing picoclaw config: %w", err)
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
	providerItem := map[string]interface{}{}
	token := pickProviderToken(provider, sess.EnvVars)
	if strings.EqualFold(provider.ID, "openai-codex") {
		modelItem["auth_method"] = "oauth"
		providerItem["auth_method"] = "oauth"
		if token != "" {
			modelItem["api_key"] = token
			providerItem["api_key"] = token
		}
		accountID := extractOpenAIAccountID(token)
		if err := savePicoclawAuthCredential(home, "openai", token, accountID); err != nil {
			return nil, fmt.Errorf("write picoclaw auth store: %w", err)
		}
	} else if token != "" {
		modelItem["api_key"] = token
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
		return nil, fmt.Errorf("marshal picoclaw config: %w", err)
	}
	if err := os.WriteFile(configPath, append(raw, '\n'), 0o600); err != nil {
		return nil, fmt.Errorf("write picoclaw config: %w", err)
	}

	record := map[string]interface{}{
		"instance_id":    instanceID,
		"agent_id":       "picoclaw",
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

	return &picoclawManagedOnboardResult{
		WorkspacePath: workspacePath,
		ConfigPath:    configPath,
		RecordPath:    recordPath,
	}, nil
}

func backupIfExists(path string) error {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	backupPath := fmt.Sprintf("%s.bak.%s", path, time.Now().UTC().Format("20060102T150405Z"))
	return os.Rename(path, backupPath)
}

func actorChatID(actor string) string {
	_, chatID, ok := strings.Cut(strings.TrimSpace(actor), ":")
	if !ok {
		return ""
	}
	trimmed := strings.TrimSpace(chatID)
	if trimmed == "" {
		return ""
	}
	for _, r := range trimmed {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return trimmed
}

func mapCarrierProviderToPicoclawProvider(providerID string) string {
	switch strings.ToLower(strings.TrimSpace(providerID)) {
	case "openai-codex":
		return "openai"
	default:
		return strings.TrimSpace(providerID)
	}
}

func pickProviderToken(provider *LLMProvider, envVars map[string]string) string {
	if provider == nil || envVars == nil {
		return ""
	}
	if strings.EqualFold(provider.ID, "openai-codex") {
		for _, key := range []string{"OPENAI_CODEX_TOKEN", "OPENAI_API_KEY", provider.EnvVar} {
			if token := strings.TrimSpace(envVars[key]); token != "" {
				return token
			}
		}
		return ""
	}
	if provider.EnvVar == "" {
		return ""
	}
	return strings.TrimSpace(envVars[provider.EnvVar])
}

type picoclawAuthCredential struct {
	AccessToken string `json:"access_token"`
	AccountID   string `json:"account_id,omitempty"`
	Provider    string `json:"provider"`
	AuthMethod  string `json:"auth_method"`
}

type picoclawAuthStore struct {
	Credentials map[string]*picoclawAuthCredential `json:"credentials"`
}

func savePicoclawAuthCredential(home, providerID, accessToken, accountID string) error {
	if strings.TrimSpace(accessToken) == "" {
		return fmt.Errorf("empty access token")
	}
	path := filepath.Join(home, ".picoclaw", "auth.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create auth dir: %w", err)
	}

	store := picoclawAuthStore{Credentials: map[string]*picoclawAuthCredential{}}
	if existing, err := os.ReadFile(path); err == nil && len(existing) > 0 {
		if err := json.Unmarshal(existing, &store); err != nil {
			return fmt.Errorf("parse existing auth store: %w", err)
		}
	}
	if store.Credentials == nil {
		store.Credentials = map[string]*picoclawAuthCredential{}
	}
	store.Credentials[strings.TrimSpace(providerID)] = &picoclawAuthCredential{
		AccessToken: strings.TrimSpace(accessToken),
		AccountID:   strings.TrimSpace(accountID),
		Provider:    strings.TrimSpace(providerID),
		AuthMethod:  "oauth",
	}

	raw, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal auth store: %w", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("write auth store: %w", err)
	}
	return nil
}

func extractOpenAIAccountID(token string) string {
	claims, err := parseJWTClaims(token)
	if err != nil {
		return ""
	}
	if accountID, ok := claims["chatgpt_account_id"].(string); ok && strings.TrimSpace(accountID) != "" {
		return strings.TrimSpace(accountID)
	}
	if accountID, ok := claims["https://api.openai.com/auth.chatgpt_account_id"].(string); ok && strings.TrimSpace(accountID) != "" {
		return strings.TrimSpace(accountID)
	}
	if authClaim, ok := claims["https://api.openai.com/auth"].(map[string]interface{}); ok {
		if accountID, ok := authClaim["chatgpt_account_id"].(string); ok && strings.TrimSpace(accountID) != "" {
			return strings.TrimSpace(accountID)
		}
	}
	return ""
}

func parseJWTClaims(token string) (map[string]interface{}, error) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("token is not a JWT")
	}
	payload := parts[1]
	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(payload)
		if err != nil {
			return nil, fmt.Errorf("decode jwt payload: %w", err)
		}
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return nil, fmt.Errorf("decode jwt claims: %w", err)
	}
	return claims, nil
}
