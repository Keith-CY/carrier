package gateway

import "strings"

// AuthMode describes how an LLM provider authenticates.
type AuthMode string

const (
	AuthModeAPIKey          AuthMode = "api_key"
	AuthModeOAuthDeviceCode AuthMode = "oauth_device_code"
	AuthModeOAuthPlugin     AuthMode = "oauth_plugin"
	AuthModeGcloudADC       AuthMode = "gcloud_adc"
	AuthModeNone            AuthMode = "none"
)

// LLMProvider describes an LLM provider in the catalog.
type LLMProvider struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	AuthMode     AuthMode `json:"auth_mode"`
	EnvVar       string   `json:"env_var,omitempty"`
	ExampleModel string   `json:"example_model,omitempty"`
	Category     string   `json:"category"` // "builtin", "custom" (legacy "local" bucket kept for compatibility)
	Description  string   `json:"description,omitempty"`
}

// llmProviderCatalog is the complete registry of supported LLM providers.
var llmProviderCatalog = []LLMProvider{
	{
		ID:           "anthropic",
		Name:         "Anthropic",
		AuthMode:     AuthModeAPIKey,
		EnvVar:       "ANTHROPIC_API_KEY",
		ExampleModel: "anthropic/claude-opus-4-6",
		Category:     "builtin",
		Description:  "Anthropic Claude models",
	},
	{
		ID:           "openai",
		Name:         "OpenAI",
		AuthMode:     AuthModeAPIKey,
		EnvVar:       "OPENAI_API_KEY",
		ExampleModel: "openai/gpt-5.1-codex",
		Category:     "builtin",
		Description:  "OpenAI GPT models",
	},
	{
		ID:           "openai-codex",
		Name:         "OpenAI Codex (OAuth)",
		AuthMode:     AuthModeOAuthDeviceCode,
		EnvVar:       "OPENAI_CODEX_TOKEN",
		ExampleModel: "openai-codex/gpt-5.3-codex",
		Category:     "custom",
		Description:  "OpenAI Codex via OAuth device code flow",
	},
	{
		ID:           "openai-compatible",
		Name:         "OpenAI-Compatible (v1)",
		AuthMode:     AuthModeNone,
		EnvVar:       "VLLM_API_KEY",
		ExampleModel: "openai-compatible/your-model-id",
		Category:     "custom",
		Description:  "OpenAI v1-compatible endpoint",
	},
}

// llmProviderIndex is a precomputed lookup by ID.
var llmProviderIndex map[string]*LLMProvider

// llmProviderAliases keeps backward-compatible IDs that map to canonical IDs.
var llmProviderAliases = map[string]string{
	"vllm":      "openai-compatible",
	"openai-v1": "openai-compatible",
}

func init() {
	llmProviderIndex = make(map[string]*LLMProvider, len(llmProviderCatalog))
	for i := range llmProviderCatalog {
		p := &llmProviderCatalog[i]
		llmProviderIndex[p.ID] = p
	}
	for alias, canonicalID := range llmProviderAliases {
		if canonical, ok := llmProviderIndex[canonicalID]; ok {
			llmProviderIndex[alias] = canonical
		}
	}
}

// ListLLMProviders returns a copy of the full provider catalog.
func ListLLMProviders() []LLMProvider {
	out := make([]LLMProvider, len(llmProviderCatalog))
	copy(out, llmProviderCatalog)
	return out
}

// GetLLMProvider returns the provider with the given ID, or nil if not found.
func GetLLMProvider(id string) *LLMProvider {
	lookupID := strings.ToLower(strings.TrimSpace(id))
	p, ok := llmProviderIndex[lookupID]
	if !ok {
		return nil
	}
	cp := *p
	return &cp
}

// LLMProvidersByCategory returns providers grouped by category.
// Categories are returned in order: builtin, custom, local.
func LLMProvidersByCategory() map[string][]LLMProvider {
	result := map[string][]LLMProvider{
		"builtin": {},
		"custom":  {},
		"local":   {},
	}
	for _, p := range llmProviderCatalog {
		if _, ok := result[p.Category]; !ok {
			result[p.Category] = []LLMProvider{}
		}
		result[p.Category] = append(result[p.Category], p)
	}
	return result
}
