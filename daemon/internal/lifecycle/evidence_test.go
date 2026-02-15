package lifecycle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEvidenceCollector_Collect(t *testing.T) {
	// Setup
	logStore := map[string][]string{
		"agent1": {"log line 1", "log line 2", "log line 3"},
		"agent2": {},
	}
	exitCodeStore := map[string]*int{
		"agent1": intPtr(1),
	}

	collector := NewEvidenceCollector(logStore, exitCodeStore, 100)

	t.Run("collect evidence with logs and exit code", func(t *testing.T) {
		evidence := collector.Collect("agent1", "test error")

		if evidence.AgentID != "agent1" {
			t.Errorf("expected AgentID 'agent1', got %s", evidence.AgentID)
		}

		if evidence.Version != "1.0.0" {
			t.Errorf("expected Version '1.0.0', got %s", evidence.Version)
		}

		if len(evidence.LogTail) != 3 {
			t.Errorf("expected 3 log lines, got %d", len(evidence.LogTail))
		}

		if evidence.ExitCode == nil {
			t.Fatal("expected exit code to be set")
		}

		if *evidence.ExitCode != 1 {
			t.Errorf("expected exit code 1, got %d", *evidence.ExitCode)
		}

		if len(evidence.Probes) == 0 {
			t.Error("expected probes to be run")
		}

		if evidence.TraceID == "" {
			t.Error("expected TraceID to be set")
		}

		if evidence.HostInfo.OS == "" {
			t.Error("expected HostInfo.OS to be set")
		}
	})

	t.Run("collect evidence without exit code", func(t *testing.T) {
		evidence := collector.Collect("agent2", "test error")

		if evidence.ExitCode != nil {
			t.Errorf("expected exit code to be nil, got %v", evidence.ExitCode)
		}

		if len(evidence.LogTail) != 0 {
			t.Errorf("expected 0 log lines, got %d", len(evidence.LogTail))
		}
	})

	t.Run("collect evidence for unknown agent", func(t *testing.T) {
		evidence := collector.Collect("unknown", "test error")

		if evidence.ExitCode != nil {
			t.Errorf("expected exit code to be nil, got %v", evidence.ExitCode)
		}

		if len(evidence.LogTail) != 0 {
			t.Errorf("expected 0 log lines, got %d", len(evidence.LogTail))
		}
	})
}

func TestEvidenceCollector_CollectLogTail(t *testing.T) {
	t.Run("collect all logs when under limit", func(t *testing.T) {
		logStore := map[string][]string{
			"agent1": {"line1", "line2", "line3"},
		}
		collector := NewEvidenceCollector(logStore, make(map[string]*int), 100)

		tail := collector.collectLogTail("agent1")

		if len(tail) != 3 {
			t.Errorf("expected 3 log lines, got %d", len(tail))
		}

		if tail[0] != "line1" || tail[1] != "line2" || tail[2] != "line3" {
			t.Errorf("unexpected log content: %v", tail)
		}
	})

	t.Run("truncate logs when over limit", func(t *testing.T) {
		logStore := map[string][]string{
			"agent1": {"line1", "line2", "line3", "line4", "line5"},
		}
		collector := NewEvidenceCollector(logStore, make(map[string]*int), 3)

		tail := collector.collectLogTail("agent1")

		if len(tail) != 3 {
			t.Errorf("expected 3 log lines, got %d", len(tail))
		}

		// Should get last 3 lines
		if tail[0] != "line3" || tail[1] != "line4" || tail[2] != "line5" {
			t.Errorf("unexpected log content: %v", tail)
		}
	})

	t.Run("return empty for missing agent", func(t *testing.T) {
		logStore := map[string][]string{}
		collector := NewEvidenceCollector(logStore, make(map[string]*int), 100)

		tail := collector.collectLogTail("missing")

		if len(tail) != 0 {
			t.Errorf("expected empty log tail, got %d lines", len(tail))
		}
	})
}

func TestEvidence_ToBaseAgentEvidence(t *testing.T) {
	evidence := Evidence{
		AgentID:  "test-agent",
		ExitCode: intPtr(2),
		LogTail:  []string{"log1", "log2"},
		Probes: []ProbeResult{
			{Name: "test", Status: "pass"},
			{Name: "test2", Status: "fail"},
		},
	}

	baseEvidence := evidence.ToBaseAgentEvidence("test error")

	if baseEvidence.AgentID != "test-agent" {
		t.Errorf("expected AgentID 'test-agent', got %s", baseEvidence.AgentID)
	}

	if baseEvidence.LastError != "test error" {
		t.Errorf("expected LastError 'test error', got %s", baseEvidence.LastError)
	}

	if baseEvidence.ExitCode == nil || *baseEvidence.ExitCode != 2 {
		t.Errorf("expected ExitCode 2, got %v", baseEvidence.ExitCode)
	}

	if len(baseEvidence.LogTail) != 2 {
		t.Errorf("expected 2 log lines, got %d", len(baseEvidence.LogTail))
	}

	if baseEvidence.HealthProbe == "" {
		t.Error("expected HealthProbe to be set")
	}
}

func TestFormatProbeResults(t *testing.T) {
	tests := []struct {
		name     string
		probes   []ProbeResult
		expected string
	}{
		{
			name:     "no probes",
			probes:   []ProbeResult{},
			expected: "no probes run",
		},
		{
			name: "all passed",
			probes: []ProbeResult{
				{Status: "pass"},
				{Status: "pass"},
			},
			expected: "2 probes: 2 passed, 0 failed",
		},
		{
			name: "mixed results",
			probes: []ProbeResult{
				{Status: "pass"},
				{Status: "fail"},
				{Status: "skip"},
			},
			expected: "3 probes: 1 passed, 1 failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatProbeResults(tt.probes)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestEvidence_ToJSON(t *testing.T) {
	evidence := Evidence{
		Version:  "1.0.0",
		AgentID:  "test-agent",
		ExitCode: intPtr(1),
		LogTail:  []string{"log1"},
	}

	data, err := evidence.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	// Verify it's valid JSON
	var decoded Evidence
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if decoded.AgentID != "test-agent" {
		t.Errorf("expected AgentID 'test-agent', got %s", decoded.AgentID)
	}
}

func TestEvidence_SaveToFile(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "evidence.json")

	evidence := Evidence{
		Version:  "1.0.0",
		AgentID:  "test-agent",
		ExitCode: intPtr(1),
		LogTail:  []string{"log1", "log2"},
	}

	err := evidence.SaveToFile(filePath)
	if err != nil {
		t.Fatalf("SaveToFile failed: %v", err)
	}

	// Verify file was created and contains valid JSON
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	var decoded Evidence
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if decoded.AgentID != "test-agent" {
		t.Errorf("expected AgentID 'test-agent', got %s", decoded.AgentID)
	}

	if len(decoded.LogTail) != 2 {
		t.Errorf("expected 2 log lines, got %d", len(decoded.LogTail))
	}
}

func TestCollectSystemInfo(t *testing.T) {
	info := collectSystemInfo()

	if info.NumCPU <= 0 {
		t.Error("expected NumCPU to be positive")
	}

	if info.NumGoroutine <= 0 {
		t.Error("expected NumGoroutine to be positive")
	}

	// Memory values can be 0 in some test environments, so just check they're present
	// (not negative)
	if info.MemAllocMB < 0 {
		t.Error("expected MemAllocMB to be non-negative")
	}

	if info.MemTotalMB < 0 {
		t.Error("expected MemTotalMB to be non-negative")
	}
}

// Helper function
func intPtr(i int) *int {
	return &i
}
