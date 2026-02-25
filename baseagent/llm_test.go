package baseagent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDefaultModelConfig(t *testing.T, providerID, modelID, envVar string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.v2.json")
	t.Setenv("CARRIER_CONFIG", path)
	modelName := providerID + "-default"
	payload := map[string]interface{}{
		"config_version": 2,
		"default_model":  modelName,
		"model_list": []map[string]string{
			{
				"model_name":  modelName,
				"model":       modelID,
				"provider_id": providerID,
				"env_var":     envVar,
			},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal config payload: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write config payload: %v", err)
	}
}

func TestNormalizeModelForProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		model    string
		want     string
	}{
		{
			name:     "openai strips provider prefix",
			provider: "openai-codex",
			model:    "openai-codex/gpt-5.3-codex",
			want:     "gpt-5.3-codex",
		},
		{
			name:     "openrouter keeps full model id",
			provider: "openrouter",
			model:    "openai/gpt-5.3",
			want:     "openai/gpt-5.3",
		},
		{
			name:     "already plain model id",
			provider: "openai",
			model:    "gpt-5.3",
			want:     "gpt-5.3",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeModelForProvider(tc.provider, tc.model)
			if got != tc.want {
				t.Fatalf("normalizeModelForProvider(%q, %q) = %q, want %q", tc.provider, tc.model, got, tc.want)
			}
		})
	}
}

func TestExtractModelContent(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		got := extractModelContent("hello")
		if got != "hello" {
			t.Fatalf("extractModelContent(string) = %q, want %q", got, "hello")
		}
	})

	t.Run("array with text maps", func(t *testing.T) {
		raw := []interface{}{
			map[string]interface{}{"type": "output_text", "text": "hello"},
			map[string]interface{}{"type": "output_text", "text": "world"},
		}
		got := extractModelContent(raw)
		if got != "hello\nworld" {
			t.Fatalf("extractModelContent(array) = %q, want %q", got, "hello\nworld")
		}
	})

	t.Run("nested map content", func(t *testing.T) {
		raw := map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{"text": "nested"},
			},
		}
		got := extractModelContent(raw)
		if got != "nested" {
			t.Fatalf("extractModelContent(nested map) = %q, want %q", got, "nested")
		}
	})
}

func TestResolveLLMRuntimeConfig_OpenAICodexBaseURL(t *testing.T) {
	writeDefaultModelConfig(t, "openai-codex", "openai-codex/gpt-5.3-codex", "OPENAI_CODEX_TOKEN")
	t.Setenv("OPENAI_CODEX_TOKEN", "test-token")
	t.Setenv("CARRIER_OPENAI_CODEX_BASE_URL", "")
	t.Setenv("CARRIER_OPENAI_BASE_URL", "")

	cfg, err := resolveLLMRuntimeConfig()
	if err != nil {
		t.Fatalf("resolveLLMRuntimeConfig() error = %v", err)
	}
	if cfg.BaseURL != defaultOpenAICodexBaseURL {
		t.Fatalf("BaseURL = %q, want %q", cfg.BaseURL, defaultOpenAICodexBaseURL)
	}
}

func TestResolveLLMRuntimeConfig_OpenAICodexBaseURLOverride(t *testing.T) {
	writeDefaultModelConfig(t, "openai-codex", "openai-codex/gpt-5.3-codex", "OPENAI_CODEX_TOKEN")
	t.Setenv("OPENAI_CODEX_TOKEN", "test-token")
	t.Setenv("CARRIER_OPENAI_CODEX_BASE_URL", "http://127.0.0.1:3001/backend-api")

	cfg, err := resolveLLMRuntimeConfig()
	if err != nil {
		t.Fatalf("resolveLLMRuntimeConfig() error = %v", err)
	}
	if cfg.BaseURL != "http://127.0.0.1:3001/backend-api" {
		t.Fatalf("BaseURL = %q, want custom codex base url", cfg.BaseURL)
	}
}

