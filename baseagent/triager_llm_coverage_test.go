package baseagent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

type fallbackTriagerFake struct {
	result TriageResult
	err    error
}

func (f fallbackTriagerFake) Analyze(_ context.Context, _ Evidence) (TriageResult, error) {
	return f.result, f.err
}

func TestCompactNonEmpty(t *testing.T) {
	got := compactNonEmpty([]string{" a ", "", "b", " ", "c"}, 2)
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("unexpected compact output: %+v", got)
	}

	all := compactNonEmpty([]string{" a ", "", "b"}, 0)
	if len(all) != 2 || all[0] != "a" || all[1] != "b" {
		t.Fatalf("unexpected full compact output: %+v", all)
	}
}

func TestBuildInstallFailurePromptRedactsAndTrimsTail(t *testing.T) {
	lines := make([]string, 0, 60)
	for i := 0; i < 60; i++ {
		lines = append(lines, fmt.Sprintf("line-%d", i))
	}
	lines[59] = "OPENAI_API_KEY=sk-live-abc"

	prompt := buildInstallFailurePrompt(Evidence{
		AgentID:   "openclaw",
		LastError: "token OPENAI_API_KEY=sk-live-abc invalid",
		LogTail:   lines,
	})

	if !strings.Contains(prompt, "Analyze the install failure evidence") {
		t.Fatalf("missing prompt prefix: %q", prompt)
	}
	if strings.Contains(prompt, "sk-live-abc") {
		t.Fatalf("expected secret to be redacted in prompt: %q", prompt)
	}
	if strings.Contains(prompt, "line-0") {
		t.Fatalf("expected early log lines trimmed from prompt: %q", prompt)
	}
}

func TestParseLLMTriageResponseErrors(t *testing.T) {
	if _, err := parseLLMTriageResponse("   "); err == nil {
		t.Fatal("expected empty response error")
	}
	if _, err := parseLLMTriageResponse("plain text"); err == nil {
		t.Fatal("expected missing-json error")
	}
	if _, err := parseLLMTriageResponse("{bad json}"); err == nil {
		t.Fatal("expected json decode error")
	}
}

func TestNewLLMTriagerNilFallback(t *testing.T) {
	triager := NewLLMTriager(nil)
	if triager == nil {
		t.Fatal("expected non-nil triager")
	}
}

func TestLLMTriagerAnalyzeSuccess(t *testing.T) {
	server := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"resolved\":false,\"summary\":\"cache issue\",\"suggestedActions\":[\"  npm cache clean --force  \",\"\"],\"requiresRemoteDiagnosis\":false,\"repairAction\":{\"command\":\"npm cache clean --force\",\"targetPath\":\"\",\"riskLevel\":\"low\"}}"}}]}`))
	}))
	defer server.Close()

	writeDefaultModelConfig(t, "openai", "openai/gpt-5.3", "OPENAI_API_KEY")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("CARRIER_OPENAI_BASE_URL", server.URL)

	triager := NewLLMTriager(fallbackTriagerFake{})
	got, err := triager.Analyze(context.Background(), Evidence{
		AgentID:   "openclaw",
		LastError: "install failed",
		LogTail:   []string{"line 1"},
	})
	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}
	if got.Summary != "cache issue" {
		t.Fatalf("unexpected summary: %q", got.Summary)
	}
	if len(got.SuggestedActions) != 1 || got.SuggestedActions[0] != "npm cache clean --force" {
		t.Fatalf("unexpected actions: %+v", got.SuggestedActions)
	}
	if got.RepairAction == nil || got.RepairAction.Command != "npm cache clean --force" {
		t.Fatalf("unexpected repair action: %+v", got.RepairAction)
	}
}

func TestLLMTriagerAnalyzeSetsDefaultSummaryWhenMissing(t *testing.T) {
	server := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"resolved\":false,\"summary\":\"\",\"suggestedActions\":[],\"requiresRemoteDiagnosis\":true,\"repairAction\":null}"}}]}`))
	}))
	defer server.Close()

	writeDefaultModelConfig(t, "openai", "openai/gpt-5.3", "OPENAI_API_KEY")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("CARRIER_OPENAI_BASE_URL", server.URL)

	triager := NewLLMTriager(fallbackTriagerFake{})
	got, err := triager.Analyze(context.Background(), Evidence{AgentID: "openclaw"})
	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}
	if got.Summary != "LLM triage produced no summary; using fallback guidance." {
		t.Fatalf("unexpected default summary: %q", got.Summary)
	}
}

func TestLLMTriagerAnalyzeFallbackOnInvalidJSON(t *testing.T) {
	server := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"not-json"}}]}`))
	}))
	defer server.Close()

	writeDefaultModelConfig(t, "openai", "openai/gpt-5.3", "OPENAI_API_KEY")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("CARRIER_OPENAI_BASE_URL", server.URL)

	triager := NewLLMTriager(fallbackTriagerFake{
		result: TriageResult{
			Summary:          "fallback summary",
			SuggestedActions: []string{"check logs"},
		},
	})
	got, err := triager.Analyze(context.Background(), Evidence{AgentID: "openclaw"})
	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}
	if !strings.Contains(got.Summary, "fallback summary") || !strings.Contains(got.Summary, "LLM unavailable:") {
		t.Fatalf("unexpected fallback summary: %q", got.Summary)
	}
	if got.RepairAction != nil {
		t.Fatalf("expected nil repair action in fallback, got %+v", got.RepairAction)
	}
	if !got.RequiresRemoteDiagnosis {
		t.Fatal("expected requires remote diagnosis")
	}
}

func TestFallbackResultDefaultsAndTruncation(t *testing.T) {
	triager := NewLLMTriager(fallbackTriagerFake{
		result: TriageResult{},
	})
	longErr := errors.New(strings.Repeat("x", 400) + "\nline2")
	got := triager.fallbackResult(context.Background(), Evidence{}, longErr)

	if !strings.Contains(got.Summary, "Base Agent fallback triage activated.") {
		t.Fatalf("unexpected summary: %q", got.Summary)
	}
	if len(got.Summary) > 400 {
		t.Fatalf("expected truncated summary, got len=%d", len(got.Summary))
	}
	if len(got.SuggestedActions) == 0 {
		t.Fatal("expected default suggested actions")
	}
	if !got.RequiresRemoteDiagnosis {
		t.Fatal("expected requires remote diagnosis to be true")
	}
}
