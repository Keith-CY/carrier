package baseagent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type fakeNetError struct {
	msg string
}

func (e fakeNetError) Error() string   { return e.msg }
func (e fakeNetError) Timeout() bool   { return false }
func (e fakeNetError) Temporary() bool { return true }

func TestParseModelError(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{
			name:   "nested error with code",
			status: 401,
			body:   `{"error":{"message":"invalid token","code":"invalid_api_key"}}`,
			want:   "invalid token (invalid_api_key)",
		},
		{
			name:   "top-level message without code",
			status: 500,
			body:   `{"message":"upstream timeout"}`,
			want:   "upstream timeout",
		},
		{
			name:   "empty detail returns nil",
			status: 500,
			body:   `{"error":{"message":""},"message":""}`,
			want:   "",
		},
		{
			name:   "invalid json returns nil",
			status: 500,
			body:   `{bad`,
			want:   "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := parseModelError(tc.status, []byte(tc.body))
			if tc.want == "" {
				if err != nil {
					t.Fatalf("expected nil error, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestInferProviderEnvVar(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{provider: "openai-codex", want: "OPENAI_CODEX_TOKEN"},
		{provider: "openai", want: "OPENAI_API_KEY"},
		{provider: "anthropic", want: "ANTHROPIC_API_KEY"},
		{provider: "openai-compatible", want: "OPENAI_COMPATIBLE_API_KEY"},
		{provider: "unknown", want: ""},
	}
	for _, tc := range tests {
		if got := inferProviderEnvVar(tc.provider); got != tc.want {
			t.Fatalf("inferProviderEnvVar(%q)=%q want=%q", tc.provider, got, tc.want)
		}
	}
}

func TestParseOpenAICompatibleChatResponseErrors(t *testing.T) {
	if _, err := parseOpenAICompatibleChatResponse([]byte(`{bad`)); err == nil {
		t.Fatal("expected decode error")
	}
	if _, err := parseOpenAICompatibleChatResponse([]byte(`{"choices":[]}`)); err == nil {
		t.Fatal("expected no choices error")
	}
}

func TestParseOpenAICodexSSEErrorAndNoOutput(t *testing.T) {
	errorSSE := strings.Join([]string{
		"event: error",
		`data: {"type":"error","message":"codex unavailable"}`,
		"",
	}, "\n")
	if _, err := parseOpenAICodexSSE([]byte(errorSSE)); err == nil || !strings.Contains(err.Error(), "codex stream error") {
		t.Fatalf("expected codex stream error, got %v", err)
	}

	if _, err := parseOpenAICodexSSE([]byte("event: noop\n\ndata: [DONE]\n\n")); err == nil || !strings.Contains(err.Error(), "no output") {
		t.Fatalf("expected no output error, got %v", err)
	}
}

func TestRequestLLMCompletionErrorScenarios(t *testing.T) {
	writeDefaultModelConfig(t, "openai", "openai/gpt-5.3", "OPENAI_API_KEY")
	t.Setenv("OPENAI_API_KEY", "sk-test")

	t.Run("invalid base url", func(t *testing.T) {
		t.Setenv("CARRIER_OPENAI_BASE_URL", "://bad")
		_, err := requestLLMCompletion(context.Background(), "sys", "hello")
		if err == nil || !strings.Contains(err.Error(), "build model request") {
			t.Fatalf("expected build request error, got %v", err)
		}
	})

	t.Run("transport error", func(t *testing.T) {
		t.Setenv("CARRIER_OPENAI_BASE_URL", "http://127.0.0.1:1")
		_, err := requestLLMCompletion(context.Background(), "sys", "hello")
		if err == nil || !strings.Contains(err.Error(), "model request failed") {
			t.Fatalf("expected transport error, got %v", err)
		}
	})

	t.Run("non-2xx with parsed api error", func(t *testing.T) {
		server := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"invalid token","code":"invalid_api_key"}}`))
		}))
		defer server.Close()

		t.Setenv("CARRIER_OPENAI_BASE_URL", server.URL)
		_, err := requestLLMCompletion(context.Background(), "sys", "hello")
		if err == nil || !strings.Contains(err.Error(), "invalid_api_key") {
			t.Fatalf("expected parsed api error, got %v", err)
		}
	})

	t.Run("non-2xx with plain body", func(t *testing.T) {
		server := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("upstream failed"))
		}))
		defer server.Close()

		t.Setenv("CARRIER_OPENAI_BASE_URL", server.URL)
		_, err := requestLLMCompletion(context.Background(), "sys", "hello")
		if err == nil || !strings.Contains(err.Error(), "upstream failed") {
			t.Fatalf("expected plain body error, got %v", err)
		}
	})

	t.Run("non-2xx with empty body", func(t *testing.T) {
		server := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer server.Close()

		t.Setenv("CARRIER_OPENAI_BASE_URL", server.URL)
		_, err := requestLLMCompletion(context.Background(), "sys", "hello")
		if err == nil || !strings.Contains(err.Error(), "status 502") {
			t.Fatalf("expected status-only error, got %v", err)
		}
	})

	t.Run("invalid success payload", func(t *testing.T) {
		server := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{bad`))
		}))
		defer server.Close()

		t.Setenv("CARRIER_OPENAI_BASE_URL", server.URL)
		_, err := requestLLMCompletion(context.Background(), "sys", "hello")
		if err == nil || !strings.Contains(err.Error(), "decode model response") {
			t.Fatalf("expected decode error, got %v", err)
		}
	})

	t.Run("empty content in success payload", func(t *testing.T) {
		server := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"   "}}]}`))
		}))
		defer server.Close()

		t.Setenv("CARRIER_OPENAI_BASE_URL", server.URL)
		_, err := requestLLMCompletion(context.Background(), "sys", "hello")
		if err == nil || !strings.Contains(err.Error(), "empty content") {
			t.Fatalf("expected empty content error, got %v", err)
		}
	})
}

func TestRequestLLMCompletionOpenAICompatibleDoesNotSetOpenRouterHeaders(t *testing.T) {
	var gotReferer string
	var gotTitle string

	server := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReferer = r.Header.Get("HTTP-Referer")
		gotTitle = r.Header.Get("X-Title")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	writeDefaultModelConfig(t, "openai-compatible", "openai-compatible/demo-model", "OPENAI_COMPATIBLE_API_KEY")
	t.Setenv("OPENAI_COMPATIBLE_API_KEY", "ok-test")
	t.Setenv("CARRIER_OPENAI_BASE_URL", server.URL)

	got, err := requestLLMCompletion(context.Background(), "sys", "hello")
	if err != nil {
		t.Fatalf("requestLLMCompletion error: %v", err)
	}
	if got != "ok" {
		t.Fatalf("unexpected response: %q", got)
	}
	if gotReferer != "" {
		t.Fatalf("unexpected HTTP-Referer header: %q", gotReferer)
	}
	if gotTitle != "" {
		t.Fatalf("unexpected X-Title header: %q", gotTitle)
	}
}

func TestRequestLLMCompletionCodexWithoutAccountHeader(t *testing.T) {
	var gotAccountID string

	server := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccountID = r.Header.Get("chatgpt-account-id")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"output_text":"codex ok"}`))
	}))
	defer server.Close()

	writeDefaultModelConfig(t, "openai-codex", "openai-codex/gpt-5.3-codex", "OPENAI_CODEX_TOKEN")
	t.Setenv("OPENAI_CODEX_TOKEN", "not-a-jwt")
	t.Setenv("CARRIER_OPENAI_CODEX_BASE_URL", server.URL)

	got, err := requestLLMCompletion(context.Background(), "sys", "hello")
	if err != nil {
		t.Fatalf("requestLLMCompletion error: %v", err)
	}
	if got != "codex ok" {
		t.Fatalf("unexpected response: %q", got)
	}
	if gotAccountID != "" {
		t.Fatalf("expected empty chatgpt-account-id header, got %q", gotAccountID)
	}
}

