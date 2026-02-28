package catalog

import "strings"

// AuthMode describes how a provider authenticates.
type AuthMode string

const (
	AuthModeAPIKey          AuthMode = "api_key"
	AuthModeOAuthDeviceCode AuthMode = "oauth_device_code"
	AuthModeOAuthPlugin     AuthMode = "oauth_plugin"
	AuthModeGcloudADC       AuthMode = "gcloud_adc"
	AuthModeNone            AuthMode = "none"
)

// ProviderSpec is the canonical provider catalog entry shared across Carrier modules.
type ProviderSpec struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	AuthMode     AuthMode `json:"auth_mode"`
	EnvVar       string   `json:"env_var,omitempty"`
	ExampleModel string   `json:"example_model,omitempty"`
	Category     string   `json:"category"`
	Description  string   `json:"description,omitempty"`
	Setup        string   `json:"-"`
}

// ChannelSpec is the canonical channel catalog entry shared across Carrier modules.
type ChannelSpec struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Setup        string `json:"setup,omitempty"`
	TokenEnv     string `json:"token_env,omitempty"`
	RequireToken bool   `json:"require_token,omitempty"`
	SecretEnv    string `json:"secret_env,omitempty"`
}

var providerCatalog = []ProviderSpec{
	{
		ID:           "anthropic",
		Name:         "Anthropic",
		AuthMode:     AuthModeAPIKey,
		EnvVar:       "ANTHROPIC_API_KEY",
		ExampleModel: "anthropic/claude-opus-4-6",
		Category:     "builtin",
		Description:  "Anthropic Claude models",
		Setup:        "Claude direct API key",
	},
	{
		ID:           "openai",
		Name:         "OpenAI",
		AuthMode:     AuthModeAPIKey,
		EnvVar:       "OPENAI_API_KEY",
		ExampleModel: "openai/gpt-5.2",
		Category:     "builtin",
		Description:  "OpenAI GPT models",
		Setup:        "GPT direct API key",
	},
	{
		ID:           "openai-codex",
		Name:         "OpenAI Codex (OAuth)",
		AuthMode:     AuthModeOAuthDeviceCode,
		EnvVar:       "OPENAI_CODEX_TOKEN",
		ExampleModel: "openai-codex/gpt-5.3-codex",
		Category:     "custom",
		Description:  "OpenAI Codex via OAuth device code flow",
		Setup:        "OAuth device-code login",
	},
	{
		ID:           "openai-compatible",
		Name:         "OpenAI-Compatible (v1)",
		AuthMode:     AuthModeNone,
		EnvVar:       "OPENAI_COMPATIBLE_API_KEY",
		ExampleModel: "openai-compatible/your-model-id",
		Category:     "generic",
		Description:  "OpenAI v1-compatible endpoint",
		Setup:        "OpenAI v1-compatible endpoint",
	},
}

var channelCatalog = []ChannelSpec{
	{
		ID:           "telegram",
		Name:         "Telegram",
		Setup:        "Easy (bot token)",
		TokenEnv:     "CARRIER_TELEGRAM_BOT_TOKEN",
		RequireToken: true,
		SecretEnv:    "CARRIER_TELEGRAM_WEBHOOK_SECRET",
	},
	{
		ID:        "discord",
		Name:      "Discord",
		Setup:     "Easy (bot token + intents)",
		TokenEnv:  "CARRIER_DISCORD_BOT_TOKEN",
		SecretEnv: "CARRIER_DISCORD_PUBLIC_KEY",
	},
	{
		ID:        "feishu",
		Name:      "Feishu",
		Setup:     "Medium (app credentials + webhook)",
		TokenEnv:  "CARRIER_FEISHU_APP_TOKEN",
		SecretEnv: "CARRIER_FEISHU_VERIFICATION_TOKEN",
	},
}

var providerIndex map[string]ProviderSpec
var channelIndex map[string]ChannelSpec

func init() {
	providerIndex = make(map[string]ProviderSpec, len(providerCatalog))
	for _, provider := range providerCatalog {
		providerIndex[provider.ID] = provider
	}

	channelIndex = make(map[string]ChannelSpec, len(channelCatalog))
	for _, channel := range channelCatalog {
		channelIndex[channel.ID] = channel
	}
}

// ListProviders returns a copy of canonical provider entries.
func ListProviders() []ProviderSpec {
	out := make([]ProviderSpec, len(providerCatalog))
	copy(out, providerCatalog)
	return out
}

// GetProvider returns a provider by canonical ID.
func GetProvider(id string) *ProviderSpec {
	provider, ok := providerIndex[strings.ToLower(strings.TrimSpace(id))]
	if !ok {
		return nil
	}
	cp := provider
	return &cp
}

// IsSupportedProvider reports whether the provider ID is canonical and supported.
func IsSupportedProvider(id string) bool {
	_, ok := providerIndex[strings.ToLower(strings.TrimSpace(id))]
	return ok
}

// SupportedProviderIDs returns canonical provider IDs in catalog order.
func SupportedProviderIDs() []string {
	ids := make([]string, 0, len(providerCatalog))
	for _, provider := range providerCatalog {
		ids = append(ids, provider.ID)
	}
	return ids
}

// ProvidersByCategory returns providers grouped by category.
func ProvidersByCategory() map[string][]ProviderSpec {
	result := map[string][]ProviderSpec{
		"builtin": {},
		"custom":  {},
		"generic": {},
	}
	for _, provider := range providerCatalog {
		result[provider.Category] = append(result[provider.Category], provider)
	}
	return result
}

// ListChannels returns a copy of canonical channel entries.
func ListChannels() []ChannelSpec {
	out := make([]ChannelSpec, len(channelCatalog))
	copy(out, channelCatalog)
	return out
}

// GetChannel returns a channel by canonical ID.
func GetChannel(id string) *ChannelSpec {
	channel, ok := channelIndex[strings.ToLower(strings.TrimSpace(id))]
	if !ok {
		return nil
	}
	cp := channel
	return &cp
}

// IsSupportedChannel reports whether the channel ID is canonical and supported.
func IsSupportedChannel(id string) bool {
	_, ok := channelIndex[strings.ToLower(strings.TrimSpace(id))]
	return ok
}

// MapToManagedProvider maps canonical provider IDs to managed-agent provider keys.
// Managed agents currently support a smaller provider key surface and collapse
// compatible providers onto "openai".
func MapToManagedProvider(providerID string) string {
	normalized := strings.ToLower(strings.TrimSpace(providerID))
	switch normalized {
	case "openai-codex", "openai-compatible":
		return "openai"
	default:
		return normalized
	}
}
