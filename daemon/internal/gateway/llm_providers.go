package gateway

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
	Category     string   `json:"category"` // "builtin", "custom", "local"
	Description  string   `json:"description,omitempty"`
}

// llmProviderCatalog is the complete registry of supported LLM providers.
var llmProviderCatalog = []LLMProvider{
	// --- Built-in providers (API key auth) ---
	{
		ID:           "anthropic",
		Name:         "Anthropic",
		AuthMode:     AuthModeAPIKey,
		EnvVar:       "ANTHROPIC_API_KEY",
		ExampleModel: "anthropic/claude-opus-4-6",
		Category:     "builtin",
		Description:  "Anthropic Claude models (Opus, Sonnet, Haiku)",
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
		ID:           "opencode",
		Name:         "OpenCode",
		AuthMode:     AuthModeAPIKey,
		EnvVar:       "OPENCODE_API_KEY",
		ExampleModel: "opencode/claude-opus-4-6",
		Category:     "builtin",
		Description:  "OpenCode hosted models",
	},
	{
		ID:           "google",
		Name:         "Google (Gemini)",
		AuthMode:     AuthModeAPIKey,
		EnvVar:       "GEMINI_API_KEY",
		ExampleModel: "google/gemini-3-pro-preview",
		Category:     "builtin",
		Description:  "Google Gemini models via API key",
	},
	{
		ID:           "groq",
		Name:         "Groq",
		AuthMode:     AuthModeAPIKey,
		EnvVar:       "GROQ_API_KEY",
		ExampleModel: "groq/llama-4-scout",
		Category:     "builtin",
		Description:  "Groq ultra-fast inference",
	},
	{
		ID:           "deepseek",
		Name:         "DeepSeek",
		AuthMode:     AuthModeAPIKey,
		EnvVar:       "DEEPSEEK_API_KEY",
		ExampleModel: "deepseek/deepseek-chat",
		Category:     "builtin",
		Description:  "DeepSeek models",
	},
	{
		ID:           "mistral",
		Name:         "Mistral AI",
		AuthMode:     AuthModeAPIKey,
		EnvVar:       "MISTRAL_API_KEY",
		ExampleModel: "mistral/mistral-large-latest",
		Category:     "builtin",
		Description:  "Mistral AI models",
	},
	{
		ID:           "xai",
		Name:         "xAI (Grok)",
		AuthMode:     AuthModeAPIKey,
		EnvVar:       "XAI_API_KEY",
		ExampleModel: "xai/grok-3",
		Category:     "builtin",
		Description:  "xAI Grok models",
	},
	{
		ID:           "openrouter",
		Name:         "OpenRouter",
		AuthMode:     AuthModeAPIKey,
		EnvVar:       "OPENROUTER_API_KEY",
		ExampleModel: "openrouter/anthropic/claude-sonnet-4-5",
		Category:     "builtin",
		Description:  "OpenRouter — unified API for 100+ models",
	},
	{
		ID:           "cerebras",
		Name:         "Cerebras",
		AuthMode:     AuthModeAPIKey,
		EnvVar:       "CEREBRAS_API_KEY",
		ExampleModel: "",
		Category:     "builtin",
		Description:  "Cerebras fast inference",
	},
	{
		ID:           "zai",
		Name:         "ZAI",
		AuthMode:     AuthModeAPIKey,
		EnvVar:       "ZAI_API_KEY",
		ExampleModel: "zai/glm-4.7",
		Category:     "builtin",
		Description:  "ZAI (GLM) models",
	},
	{
		ID:           "vercel-ai-gateway",
		Name:         "Vercel AI Gateway",
		AuthMode:     AuthModeAPIKey,
		EnvVar:       "AI_GATEWAY_API_KEY",
		ExampleModel: "",
		Category:     "builtin",
		Description:  "Vercel AI Gateway unified access",
	},
	{
		ID:           "github-copilot",
		Name:         "GitHub Copilot",
		AuthMode:     AuthModeAPIKey,
		EnvVar:       "COPILOT_GITHUB_TOKEN",
		ExampleModel: "",
		Category:     "builtin",
		Description:  "GitHub Copilot LLM access",
	},
	{
		ID:           "huggingface",
		Name:         "Hugging Face",
		AuthMode:     AuthModeAPIKey,
		EnvVar:       "HUGGINGFACE_HUB_TOKEN",
		ExampleModel: "",
		Category:     "builtin",
		Description:  "Hugging Face Inference API",
	},
	{
		ID:           "moonshot",
		Name:         "Moonshot (Kimi)",
		AuthMode:     AuthModeAPIKey,
		EnvVar:       "MOONSHOT_API_KEY",
		ExampleModel: "moonshot/kimi-k2.5",
		Category:     "builtin",
		Description:  "Moonshot AI Kimi models",
	},
	{
		ID:           "kimi-coding",
		Name:         "Kimi Coding",
		AuthMode:     AuthModeAPIKey,
		EnvVar:       "KIMI_API_KEY",
		ExampleModel: "kimi-coding/k2p5",
		Category:     "builtin",
		Description:  "Kimi coding-optimised models",
	},
	{
		ID:           "synthetic",
		Name:         "Synthetic",
		AuthMode:     AuthModeAPIKey,
		EnvVar:       "SYNTHETIC_API_KEY",
		ExampleModel: "",
		Category:     "builtin",
		Description:  "Synthetic AI models",
	},
	{
		ID:           "minimax",
		Name:         "MiniMax",
		AuthMode:     AuthModeAPIKey,
		EnvVar:       "MINIMAX_API_KEY",
		ExampleModel: "",
		Category:     "builtin",
		Description:  "MiniMax models",
	},

	// --- OAuth / device code providers ---
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
		ID:           "qwen-portal",
		Name:         "Qwen Portal (OAuth)",
		AuthMode:     AuthModeOAuthDeviceCode,
		EnvVar:       "QWEN_PORTAL_TOKEN",
		ExampleModel: "qwen-portal/coder-model",
		Category:     "custom",
		Description:  "Alibaba Qwen via OAuth device code flow",
	},
	{
		ID:           "google-antigravity",
		Name:         "Google Antigravity (OAuth Plugin)",
		AuthMode:     AuthModeOAuthPlugin,
		EnvVar:       "",
		ExampleModel: "",
		Category:     "custom",
		Description:  "Google Antigravity access via OAuth plugin",
	},
	{
		ID:           "google-gemini-cli",
		Name:         "Google Gemini CLI (OAuth Plugin)",
		AuthMode:     AuthModeOAuthPlugin,
		EnvVar:       "",
		ExampleModel: "",
		Category:     "custom",
		Description:  "Google Gemini CLI via OAuth plugin",
	},

	// --- Other auth ---
	{
		ID:           "google-vertex",
		Name:         "Google Vertex AI (gcloud ADC)",
		AuthMode:     AuthModeGcloudADC,
		EnvVar:       "",
		ExampleModel: "",
		Category:     "custom",
		Description:  "Google Vertex AI via Application Default Credentials",
	},

	// --- Local providers (no auth) ---
	{
		ID:           "ollama",
		Name:         "Ollama (local)",
		AuthMode:     AuthModeNone,
		EnvVar:       "",
		ExampleModel: "ollama/llama3.3",
		Category:     "local",
		Description:  "Ollama — run LLMs locally",
	},
	{
		ID:           "vllm",
		Name:         "vLLM (local)",
		AuthMode:     AuthModeNone,
		EnvVar:       "VLLM_API_KEY",
		ExampleModel: "vllm/your-model-id",
		Category:     "local",
		Description:  "vLLM — high-throughput local inference (optional key)",
	},
}

// llmProviderIndex is a precomputed lookup by ID.
var llmProviderIndex map[string]*LLMProvider

func init() {
	llmProviderIndex = make(map[string]*LLMProvider, len(llmProviderCatalog))
	for i := range llmProviderCatalog {
		p := &llmProviderCatalog[i]
		llmProviderIndex[p.ID] = p
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
	p, ok := llmProviderIndex[id]
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
		result[p.Category] = append(result[p.Category], p)
	}
	return result
}