func TestNormalizeModelForProviderAdditionalCases(t *testing.T) {
	if got := normalizeModelForProvider("openai", "   "); got != "" {
		t.Fatalf("expected empty model after trim, got %q", got)
	}
	if got := normalizeModelForProvider("openai", "openai/"); got != "openai/" {
		t.Fatalf("expected unchanged trailing-slash model, got %q", got)
	}
}

func TestExtractModelContentDefaultCase(t *testing.T) {
	if got := extractModelContent(123); got != "" {
		t.Fatalf("expected empty content for unsupported type, got %q", got)
	}
}

func TestExtractOpenAICodexAccountIDBranches(t *testing.T) {
	if got := extractOpenAICodexAccountID("not-a-jwt"); got != "" {
		t.Fatalf("expected empty account id for malformed token, got %q", got)
	}

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	if got := extractOpenAICodexAccountID(header + ".%%%." + "sig"); got != "" {
		t.Fatalf("expected empty account id for undecodable payload, got %q", got)
	}

	payloadWithMissingClaim, _ := json.Marshal(map[string]interface{}{"sub": "abc"})
	paddedPayload := base64.URLEncoding.EncodeToString(payloadWithMissingClaim)
	if got := extractOpenAICodexAccountID(header + "." + paddedPayload + ".sig"); got != "" {
		t.Fatalf("expected empty account id for missing auth claim, got %q", got)
	}

	invalidJSONPayload := base64.RawURLEncoding.EncodeToString([]byte("not-json"))
	if got := extractOpenAICodexAccountID(header + "." + invalidJSONPayload + ".sig"); got != "" {
		t.Fatalf("expected empty account id for invalid json payload, got %q", got)
	}

	payloadWithClaim, _ := json.Marshal(map[string]interface{}{
		openAICodexJWTClaimPath: map[string]string{"chatgpt_account_id": "acct-xyz"},
	})
	paddedClaimPayload := base64.URLEncoding.EncodeToString(payloadWithClaim)
	if got := extractOpenAICodexAccountID(header + "." + paddedClaimPayload + ".sig"); got != "acct-xyz" {
		t.Fatalf("expected account id from padded payload, got %q", got)
	}
}

