package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"carrier/shared/catalog"
)

func TestLiveProviderSmoke(t *testing.T) {
	providerID := catalog.NormalizeProviderID(strings.TrimSpace(os.Getenv("CARRIER_LIVE_PROVIDER")))
	if providerID == "" {
		t.Skip("set CARRIER_LIVE_PROVIDER to enable live provider smoke")
	}
	spec := catalog.GetProvider(providerID)
	if spec == nil {
		t.Fatalf("unsupported live provider: %q", providerID)
	}

	apiKey := strings.TrimSpace(os.Getenv("CARRIER_LIVE_API_KEY"))
	if apiKey == "" {
		t.Skipf("CARRIER_LIVE_API_KEY is empty, skip live smoke for provider=%s", providerID)
	}

	model := strings.TrimSpace(os.Getenv("CARRIER_LIVE_MODEL"))
	if model == "" {
		model = strings.TrimSpace(spec.ExampleModel)
	}
	if _, tail, ok := strings.Cut(model, "/"); ok && strings.TrimSpace(tail) != "" {
		model = strings.TrimSpace(tail)
	}
	if model == "" {
		t.Fatalf("live model is empty for provider %s", providerID)
	}

	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("CARRIER_LIVE_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(strings.TrimSpace(spec.DefaultBase), "/")
	}
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	client := &http.Client{Timeout: 45 * time.Second}
	switch providerID {
	case "anthropic":
		liveSmokeAnthropic(t, client, baseURL, model, apiKey)
	default:
		liveSmokeOpenAICompatible(t, client, providerID, baseURL, model, apiKey)
	}
}

func liveSmokeOpenAICompatible(t *testing.T, client *http.Client, providerID, baseURL, model, apiKey string) {
	t.Helper()

	payload := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": "Reply with exactly: OK",
			},
		},
		"max_tokens":  24,
		"temperature": 0,
	}
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal openai-compatible payload: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(rawPayload))
	if err != nil {
		t.Fatalf("build openai-compatible request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	// Recommended by OpenRouter for clearer routing/account attribution.
	req.Header.Set("HTTP-Referer", "https://github.com/Keith-CY/carrier")
	req.Header.Set("X-Title", "carrier-live-smoke")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("send openai-compatible request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("provider %s live smoke failed status=%d body=%s", providerID, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode openai-compatible response: %v body=%s", err, strings.TrimSpace(string(body)))
	}
	if len(decoded.Choices) == 0 {
		t.Fatalf("provider %s returned empty choices: %s", providerID, strings.TrimSpace(string(body)))
	}
	content := strings.TrimSpace(decoded.Choices[0].Message.Content)
	if content == "" {
		t.Fatalf("provider %s returned empty message content: %s", providerID, strings.TrimSpace(string(body)))
	}
	t.Logf("provider=%s model=%s response=%q", providerID, model, truncateLiveResponse(content, 120))
}

func liveSmokeAnthropic(t *testing.T, client *http.Client, baseURL, model, apiKey string) {
	t.Helper()

	payload := map[string]interface{}{
		"model":      model,
		"max_tokens": 24,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": "Reply with exactly: OK",
			},
		},
	}
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal anthropic payload: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/messages", bytes.NewReader(rawPayload))
	if err != nil {
		t.Fatalf("build anthropic request: %v", err)
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("send anthropic request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("provider anthropic live smoke failed status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var decoded struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode anthropic response: %v body=%s", err, strings.TrimSpace(string(body)))
	}
	text := ""
	for _, item := range decoded.Content {
		if strings.EqualFold(strings.TrimSpace(item.Type), "text") && strings.TrimSpace(item.Text) != "" {
			text = strings.TrimSpace(item.Text)
			break
		}
	}
	if text == "" {
		t.Fatalf("provider anthropic returned empty text content: %s", strings.TrimSpace(string(body)))
	}
	t.Logf("provider=anthropic model=%s response=%q", model, truncateLiveResponse(text, 120))
}

func truncateLiveResponse(input string, limit int) string {
	trimmed := strings.TrimSpace(input)
	if limit <= 0 || len(trimmed) <= limit {
		return trimmed
	}
	return fmt.Sprintf("%s...", trimmed[:limit])
}
