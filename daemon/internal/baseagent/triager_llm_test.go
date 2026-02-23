package baseagent

import (
	"context"
	"path/filepath"
	"testing"
)

func TestParseLLMTriageResponse_JSONObject(t *testing.T) {
	raw := `{
		"resolved": false,
		"summary": "Dependency cache issue",
		"suggestedActions": ["clean npm cache", "retry install"],
		"requiresRemoteDiagnosis": false,
		"repairAction": {
			"command": "npm cache clean --force",
			"targetPath": "",
			"riskLevel": "low"
		}
	}`

	parsed, err := parseLLMTriageResponse(raw)
	if err != nil {
		t.Fatalf("parseLLMTriageResponse() error = %v", err)
	}
	if parsed == nil || parsed.RepairAction == nil {
		t.Fatal("expected parsed repair action")
	}
	if parsed.RepairAction.Command != "npm cache clean --force" {
		t.Fatalf("repair command = %q", parsed.RepairAction.Command)
	}
}

func TestParseLLMTriageResponse_WithMarkdownWrapper(t *testing.T) {
	raw := "```json\n{\"resolved\":false,\"summary\":\"wrapped\",\"suggestedActions\":[],\"requiresRemoteDiagnosis\":true,\"repairAction\":null}\n```"
	parsed, err := parseLLMTriageResponse(raw)
	if err != nil {
		t.Fatalf("parseLLMTriageResponse() error = %v", err)
	}
	if parsed.Summary != "wrapped" {
		t.Fatalf("summary = %q, want wrapped", parsed.Summary)
	}
}

func TestLLMTriagerAnalyze_FallbackWhenLLMUnavailable(t *testing.T) {
	t.Setenv("CARRIER_CONFIG", filepath.Join(t.TempDir(), "missing-config.v2.json"))

	triager := NewLLMTriager(NoopTriager{})
	result, err := triager.Analyze(context.Background(), Evidence{
		AgentID:   "openclaw",
		LastError: "install failed",
		LogTail:   []string{"line 1"},
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if !result.RequiresRemoteDiagnosis {
		t.Fatal("expected RequiresRemoteDiagnosis=true on fallback")
	}
	if result.RepairAction != nil {
		t.Fatalf("expected no repair action on fallback, got %+v", result.RepairAction)
	}
	if result.Summary == "" {
		t.Fatal("expected fallback summary")
	}
}
