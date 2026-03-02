package gateway

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"carrier/shared/catalog"
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

func pickProviderToken(provider *LLMProvider, envVars map[string]string) string {
	if provider == nil || envVars == nil {
		return ""
	}
	if catalog.IsOpenAICodexProviderID(provider.ID) {
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
