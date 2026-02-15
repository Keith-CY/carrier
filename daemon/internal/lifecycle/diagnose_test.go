package lifecycle

import (
	"encoding/json"
	"runtime"
	"testing"
	"time"
)

func TestDiagnoseManifest_MarshalUnmarshal(t *testing.T) {
	original := DiagnoseManifest{
		Version:   "1.0.0",
		AgentID:   "test-agent-123",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Host: HostInfo{
			OS:        "linux",
			Arch:      "amd64",
			Hostname:  "test-host",
			GoVersion: "go1.21.0",
		},
		Probes: []ProbeResult{
			{
				Name:       "test_probe",
				Status:     "pass",
				Message:    "test message",
				DurationMs: 42,
			},
		},
		TraceID: "abc123def456",
	}

	// Marshal to JSON
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}

	// Unmarshal back
	var restored DiagnoseManifest
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("failed to unmarshal manifest: %v", err)
	}

	// Verify all fields
	if restored.Version != original.Version {
		t.Errorf("version mismatch: got %q, want %q", restored.Version, original.Version)
	}
	if restored.AgentID != original.AgentID {
		t.Errorf("agentID mismatch: got %q, want %q", restored.AgentID, original.AgentID)
	}
	if restored.Timestamp != original.Timestamp {
		t.Errorf("timestamp mismatch: got %q, want %q", restored.Timestamp, original.Timestamp)
	}
	if restored.TraceID != original.TraceID {
		t.Errorf("traceID mismatch: got %q, want %q", restored.TraceID, original.TraceID)
	}

	// Verify host info
	if restored.Host.OS != original.Host.OS {
		t.Errorf("host.os mismatch: got %q, want %q", restored.Host.OS, original.Host.OS)
	}
	if restored.Host.Arch != original.Host.Arch {
		t.Errorf("host.arch mismatch: got %q, want %q", restored.Host.Arch, original.Host.Arch)
	}
	if restored.Host.Hostname != original.Host.Hostname {
		t.Errorf("host.hostname mismatch: got %q, want %q", restored.Host.Hostname, original.Host.Hostname)
	}
	if restored.Host.GoVersion != original.Host.GoVersion {
		t.Errorf("host.goVersion mismatch: got %q, want %q", restored.Host.GoVersion, original.Host.GoVersion)
	}

	// Verify probes
	if len(restored.Probes) != len(original.Probes) {
		t.Fatalf("probes length mismatch: got %d, want %d", len(restored.Probes), len(original.Probes))
	}
	probe := restored.Probes[0]
	if probe.Name != "test_probe" {
		t.Errorf("probe.name mismatch: got %q, want %q", probe.Name, "test_probe")
	}
	if probe.Status != "pass" {
		t.Errorf("probe.status mismatch: got %q, want %q", probe.Status, "pass")
	}
	if probe.Message != "test message" {
		t.Errorf("probe.message mismatch: got %q, want %q", probe.Message, "test message")
	}
	if probe.DurationMs != 42 {
		t.Errorf("probe.durationMs mismatch: got %d, want %d", probe.DurationMs, 42)
	}
}

func TestDiagnoseManifest_JSONStructure(t *testing.T) {
	manifest := DiagnoseManifest{
		Version:   "1.0.0",
		AgentID:   "agent-xyz",
		Timestamp: "2024-01-01T12:00:00Z",
		Host: HostInfo{
			OS:        "darwin",
			Arch:      "arm64",
			Hostname:  "macbook",
			GoVersion: "go1.22.0",
		},
		Probes: []ProbeResult{
			{
				Name:       "probe_1",
				Status:     "pass",
				Message:    "ok",
				DurationMs: 10,
			},
			{
				Name:       "probe_2",
				Status:     "fail",
				Message:    "error occurred",
				DurationMs: 100,
			},
		},
		TraceID: "trace-abc-123",
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Verify JSON contains expected keys
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal to map: %v", err)
	}

	requiredKeys := []string{"version", "agentId", "timestamp", "host", "probes", "traceId"}
	for _, key := range requiredKeys {
		if _, exists := raw[key]; !exists {
			t.Errorf("missing required key in JSON: %q", key)
		}
	}

	// Verify host structure
	hostMap, ok := raw["host"].(map[string]interface{})
	if !ok {
		t.Fatal("host is not a map")
	}
	hostKeys := []string{"os", "arch", "hostname", "goVersion"}
	for _, key := range hostKeys {
		if _, exists := hostMap[key]; !exists {
			t.Errorf("missing required key in host: %q", key)
		}
	}

	// Verify probes structure
	probesArray, ok := raw["probes"].([]interface{})
	if !ok {
		t.Fatal("probes is not an array")
	}
	if len(probesArray) != 2 {
		t.Errorf("expected 2 probes, got %d", len(probesArray))
	}

	probe0, ok := probesArray[0].(map[string]interface{})
	if !ok {
		t.Fatal("probe[0] is not a map")
	}
	probeKeys := []string{"name", "status", "message", "durationMs"}
	for _, key := range probeKeys {
		if _, exists := probe0[key]; !exists {
			t.Errorf("missing required key in probe: %q", key)
		}
	}
}