func TestParseOpenAICodexResponsesAdditionalBranches(t *testing.T) {
	if _, err := parseOpenAICodexResponses([]byte("   ")); err == nil {
		t.Fatal("expected error for empty codex response")
	}

	if got, err := parseOpenAICodexResponses([]byte(`{"output_text":"done"}`)); err != nil || got != "done" {
		t.Fatalf("expected output_text branch, got=%q err=%v", got, err)
	}

	respWithOutput := `{"output":[{"text":"line1","content":[{"text":"line2"}]}]}`
	if got, err := parseOpenAICodexResponses([]byte(respWithOutput)); err != nil || got != "line1\nline2" {
		t.Fatalf("expected output list branch, got=%q err=%v", got, err)
	}

	chatCompat := `{"choices":[{"message":{"content":"chat fallback"}}]}`
	if got, err := parseOpenAICodexResponses([]byte(chatCompat)); err != nil || got != "chat fallback" {
		t.Fatalf("expected chat compatibility fallback, got=%q err=%v", got, err)
	}

	// Leading whitespace keeps looksLikeSSE() false while SSE fallback still works.
	whitespacePrefixedSSE := " data: {\"type\":\"response.output_text.delta\",\"delta\":\"fallback via decode error\"}\n\n"
	if got, err := parseOpenAICodexResponses([]byte(whitespacePrefixedSSE)); err != nil || got != "fallback via decode error" {
		t.Fatalf("expected SSE fallback after decode error, got=%q err=%v", got, err)
	}

	// The "\tdata:" line is recognized by parseOpenAICodexSSE after TrimSpace(line),
	// while looksLikeSSEStream() remains false because it only checks for "\ndata:".
	decodeErrorThenSSEFallback := "x\n\tdata: {\"type\":\"response.output_text.delta\",\"delta\":\"decode-fallback\"}\n\n"
	if got, err := parseOpenAICodexResponses([]byte(decodeErrorThenSSEFallback)); err != nil || got != "decode-fallback" {
		t.Fatalf("expected SSE fallback after decode error, got=%q err=%v", got, err)
	}

	invalidSSE := "data: [DONE]\n\ndata: {bad-json}\n\n"
	if _, err := parseOpenAICodexResponses([]byte(invalidSSE)); err == nil || !strings.Contains(err.Error(), "decode codex response") {
		t.Fatalf("expected decode codex response error for invalid SSE payload, got %v", err)
	}

	if _, err := parseOpenAICodexResponses([]byte(`{"output":[]}`)); err == nil {
		t.Fatal("expected no output error")
	}
}

