package baseagent

import (
	"bytes"
	"carrier/shared/catalog"
	"carrier/shared/config"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultOpenAIBaseURL        = "https://api.openai.com/v1"
	defaultOpenAICodexBaseURL   = "https://chatgpt.com/backend-api"
	defaultBaseAgentLLMTimeout  = 45 * time.Second
	baseAgentLLMRetryAttempts   = 3
	baseAgentLLMRetryBackoff    = 200 * time.Millisecond
	baseAgentMaxModelRespBytes  = 1 << 20
	baseAgentMaxModelErrorBytes = 8 << 10
)

const openAICodexJWTClaimPath = "https://api.openai.com/auth"

type llmRequestDeps struct {
	marshalJSON func(any) ([]byte, error)
	readAll     func(io.Reader) ([]byte, error)
	doRequest   func(*http.Request) (*http.Response, error)
	sleep       func(context.Context, time.Duration) error
}

func normalizeLLMRequestDeps(deps llmRequestDeps) llmRequestDeps {
	if deps.marshalJSON == nil {
		deps.marshalJSON = json.Marshal
	}
	if deps.readAll == nil {
		deps.readAll = io.ReadAll
	}
	if deps.doRequest == nil {
		deps.doRequest = http.DefaultClient.Do
	}
	if deps.sleep == nil {
		deps.sleep = sleepWithContext
	}
	return deps
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isRetryableModelRequestError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func doModelRequestWithRetry(ctx context.Context, deps llmRequestDeps, requestTemplate *http.Request) (*http.Response, error) {
	backoff := baseAgentLLMRetryBackoff
	var lastErr error

	for attempt := 1; attempt <= baseAgentLLMRetryAttempts; attempt++ {
		req := requestTemplate.Clone(ctx)
		if requestTemplate.GetBody != nil {
			body, err := requestTemplate.GetBody()
			if err != nil {
				return nil, fmt.Errorf("clone model request body: %w", err)
			}
			req.Body = body
		}

		resp, err := deps.doRequest(req)
		if err == nil {
			return resp, nil
		}
		lastErr = err

		if attempt == baseAgentLLMRetryAttempts || !isRetryableModelRequestError(err) {
			break
		}
		if err := deps.sleep(ctx, backoff); err != nil {
			return nil, err
		}
		backoff *= 2
	}
	return nil, lastErr
}

const baseAgentSystemPrompt = "You are Carrier's built-in base agent. " +
	"Answer in the user's language. " +
	"You can help with agent management. " +
	"Onboarding and installation must be done in Carrier GUI, never via chat. Never ask users to paste secrets or tokens in chat. " +
	"When relevant, include exact slash commands (for example: /agents, /uninstall <agent>, /start <agent>, /stop <agent>, /status <agent>). " +
	"Keep responses concise and actionable."

type llmRuntimeConfig struct {
	ProviderID string
	ModelID    string
	Token      string
	BaseURL    string
}

func (r *Runtime) replyWithLLM(ctx context.Context, userMessage string) (string, error) {
	return requestLLMCompletion(ctx, baseAgentSystemPrompt, userMessage)
}

func requestLLMCompletion(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	return requestLLMCompletionWithProviderAndDeps(ctx, "", systemPrompt, userMessage, llmRequestDeps{})
}

// RequestCompletion is an exported wrapper for callers that need baseagent's
// provider/config aware text generation with the same retry and timeout policy.
func RequestCompletion(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	return requestLLMCompletion(ctx, systemPrompt, userMessage)
}

func requestLLMCompletionForProvider(ctx context.Context, providerID, systemPrompt, userMessage string) (string, error) {
	return requestLLMCompletionWithProviderAndDeps(ctx, providerID, systemPrompt, userMessage, llmRequestDeps{})
}

func requestLLMCompletionWithDeps(ctx context.Context, systemPrompt, userMessage string, deps llmRequestDeps) (string, error) {
	return requestLLMCompletionWithProviderAndDeps(ctx, "", systemPrompt, userMessage, deps)
}

func requestLLMCompletionWithProviderAndDeps(ctx context.Context, providerID, systemPrompt, userMessage string, deps llmRequestDeps) (string, error) {
	deps = normalizeLLMRequestDeps(deps)

	configs, err := resolveLLMRuntimeConfigsForProvider(providerID)
	if err != nil {
		return "", err
	}
	var lastErr error
	for _, cfg := range configs {
		text, err := requestLLMCompletionWithRuntimeConfigAndDeps(ctx, cfg, systemPrompt, userMessage, deps)
		if err == nil {
			return text, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", errors.New("default model is not configured")
}

func requestLLMCompletionWithRuntimeConfigAndDeps(ctx context.Context, cfg llmRuntimeConfig, systemPrompt, userMessage string, deps llmRequestDeps) (string, error) {
	modelID := normalizeModelForProvider(cfg.ProviderID, cfg.ModelID)

	path := "/chat/completions"
	reqBody := buildOpenAICompatibleChatRequest(modelID, systemPrompt, userMessage)
	parseResponse := parseOpenAICompatibleChatResponse
	if isOpenAICodexProvider(cfg.ProviderID) {
		path = "/codex/responses"
		reqBody = buildOpenAICodexResponsesRequest(modelID, systemPrompt, userMessage)
		parseResponse = parseOpenAICodexResponses
	}

	raw, err := deps.marshalJSON(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal model request: %w", err)
	}

	llmCtx, cancel := context.WithTimeout(ctx, defaultBaseAgentLLMTimeout)
	defer cancel()
	url := strings.TrimRight(cfg.BaseURL, "/") + path
	req, err := http.NewRequestWithContext(llmCtx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("build model request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	if isOpenAICodexProvider(cfg.ProviderID) {
		req.Header.Set("OpenAI-Beta", "responses=experimental")
		req.Header.Set("originator", "carrier")
		req.Header.Set("Accept", "text/event-stream")
		if accountID := extractOpenAICodexAccountID(cfg.Token); accountID != "" {
			req.Header.Set("chatgpt-account-id", accountID)
		}
	}

	resp, err := doModelRequestWithRetry(llmCtx, deps, req)
	if err != nil {
		return "", fmt.Errorf("model request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, readErr := deps.readAll(io.LimitReader(resp.Body, baseAgentMaxModelErrorBytes))
		if parsedErr := parseModelError(resp.StatusCode, body); parsedErr != nil {
			if readErr != nil {
				return "", fmt.Errorf("%w (unable to read full error response body: %v)", parsedErr, readErr)
			}
			return "", parsedErr
		}

		msg := strings.TrimSpace(string(body))
		if msg == "" {
			if readErr != nil {
				return "", fmt.Errorf("model request failed with status %d (unable to read error response body: %v)", resp.StatusCode, readErr)
			}
			return "", fmt.Errorf("model request failed with status %d", resp.StatusCode)
		}
		if readErr != nil {
			return "", fmt.Errorf("model request failed with status %d: %s (unable to read full error response body: %v)", resp.StatusCode, msg, readErr)
		}
		return "", fmt.Errorf("model request failed with status %d: %s", resp.StatusCode, msg)
	}

	body, err := deps.readAll(io.LimitReader(resp.Body, baseAgentMaxModelRespBytes))
	if err != nil {
		return "", fmt.Errorf("read model response: %w", err)
	}
	text, err := parseResponse(body)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(text) == "" {
		return "", errors.New("model returned empty content")
	}
	return text, nil
}

func resolveLLMRuntimeConfig() (*llmRuntimeConfig, error) {
	return resolveLLMRuntimeConfigForProvider("")
}

func resolveLLMRuntimeConfigForProvider(providerID string) (*llmRuntimeConfig, error) {
	configs, err := resolveLLMRuntimeConfigsForProvider(providerID)
	if err != nil {
		return nil, err
	}
	if len(configs) == 0 {
		return nil, errors.New("default model is not configured")
	}
	return &configs[0], nil
}

func resolveLLMRuntimeConfigsForProvider(providerID string) ([]llmRuntimeConfig, error) {
	var (
		cfg *config.CarrierDefaultModel
		err error
	)
	if strings.TrimSpace(providerID) == "" {
		cfg, err = config.LoadCarrierDefaultModel()
	} else {
		cfg, err = config.LoadCarrierModelForProvider(providerID)
	}
	if err != nil || cfg == nil {
		return nil, errors.New("default model is not configured")
	}
	candidates, err := resolveLLMRuntimeModelCandidates(*cfg)
	if err != nil {
		return nil, err
	}
	configs := make([]llmRuntimeConfig, 0, len(candidates))
	for _, candidate := range candidates {
		runtimeCfg, err := buildLLMRuntimeConfig(candidate)
		if err != nil {
			return nil, err
		}
		configs = append(configs, runtimeCfg)
	}
	if len(configs) == 0 {
		return nil, errors.New("default model is not configured")
	}
	return configs, nil
}

func resolveLLMRuntimeModelCandidates(primary config.CarrierDefaultModel) ([]config.CarrierDefaultModel, error) {
	alias := strings.TrimSpace(primary.ModelAlias)
	if alias == "" {
		return []config.CarrierDefaultModel{primary}, nil
	}
	profiles, err := config.LoadCarrierModelProfilesForAlias(primary.ProviderID, alias)
	if err != nil || len(profiles) == 0 {
		return []config.CarrierDefaultModel{primary}, nil
	}
	return profiles, nil
}

func buildLLMRuntimeConfig(cfg config.CarrierDefaultModel) (llmRuntimeConfig, error) {
	providerID := catalog.NormalizeProviderID(cfg.ProviderID)
	modelID := strings.TrimSpace(cfg.ModelID)
	envVar := strings.TrimSpace(cfg.EnvVar)

	if providerID == "" || modelID == "" {
		return llmRuntimeConfig{}, errors.New("default model is not configured")
	}
	if !catalog.IsSupportedProvider(providerID) {
		return llmRuntimeConfig{}, fmt.Errorf("unsupported provider %s", providerID)
	}
	if envVar == "" {
		envVar = inferProviderEnvVar(providerID)
	}
	if envVar == "" {
		return llmRuntimeConfig{}, fmt.Errorf("no env var mapping for provider %s", providerID)
	}

	token := strings.TrimSpace(os.Getenv(envVar))
	if token == "" {
		return llmRuntimeConfig{}, fmt.Errorf("provider credential is missing (%s)", envVar)
	}

	baseURL := strings.TrimSpace(cfg.BaseURL)
	providerSpec := catalog.GetProvider(providerID)
	switch {
	case isOpenAICodexProvider(providerID):
		if baseURL == "" {
			baseURL = strings.TrimSpace(os.Getenv("CARRIER_OPENAI_CODEX_BASE_URL"))
		}
		if baseURL == "" {
			baseURL = strings.TrimSpace(os.Getenv("CARRIER_OPENAI_BASE_URL"))
		}
		if baseURL == "" {
			baseURL = defaultOpenAICodexBaseURL
		}
	default:
		if baseURL == "" && providerSpec != nil {
			if baseEnv := strings.TrimSpace(providerSpec.BaseURLEnv); baseEnv != "" {
				baseURL = strings.TrimSpace(os.Getenv(baseEnv))
			}
		}
		if override := strings.TrimSpace(os.Getenv("CARRIER_OPENAI_BASE_URL")); baseURL == "" && override != "" {
			baseURL = override
		}
		if baseURL == "" {
			baseURL = catalog.ResolveProviderBaseURL(providerID, providerID, "")
		}
		if baseURL == "" {
			baseURL = defaultOpenAIBaseURL
		}
	}

	return llmRuntimeConfig{
		ProviderID: providerID,
		ModelID:    modelID,
		Token:      token,
		BaseURL:    baseURL,
	}, nil
}

func inferProviderEnvVar(providerID string) string {
	if provider := catalog.GetProvider(providerID); provider != nil {
		return strings.TrimSpace(provider.EnvVar)
	}
	return ""
}

func normalizeModelForProvider(providerID, modelID string) string {
	return config.NormalizeModelForProvider(providerID, modelID)
}

func extractModelContent(raw interface{}) string {
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	case []interface{}:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if text := extractModelContent(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]interface{}:
		parts := make([]string, 0, 3)
		if text, ok := v["output_text"].(string); ok && strings.TrimSpace(text) != "" {
			parts = append(parts, strings.TrimSpace(text))
		}
		if text, ok := v["text"].(string); ok && strings.TrimSpace(text) != "" {
			parts = append(parts, strings.TrimSpace(text))
		}
		for _, key := range []string{"content", "message", "output"} {
			if nested, ok := v[key]; ok {
				if text := extractModelContent(nested); text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func isOpenAICodexProvider(providerID string) bool {
	return catalog.IsOpenAICodexProviderID(providerID)
}

func extractOpenAICodexAccountID(token string) string {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 {
		return ""
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return ""
		}
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}

	authClaim, ok := claims[openAICodexJWTClaimPath].(map[string]interface{})
	if !ok {
		return ""
	}
	accountID, _ := authClaim["chatgpt_account_id"].(string)
	return strings.TrimSpace(accountID)
}