func TestBuildDiagnoseManifest_RequiredFields(t *testing.T) {
	agentID := "test-agent-456"
	manifest := buildDiagnoseManifest(agentID)

	// Verify version
	if manifest.Version == "" {
		t.Error("version should not be empty")
	}
	if manifest.Version != "1.0.0" {
		t.Errorf("version = %q, want %q", manifest.Version, "1.0.0")
	}

	// Verify agentID
	if manifest.AgentID != agentID {
		t.Errorf("agentID = %q, want %q", manifest.AgentID, agentID)
	}

	// Verify timestamp
	if manifest.Timestamp == "" {
		t.Error("timestamp should not be empty")
	}
	// Verify timestamp is valid RFC3339
	if _, err := time.Parse(time.RFC3339, manifest.Timestamp); err != nil {
		t.Errorf("timestamp is not valid RFC3339: %v", err)
	}

	// Verify traceID
	if manifest.TraceID == "" {
		t.Error("traceID should not be empty")
	}
	if len(manifest.TraceID) != 32 { // 16 bytes = 32 hex chars
		t.Errorf("traceID length = %d, want 32", len(manifest.TraceID))
	}

	// Verify probes exist
	if manifest.Probes == nil {
		t.Error("probes should not be nil")
	}
	if len(manifest.Probes) == 0 {
		t.Error("probes should not be empty")
	}
}

func TestBuildDiagnoseManifest_HostInfo(t *testing.T) {
	manifest := buildDiagnoseManifest("test-agent")

	// Verify OS
	if manifest.Host.OS == "" {
		t.Error("host.os should not be empty")
	}
	if manifest.Host.OS != runtime.GOOS {
		t.Errorf("host.os = %q, want %q", manifest.Host.OS, runtime.GOOS)
	}

	// Verify Arch
	if manifest.Host.Arch == "" {
		t.Error("host.arch should not be empty")
	}
	if manifest.Host.Arch != runtime.GOARCH {
		t.Errorf("host.arch = %q, want %q", manifest.Host.Arch, runtime.GOARCH)
	}

	// Verify GoVersion
	if manifest.Host.GoVersion == "" {
		t.Error("host.goVersion should not be empty")
	}
	if manifest.Host.GoVersion != runtime.Version() {
		t.Errorf("host.goVersion = %q, want %q", manifest.Host.GoVersion, runtime.Version())
	}

	// Hostname can be empty in some environments, but it should be populated when possible
	// We just verify it's a string (empty or not)
	_ = manifest.Host.Hostname
}

func TestBuildDiagnoseManifest_ProbeValidation(t *testing.T) {
	manifest := buildDiagnoseManifest("test-agent")

	// Verify at least some probes ran
	if len(manifest.Probes) < 1 {
		t.Error("expected at least 1 probe result")
	}

	for i, probe := range manifest.Probes {
		// Verify probe has a name
		if probe.Name == "" {
			t.Errorf("probe[%d].name should not be empty", i)
		}

		// Verify status is one of: pass, fail, skip
		validStatuses := map[string]bool{"pass": true, "fail": true, "skip": true}
		if !validStatuses[probe.Status] {
			t.Errorf("probe[%d].status = %q, want one of: pass, fail, skip", i, probe.Status)
		}

		// Verify message exists
		if probe.Message == "" {
			t.Errorf("probe[%d].message should not be empty", i)
		}

		// Verify duration is non-negative
		if probe.DurationMs < 0 {
			t.Errorf("probe[%d].durationMs = %d, should be >= 0", i, probe.DurationMs)
		}
	}
}

func TestProbeResult_StatusValues(t *testing.T) {
	validStatuses := []string{"pass", "fail", "skip"}

	for _, status := range validStatuses {
		probe := ProbeResult{
			Name:       "test",
			Status:     status,
			Message:    "test",
			DurationMs: 0,
		}

		data, err := json.Marshal(probe)
		if err != nil {
			t.Fatalf("failed to marshal probe with status %q: %v", status, err)
		}

		var restored ProbeResult
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatalf("failed to unmarshal probe with status %q: %v", status, err)
		}

		if restored.Status != status {
			t.Errorf("status mismatch: got %q, want %q", restored.Status, status)
		}
	}
}

func TestGenerateTraceID_Uniqueness(t *testing.T) {
	// Generate multiple trace IDs and verify they're unique
	ids := make(map[string]bool)
	count := 100

	for i := 0; i < count; i++ {
		id := generateTraceID()
		if id == "" {
			t.Error("generateTraceID returned empty string")
		}
		if ids[id] {
			t.Errorf("generateTraceID returned duplicate: %q", id)
		}
		ids[id] = true
	}

	if len(ids) != count {
		t.Errorf("expected %d unique IDs, got %d", count, len(ids))
	}
}

func TestGenerateTraceID_Format(t *testing.T) {
	id := generateTraceID()

	// Verify it's hex-encoded (32 chars for 16 bytes)
	if len(id) != 32 {
		t.Errorf("traceID length = %d, want 32", len(id))
	}

	// Verify it's valid hex
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("traceID contains non-hex character: %c", c)
		}
	}
}