func TestParseOpenAICodexSSEAdditionalBranches(t *testing.T) {
	withText := strings.Join([]string{
		`data: {"type":"response.output_text","text":"hello from text"}`,
		"",
	}, "\n")
	if got, err := parseOpenAICodexSSE([]byte(withText)); err != nil || got != "hello from text" {
		t.Fatalf("expected text branch, got=%q err=%v", got, err)
	}

	withResponse := strings.Join([]string{
		`data: {"type":"response.completed","response":{"message":{"content":"from response content"}}}`,
		"",
	}, "\n")
	if got, err := parseOpenAICodexSSE([]byte(withResponse)); err != nil || got != "from response content" {
		t.Fatalf("expected response branch, got=%q err=%v", got, err)
	}

	withOutput := strings.Join([]string{
		`data: {"type":"response.output","output":[{"text":"from output array"}]}`,
		"",
	}, "\n")
	if got, err := parseOpenAICodexSSE([]byte(withOutput)); err != nil || got != "from output array" {
		t.Fatalf("expected output branch, got=%q err=%v", got, err)
	}

	withMalformedThenGood := strings.Join([]string{
		`data: {bad`,
		"",
		`data: {"type":"response.output_text.delta","delta":"ok"}`,
		"",
	}, "\n")
	if got, err := parseOpenAICodexSSE([]byte(withMalformedThenGood)); err != nil || got != "ok" {
		t.Fatalf("expected malformed-event skip branch, got=%q err=%v", got, err)
	}

	withErrorNoMessage := strings.Join([]string{
		`data: {"type":"error"}`,
		"",
	}, "\n")
	if _, err := parseOpenAICodexSSE([]byte(withErrorNoMessage)); err == nil || !strings.Contains(err.Error(), "codex stream error") {
		t.Fatalf("expected generic codex stream error, got %v", err)
	}

	withTextAndOutput := strings.Join([]string{
		`data: {"type":"response.output","text":"final text","output":[{"text":"ignored output"}]}`,
		"",
	}, "\n")
	if got, err := parseOpenAICodexSSE([]byte(withTextAndOutput)); err != nil || got != "final text" {
		t.Fatalf("expected completed text to win over output branch, got=%q err=%v", got, err)
	}

	withOutputText := strings.Join([]string{
		`data: {"type":"response.completed","output_text":"from output_text field"}`,
		"",
	}, "\n")
	if got, err := parseOpenAICodexSSE([]byte(withOutputText)); err != nil || got != "from output_text field" {
		t.Fatalf("expected output_text branch, got=%q err=%v", got, err)
	}
}

