package baseagent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultOpenAIBaseURL        = "https://api.openai.com/v1"
	defaultOpenAICodexBaseURL   = "https://chatgpt.com/backend-api"
	defaultOpenRouterBaseURL    = "https://openrouter.ai/api/v1"
	defaultBaseAgentLLMTimeout  = 45 * time.Second
	baseAgentMaxModelRespBytes  = 1 << 20
	baseAgentMaxModelErrorBytes = 8 << 10
)

const openAICodexJWTClaimPath = "https://api.openai.com/auth"

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
	cfg, err := resolveLLMRuntimeConfig()
	if err != nil {
		return "", err
	}
	modelID := normalizeModelForProvider(cfg.ProviderID, cfg.ModelID)
	if strings.TrimSpace(modelID) == "" {
		return "", errors.New("empty model id")
	}

	path := "/chat/completions"
	reqBody := buildOpenAICompatibleChatRequest(modelID, systemPrompt, userMessage)
	parseResponse := parseOpenAICompatibleChatResponse
	if isOpenAICodexProvider(cfg.ProviderID) {
		path = "/codex/responses"
		reqBody = buildOpenAICodexResponsesRequest(modelID, systemPrompt, userMessage)
		parseResponse = parseOpenAICodexResponses
	}

	raw, err := json.Marshal(reqBody)
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
	if strings.EqualFold(cfg.ProviderID, "openrouter") {
		req.Header.Set("HTTP-Referer", "https://carrier.local")
		req.Header.Set("X-Title", "Carrier Base Agent")
	}
	if isOpenAICodexProvider(cfg.ProviderID) {
		req.Header.Set("OpenAI-Beta", "responses=experimental")
		req.Header.Set("originator", "carrier")
		req.Header.Set("Accept", "text/event-stream")
		if accountID := extractOpenAICodexAccountID(cfg.Token); accountID != "" {
			req.Header.Set("chatgpt-account-id", accountID)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("model request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, baseAgentMaxModelErrorBytes))
		if parsedErr := parseModelError(resp.StatusCode, body); parsedErr != nil {
			return "", parsedErr
		}

		msg := strings.TrimSpace(string(body))
		if msg == "" {
			return "", fmt.Errorf("model request failed with status %d", resp.StatusCode)
		}
		return "", fmt.Errorf("model request failed with status %d: %s", resp.StatusCode, msg)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, baseAgentMaxModelRespBytes))
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

func buildOpenAICompatibleChatRequest(modelID, systemPrompt, userMessage string) map[string]interface{} {
	return map[string]interface{}{
		"model": modelID,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userMessage},
		},
		"temperature": 0.2,
	}
}

func buildOpenAICodexResponsesRequest(modelID, systemPrompt, userMessage string) map[string]interface{} {
	return map[string]interface{}{
		"model":        modelID,
		"store":        false,
		"stream":       true,
		"instructions": systemPrompt,
		"input": []map[string]interface{}{
			{
				"role": "user",
				"content": []map[string]string{
					{
						"type": "input_text",
						"text": userMessage,
					},
				},
			},
		},
		"text": map[string]string{
			"verbosity": "medium",
		},
	}
}