func TestReplyWithLLM_OpenAICodexUsesResponsesEndpoint(t *testing.T) {
	var gotPath string
	var gotModel string
	var gotInputText string
	var gotStream bool
	var gotChatGPTAccountID string
	var gotOpenAIBeta string
	var gotOriginator string
	var gotAccept string

	server := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotChatGPTAccountID = strings.TrimSpace(r.Header.Get("chatgpt-account-id"))
		gotOpenAIBeta = strings.TrimSpace(r.Header.Get("OpenAI-Beta"))
		gotOriginator = strings.TrimSpace(r.Header.Get("originator"))
		gotAccept = strings.TrimSpace(r.Header.Get("Accept"))

		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		gotModel, _ = body["model"].(string)
		gotStream, _ = body["stream"].(bool)
		input, _ := body["input"].([]interface{})
		if len(input) > 0 {
			msg, _ := input[0].(map[string]interface{})
			content, _ := msg["content"].([]interface{})
			if len(content) > 0 {
				block, _ := content[0].(map[string]interface{})
				gotInputText, _ = block["text"].(string)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":[{"content":[{"type":"output_text","text":"hello from codex"}]}]}`))
	}))
	defer server.Close()

	writeDefaultModelConfig(t, "openai-codex", "openai-codex/gpt-5.3-codex", "OPENAI_CODEX_TOKEN")
	t.Setenv("CARRIER_OPENAI_CODEX_BASE_URL", server.URL)
	t.Setenv("OPENAI_CODEX_TOKEN", makeOpenAICodexJWT("acct-test-123"))

	r := &Runtime{}
	got, err := r.replyWithLLM(context.Background(), "hello codex")
	if err != nil {
		t.Fatalf("replyWithLLM() error = %v", err)
	}
	if got != "hello from codex" {
		t.Fatalf("replyWithLLM() = %q, want %q", got, "hello from codex")
	}
	if gotPath != "/codex/responses" {
		t.Fatalf("request path = %q, want %q", gotPath, "/codex/responses")
	}
	if gotModel != "gpt-5.3-codex" {
		t.Fatalf("model = %q, want %q", gotModel, "gpt-5.3-codex")
	}
	if gotInputText != "hello codex" {
		t.Fatalf("input text = %q, want %q", gotInputText, "hello codex")
	}
	if !gotStream {
		t.Fatalf("stream = %v, want true", gotStream)
	}
	if gotChatGPTAccountID != "acct-test-123" {
		t.Fatalf("chatgpt-account-id header = %q, want %q", gotChatGPTAccountID, "acct-test-123")
	}
	if gotOpenAIBeta != "responses=experimental" {
		t.Fatalf("OpenAI-Beta header = %q, want %q", gotOpenAIBeta, "responses=experimental")
	}
	if gotOriginator != "carrier" {
		t.Fatalf("originator header = %q, want %q", gotOriginator, "carrier")
	}
	if gotAccept != "text/event-stream" {
		t.Fatalf("Accept header = %q, want %q", gotAccept, "text/event-stream")
	}
}

func TestParseOpenAICodexResponses_SSE(t *testing.T) {
	raw := strings.Join([]string{
		"event: response.output_text.delta",
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello\"}",
		"",
		"event: response.output_text.delta",
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\" world\"}",
		"",
		"event: response.completed",
		"data: {\"type\":\"response.completed\",\"response\":{\"output_text\":\"Hello world\"}}",
		"",
		"data: [DONE]",
		"",
	}, "\n")
	got, err := parseOpenAICodexResponses([]byte(raw))
	if err != nil {
		t.Fatalf("parseOpenAICodexResponses() error = %v", err)
	}
	if got != "Hello world" {
		t.Fatalf("parseOpenAICodexResponses() = %q, want %q", got, "Hello world")
	}
}

func TestReplyWithLLM_OpenAIUsesChatCompletionsEndpoint(t *testing.T) {
	var gotPath string

	server := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hello from openai"}}]}`))
	}))
	defer server.Close()

	writeDefaultModelConfig(t, "openai", "openai/gpt-5.3", "OPENAI_API_KEY")
	t.Setenv("CARRIER_OPENAI_BASE_URL", server.URL)
	t.Setenv("OPENAI_API_KEY", "sk-test")

	r := &Runtime{}
	got, err := r.replyWithLLM(context.Background(), "hello")
	if err != nil {
		t.Fatalf("replyWithLLM() error = %v", err)
	}
	if got != "hello from openai" {
		t.Fatalf("replyWithLLM() = %q, want %q", got, "hello from openai")
	}
	if gotPath != "/chat/completions" {
		t.Fatalf("request path = %q, want %q", gotPath, "/chat/completions")
	}
}

func makeOpenAICodexJWT(accountID string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payloadStruct := map[string]interface{}{
		openAICodexJWTClaimPath: map[string]string{
			"chatgpt_account_id": accountID,
		},
	}
	rawPayload, _ := json.Marshal(payloadStruct)
	payload := base64.RawURLEncoding.EncodeToString(rawPayload)
	return header + "." + payload + ".sig"
}