func TestResolveLLMRuntimeConfigAdditionalBranches(t *testing.T) {
	t.Run("missing config", func(t *testing.T) {
		t.Setenv("CARRIER_CONFIG", "/nonexistent/path/config.v2.json")
		if _, err := resolveLLMRuntimeConfig(); err == nil || !strings.Contains(err.Error(), "default model is not configured") {
			t.Fatalf("expected missing config error, got %v", err)
		}
	})

	t.Run("unknown provider env mapping", func(t *testing.T) {
		writeDefaultModelConfig(t, "unknown-provider", "m1", "")
		if _, err := resolveLLMRuntimeConfig(); err == nil || !strings.Contains(err.Error(), "unsupported provider") {
			t.Fatalf("expected unsupported provider error, got %v", err)
		}
	})

	t.Run("missing provider token", func(t *testing.T) {
		writeDefaultModelConfig(t, "openai", "openai/gpt-5.3", "OPENAI_API_KEY")
		t.Setenv("OPENAI_API_KEY", "")
		if _, err := resolveLLMRuntimeConfig(); err == nil || !strings.Contains(err.Error(), "provider credential is missing") {
			t.Fatalf("expected missing token error, got %v", err)
		}
	})

	t.Run("openai-compatible default base url", func(t *testing.T) {
		writeDefaultModelConfig(t, "openai-compatible", "openai-compatible/demo-model", "OPENAI_COMPATIBLE_API_KEY")
		t.Setenv("OPENAI_COMPATIBLE_API_KEY", "ok-token")
		t.Setenv("CARRIER_OPENAI_BASE_URL", "")
		cfg, err := resolveLLMRuntimeConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.BaseURL != defaultOpenAIBaseURL {
			t.Fatalf("expected openai default base url, got %q", cfg.BaseURL)
		}
	})

	t.Run("codex fallback to openai base url", func(t *testing.T) {
		writeDefaultModelConfig(t, "openai-codex", "openai-codex/gpt-5.3-codex", "OPENAI_CODEX_TOKEN")
		t.Setenv("OPENAI_CODEX_TOKEN", "token")
		t.Setenv("CARRIER_OPENAI_CODEX_BASE_URL", "")
		t.Setenv("CARRIER_OPENAI_BASE_URL", "https://proxy.example/v1")
		cfg, err := resolveLLMRuntimeConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.BaseURL != "https://proxy.example/v1" {
			t.Fatalf("expected codex to fallback to CARRIER_OPENAI_BASE_URL, got %q", cfg.BaseURL)
		}
	})

	t.Run("openai override base url", func(t *testing.T) {
		writeDefaultModelConfig(t, "openai", "openai/gpt-5.3", "OPENAI_API_KEY")
		t.Setenv("OPENAI_API_KEY", "sk")
		t.Setenv("CARRIER_OPENAI_BASE_URL", "https://openai-proxy.example/v1")
		cfg, err := resolveLLMRuntimeConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.BaseURL != "https://openai-proxy.example/v1" {
			t.Fatalf("expected openai override base url, got %q", cfg.BaseURL)
		}
	})

	t.Run("infer env var when env_var is empty", func(t *testing.T) {
		writeDefaultModelConfig(t, "openai", "openai/gpt-5.3", "")
		t.Setenv("OPENAI_API_KEY", "sk-test")
		cfg, err := resolveLLMRuntimeConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Token != "sk-test" {
			t.Fatalf("expected inferred token env var, got %q", cfg.Token)
		}
	})

	t.Run("missing provider or model", func(t *testing.T) {
		writeDefaultModelConfig(t, "", "", "OPENAI_API_KEY")
		t.Setenv("OPENAI_API_KEY", "sk-test")
		if _, err := resolveLLMRuntimeConfig(); err == nil || !strings.Contains(err.Error(), "default model is not configured") {
			t.Fatalf("expected missing provider/model error, got %v", err)
		}
	})
}