func parseOpenAICompatibleChatResponse(body []byte) (string, error) {
	var parsed struct {
		Choices []struct {
			Message struct {
				Content interface{} `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("decode model response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("model response has no choices")
	}
	return extractModelContent(parsed.Choices[0].Message.Content), nil
}

func parseOpenAICodexResponses(body []byte) (string, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return "", errors.New("model response has no output")
	}

	if looksLikeSSEStream(trimmed) {
		if text, err := parseOpenAICodexSSE(trimmed); err == nil && strings.TrimSpace(text) != "" {
			return text, nil
		}
	}

	var parsed struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Content interface{} `json:"content"`
			Text    string      `json:"text"`
		} `json:"output"`
	}
	if err := json.Unmarshal(trimmed, &parsed); err != nil {
		// Fallback to stream parser for gateways that always return SSE.
		if text, sseErr := parseOpenAICodexSSE(trimmed); sseErr == nil && strings.TrimSpace(text) != "" {
			return text, nil
		}
		return "", fmt.Errorf("decode codex response: %w", err)
	}
	if text := strings.TrimSpace(parsed.OutputText); text != "" {
		return text, nil
	}

	parts := make([]string, 0, len(parsed.Output))
	for _, item := range parsed.Output {
		if text := strings.TrimSpace(item.Text); text != "" {
			parts = append(parts, text)
		}
		if text := extractModelContent(item.Content); text != "" {
			parts = append(parts, text)
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, "\n"), nil
	}

	// Allow OpenAI-compatible proxies that still answer in chat/completions schema.
	if text, err := parseOpenAICompatibleChatResponse(trimmed); err == nil {
		return text, nil
	}
	return "", errors.New("model response has no output")
}

func looksLikeSSEStream(body []byte) bool {
	return bytes.HasPrefix(body, []byte("data:")) ||
		bytes.Contains(body, []byte("\nevent:")) ||
		bytes.Contains(body, []byte("\ndata:"))
}

func parseOpenAICodexSSE(body []byte) (string, error) {
	raw := strings.ReplaceAll(string(body), "\r\n", "\n")
	events := strings.Split(raw, "\n\n")

	deltaParts := make([]string, 0, 8)
	completedText := ""

	for _, block := range events {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}

		lines := strings.Split(block, "\n")
		dataLines := make([]string, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "data:") {
				dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		if len(dataLines) == 0 {
			continue
		}

		payload := strings.TrimSpace(strings.Join(dataLines, "\n"))
		if payload == "" || payload == "[DONE]" {
			continue
		}

		var evt map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &evt); err != nil {
			continue
		}

		if t, _ := evt["type"].(string); t == "error" {
			if msg := strings.TrimSpace(extractModelContent(evt)); msg != "" {
				return "", fmt.Errorf("codex stream error: %s", msg)
			}
			return "", errors.New("codex stream error")
		}

		if d, ok := evt["delta"].(string); ok && strings.TrimSpace(d) != "" {
			deltaParts = append(deltaParts, d)
		}
		if text, ok := evt["output_text"].(string); ok && strings.TrimSpace(text) != "" {
			completedText = strings.TrimSpace(text)
		}
		if text, ok := evt["text"].(string); ok && strings.TrimSpace(text) != "" && completedText == "" {
			completedText = strings.TrimSpace(text)
		}
		if response, ok := evt["response"].(map[string]interface{}); ok {
			if text := strings.TrimSpace(extractModelContent(response)); text != "" {
				completedText = text
			}
		}
		if output, ok := evt["output"]; ok && completedText == "" {
			if text := strings.TrimSpace(extractModelContent(output)); text != "" {
				completedText = text
			}
		}
	}

	if len(deltaParts) > 0 {
		return strings.TrimSpace(strings.Join(deltaParts, "")), nil
	}
	if completedText != "" {
		return completedText, nil
	}
	return "", errors.New("model response has no output")
}

func parseModelError(statusCode int, body []byte) error {
	type apiErrBody struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
		Message string `json:"message"`
		Code    string `json:"code"`
	}
	var parsedErr apiErrBody
	if err := json.Unmarshal(body, &parsedErr); err != nil {
		return nil
	}

	detail := strings.TrimSpace(parsedErr.Error.Message)
	if detail == "" {
		detail = strings.TrimSpace(parsedErr.Message)
	}
	if detail == "" {
		return nil
	}

	code := strings.TrimSpace(parsedErr.Error.Code)
	if code == "" {
		code = strings.TrimSpace(parsedErr.Code)
	}
	if code != "" {
		return fmt.Errorf("model request failed with status %d: %s (%s)", statusCode, detail, code)
	}
	return fmt.Errorf("model request failed with status %d: %s", statusCode, detail)
}

func resolveLLMRuntimeConfig() (*llmRuntimeConfig, error) {
	cfg, err := readDefaultModelFromConfigFile()
	if err != nil || cfg == nil {
		return nil, errors.New("default model is not configured")
	}
	providerID := strings.TrimSpace(cfg.ProviderID)
	modelID := strings.TrimSpace(cfg.ModelID)
	envVar := strings.TrimSpace(cfg.EnvVar)

	if providerID == "" || modelID == "" {
		return nil, errors.New("default model is not configured")
	}
	if envVar == "" {
		envVar = inferProviderEnvVar(providerID)
	}
	if envVar == "" {
		return nil, fmt.Errorf("no env var mapping for provider %s", providerID)
	}

	token := strings.TrimSpace(os.Getenv(envVar))
	if token == "" {
		return nil, fmt.Errorf("provider credential is missing (%s)", envVar)
	}

	baseURL := defaultOpenAIBaseURL
	switch {
	case strings.EqualFold(providerID, "openrouter"):
		baseURL = strings.TrimSpace(os.Getenv("CARRIER_OPENROUTER_BASE_URL"))
		if baseURL == "" {
			baseURL = defaultOpenRouterBaseURL
		}
	case isOpenAICodexProvider(providerID):
		baseURL = strings.TrimSpace(os.Getenv("CARRIER_OPENAI_CODEX_BASE_URL"))
		if baseURL == "" {
			baseURL = strings.TrimSpace(os.Getenv("CARRIER_OPENAI_BASE_URL"))
		}
		if baseURL == "" {
			baseURL = defaultOpenAICodexBaseURL
		}
	default:
		override := strings.TrimSpace(os.Getenv("CARRIER_OPENAI_BASE_URL"))
		if override != "" {
			baseURL = override
		}
	}

	return &llmRuntimeConfig{
		ProviderID: providerID,
		ModelID:    modelID,
		Token:      token,
		BaseURL:    baseURL,
	}, nil
}

type defaultModelConfig struct {
	ProviderID string
	ModelID    string
	EnvVar     string
}

func readDefaultModelFromConfigFile() (*defaultModelConfig, error) {
	path, err := resolveConfigV2Path()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg struct {
		DefaultModel string `json:"default_model"`
		ModelList    []struct {
			ModelName  string `json:"model_name"`
			Model      string `json:"model"`
			ProviderID string `json:"provider_id"`
			EnvVar     string `json:"env_var"`
		} `json:"model_list"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	if len(cfg.ModelList) == 0 {
		return nil, errors.New("empty model_list")
	}
	defaultName := strings.TrimSpace(cfg.DefaultModel)
	if defaultName != "" {
		for _, m := range cfg.ModelList {
			if strings.EqualFold(strings.TrimSpace(m.ModelName), defaultName) {
				return &defaultModelConfig{
					ProviderID: strings.TrimSpace(m.ProviderID),
					ModelID:    strings.TrimSpace(m.Model),
					EnvVar:     strings.TrimSpace(m.EnvVar),
				}, nil
			}
		}
	}
	m := cfg.ModelList[0]
	return &defaultModelConfig{
		ProviderID: strings.TrimSpace(m.ProviderID),
		ModelID:    strings.TrimSpace(m.Model),
		EnvVar:     strings.TrimSpace(m.EnvVar),
	}, nil
}

func resolveConfigV2Path() (string, error) {
	if path := strings.TrimSpace(os.Getenv("CARRIER_CONFIG")); path != "" {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".carrier", "config.v2.json"), nil
}

func inferProviderEnvVar(providerID string) string {
	switch strings.ToLower(strings.TrimSpace(providerID)) {
	case "openai-codex":
		return "OPENAI_CODEX_TOKEN"
	case "openai":
		return "OPENAI_API_KEY"
	case "openrouter":
		return "OPENROUTER_API_KEY"
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	case "google":
		return "GEMINI_API_KEY"
	case "groq":
		return "GROQ_API_KEY"
	case "deepseek":
		return "DEEPSEEK_API_KEY"
	case "mistral":
		return "MISTRAL_API_KEY"
	case "opencode":
		return "OPENCODE_API_KEY"
	case "zai":
		return "ZAI_API_KEY"
	case "cerebras":
		return "CEREBRAS_API_KEY"
	default:
		return ""
	}
}

func normalizeModelForProvider(providerID, modelID string) string {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(providerID), "openrouter") {
		return modelID
	}
	if slash := strings.Index(modelID, "/"); slash > 0 && slash < len(modelID)-1 {
		return strings.TrimSpace(modelID[slash+1:])
	}
	return modelID
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
	return strings.EqualFold(strings.TrimSpace(providerID), "openai-codex")
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