func TestRequestLLMCompletionInjectedFailureBranches(t *testing.T) {
	writeDefaultModelConfig(t, "openai", "openai/gpt-5.3", "OPENAI_API_KEY")
	t.Setenv("OPENAI_API_KEY", "sk-test")

	t.Run("marshal failure", func(t *testing.T) {
		_, err := requestLLMCompletionWithDeps(context.Background(), "sys", "hello", llmRequestDeps{
			marshalJSON: func(any) ([]byte, error) {
				return nil, errors.New("marshal failed")
			},
		})
		if err == nil || !strings.Contains(err.Error(), "marshal model request") {
			t.Fatalf("expected marshal failure, got %v", err)
		}
	})

	t.Run("response body read failure", func(t *testing.T) {
		server := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ignored"}}]}`))
		}))
		defer server.Close()
		t.Setenv("CARRIER_OPENAI_BASE_URL", server.URL)

		_, err := requestLLMCompletionWithDeps(context.Background(), "sys", "hello", llmRequestDeps{
			readAll: func(_ io.Reader) ([]byte, error) {
				return nil, errors.New("read failed")
			},
		})
		if err == nil || !strings.Contains(err.Error(), "read model response") {
			t.Fatalf("expected read failure, got %v", err)
		}
	})
}

func TestRequestLLMCompletionWithDepsRetriesTransientErrors(t *testing.T) {
	writeDefaultModelConfig(t, "openai", "openai/gpt-5.3", "OPENAI_API_KEY")
	t.Setenv("OPENAI_API_KEY", "sk-test")

	attempts := 0
	sleepCalls := 0
	got, err := requestLLMCompletionWithDeps(context.Background(), "sys", "hello", llmRequestDeps{
		doRequest: func(_ *http.Request) (*http.Response, error) {
			attempts++
			if attempts < 3 {
				return nil, fakeNetError{msg: "temporary network failure"}
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"ok"}}]}`)),
				Header:     make(http.Header),
			}, nil
		},
		sleep: func(_ context.Context, _ time.Duration) error {
			sleepCalls++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("expected retry success, got %v", err)
	}
	if got != "ok" {
		t.Fatalf("expected successful model content, got %q", got)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
	if sleepCalls != 2 {
		t.Fatalf("expected 2 backoff sleeps, got %d", sleepCalls)
	}
}

func TestRequestLLMCompletionWithDepsNoRetryForNonNetworkError(t *testing.T) {
	writeDefaultModelConfig(t, "openai", "openai/gpt-5.3", "OPENAI_API_KEY")
	t.Setenv("OPENAI_API_KEY", "sk-test")

	attempts := 0
	sleepCalls := 0
	_, err := requestLLMCompletionWithDeps(context.Background(), "sys", "hello", llmRequestDeps{
		doRequest: func(_ *http.Request) (*http.Response, error) {
			attempts++
			return nil, errors.New("boom")
		},
		sleep: func(_ context.Context, _ time.Duration) error {
			sleepCalls++
			return nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "model request failed") {
		t.Fatalf("expected model request failure, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt for non-network errors, got %d", attempts)
	}
	if sleepCalls != 0 {
		t.Fatalf("expected no sleep for non-network errors, got %d", sleepCalls)
	}
}

func TestRequestLLMCompletionWithDepsErrorBodyReadFailureIsReported(t *testing.T) {
	writeDefaultModelConfig(t, "openai", "openai/gpt-5.3", "OPENAI_API_KEY")
	t.Setenv("OPENAI_API_KEY", "sk-test")

	_, err := requestLLMCompletionWithDeps(context.Background(), "sys", "hello", llmRequestDeps{
		doRequest: func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadGateway,
				Body:       io.NopCloser(strings.NewReader("ignored")),
				Header:     make(http.Header),
			}, nil
		},
		readAll: func(_ io.Reader) ([]byte, error) {
			return nil, errors.New("read failed")
		},
	})
	if err == nil || !strings.Contains(err.Error(), "unable to read error response body") {
		t.Fatalf("expected read-error context in failure message, got %v", err)
	}
}
