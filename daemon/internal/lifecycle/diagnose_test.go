package lifecycle

import (
	"bytes"
	"encoding/json"
	"errors"
	"runtime"
	"testing"
	"time"
)

// --- Mock implementations for deterministic testing ---

type fixedClock struct {
	t time.Time
}

func (c *fixedClock) Now() time.Time { return c.t }

type staticHostInfo struct {
	hostname  string
	hostErr   error
	os        string
	arch      string
	goVersion string
}

func (h *staticHostInfo) Hostname() (string, error) { return h.hostname, h.hostErr }
func (h *staticHostInfo) OS() string                { return h.os }
func (h *staticHostInfo) Arch() string              { return h.arch }
func (h *staticHostInfo) GoVersion() string         { return h.goVersion }

type staticProbeRunner struct {
	probes []ProbeResult
}

func (r *staticProbeRunner) RunProbes() []ProbeResult { return r.probes }

type fixedRandReader struct {
	data []byte
}

func (r *fixedRandReader) Read(b []byte) (int, error) {
	return copy(b, r.data), nil
}

type failingRandReader struct{}

func (failingRandReader) Read([]byte) (int, error) {
	return 0, errors.New("rand failure")
}

// --- Deterministic tests ---

func TestBuildDiagnoseManifest_Deterministic(t *testing.T) {
	ts := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	randBytes := bytes.Repeat([]byte{0xab}, 16)

	manifest := buildDiagnoseManifest("agent-007",
		WithClock(&fixedClock{t: ts}),
		WithHostInfoProvider(&staticHostInfo{
			hostname:  "test-node",
			os:        "linux",
			arch:      "amd64",
			goVersion: "go1.23.0",
		}),
		WithProbeRunner(&staticProbeRunner{
			probes: []ProbeResult{
				{Name: "mock_probe", Status: "pass", Message: "ok", DurationMs: 5},
			},
		}),
		WithRandReader(&fixedRandReader{data: randBytes}),
	)

	if manifest.Version != "1.0.0" {
		t.Errorf("version = %q, want %q", manifest.Version, "1.0.0")
	}
	if manifest.AgentID != "agent-007" {
		t.Errorf("agentID = %q, want %q", manifest.AgentID, "agent-007")
	}
	if manifest.Timestamp != "2025-06-15T12:00:00Z" {
		t.Errorf("timestamp = %q, want %q", manifest.Timestamp, "2025-06-15T12:00:00Z")
	}
	if manifest.TraceID != "abababababababababababababababab" {
		t.Errorf("traceID = %q, want %q", manifest.TraceID, "abababababababababababababababab")
	}
	if manifest.Host.OS != "linux" {
		t.Errorf("host.os = %q, want %q", manifest.Host.OS, "linux")
	}
	if manifest.Host.Arch != "amd64" {
		t.Errorf("host.arch = %q, want %q", manifest.Host.Arch, "amd64")
	}
	if manifest.Host.Hostname != "test-node" {
		t.Errorf("host.hostname = %q, want %q", manifest.Host.Hostname, "test-node")
	}
	if manifest.Host.GoVersion != "go1.23.0" {
		t.Errorf("host.goVersion = %q, want %q", manifest.Host.GoVersion, "go1.23.0")
	}
	if len(manifest.Probes) != 1 || manifest.Probes[0].Name != "mock_probe" {
		t.Errorf("unexpected probes: %+v", manifest.Probes)
	}
}

func TestBuildDiagnoseManifest_HostnameError(t *testing.T) {
	manifest := buildDiagnoseManifest("agent-err",
		WithClock(&fixedClock{t: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}),
		WithHostInfoProvider(&staticHostInfo{
			hostname:  "",
			hostErr:   errors.New("no hostname"),
			os:        "darwin",
			arch:      "arm64",
			goVersion: "go1.22.0",
		}),
		WithProbeRunner(&staticProbeRunner{probes: []ProbeResult{}}),
		WithRandReader(&fixedRandReader{data: bytes.Repeat([]byte{0x00}, 16)}),
	)

	if manifest.Host.Hostname != "" {
		t.Errorf("hostname should be empty on error, got %q", manifest.Host.Hostname)
	}
	if manifest.TraceID != "00000000000000000000000000000000" {
		t.Errorf("traceID = %q, want all zeros", manifest.TraceID)
	}
}

func TestBuildDiagnoseManifest_Reproducible(t *testing.T) {
	opts := []DiagnoseOption{
		WithClock(&fixedClock{t: time.Date(2025, 3, 1, 8, 30, 0, 0, time.UTC)}),
		WithHostInfoProvider(&staticHostInfo{
			hostname: "node-1", os: "linux", arch: "amd64", goVersion: "go1.23.0",
		}),
		WithProbeRunner(&staticProbeRunner{probes: []ProbeResult{
			{Name: "p1", Status: "pass", Message: "ok", DurationMs: 0},
		}}),
		WithRandReader(&fixedRandReader{data: bytes.Repeat([]byte{0xff}, 16)}),
	}

	m1 := buildDiagnoseManifest("a1", opts...)
	m2 := buildDiagnoseManifest("a1", opts...)

	d1, _ := json.Marshal(m1)
	d2, _ := json.Marshal(m2)
	if string(d1) != string(d2) {
		t.Error("two calls with same inputs produced different outputs")
	}
}

func TestGenerateTraceIDFrom_FailingReader(t *testing.T) {
	id := generateTraceIDFrom(failingRandReader{})
	if id == "" {
		t.Error("should produce fallback ID on reader failure")
	}
	// Fallback is timestamp-based, so length varies but should be non-empty
	if len(id) < 10 {
		t.Errorf("fallback ID too short: %q", id)
	}
}

// --- Original tests (updated to work with options signature) ---

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

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}

	var restored DiagnoseManifest
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("failed to unmarshal manifest: %v", err)
	}

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
	if len(restored.Probes) != len(original.Probes) {
		t.Fatalf("probes length mismatch: got %d, want %d", len(restored.Probes), len(original.Probes))
	}
	probe := restored.Probes[0]
	if probe.Name != "test_probe" || probe.Status != "pass" || probe.Message != "test message" || probe.DurationMs != 42 {
		t.Errorf("probe mismatch: %+v", probe)
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
			{Name: "probe_1", Status: "pass", Message: "ok", DurationMs: 10},
			{Name: "probe_2", Status: "fail", Message: "error occurred", DurationMs: 100},
		},
		TraceID: "trace-abc-123",
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal to map: %v", err)
	}

	for _, key := range []string{"version", "agentId", "timestamp", "host", "probes", "traceId"} {
		if _, exists := raw[key]; !exists {
			t.Errorf("missing required key in JSON: %q", key)
		}
	}

	hostMap, ok := raw["host"].(map[string]interface{})
	if !ok {
		t.Fatal("host is not a map")
	}
	for _, key := range []string{"os", "arch", "hostname", "goVersion"} {
		if _, exists := hostMap[key]; !exists {
			t.Errorf("missing required key in host: %q", key)
		}
	}

	probesArray, ok := raw["probes"].([]interface{})
	if !ok {
		t.Fatal("probes is not an array")
	}
	if len(probesArray) != 2 {
		t.Errorf("expected 2 probes, got %d", len(probesArray))
	}
}

func TestBuildDiagnoseManifest_RequiredFields(t *testing.T) {
	agentID := "test-agent-456"
	manifest := buildDiagnoseManifest(agentID)

	if manifest.Version != "1.0.0" {
		t.Errorf("version = %q, want %q", manifest.Version, "1.0.0")
	}
	if manifest.AgentID != agentID {
		t.Errorf("agentID = %q, want %q", manifest.AgentID, agentID)
	}
	if manifest.Timestamp == "" {
		t.Error("timestamp should not be empty")
	}
	if _, err := time.Parse(time.RFC3339, manifest.Timestamp); err != nil {
		t.Errorf("timestamp is not valid RFC3339: %v", err)
	}
	if manifest.TraceID == "" {
		t.Error("traceID should not be empty")
	}
	if len(manifest.TraceID) != 32 {
		t.Errorf("traceID length = %d, want 32", len(manifest.TraceID))
	}
	if len(manifest.Probes) == 0 {
		t.Error("probes should not be empty")
	}
}

func TestBuildDiagnoseManifest_HostInfo(t *testing.T) {
	manifest := buildDiagnoseManifest("test-agent")

	if manifest.Host.OS != runtime.GOOS {
		t.Errorf("host.os = %q, want %q", manifest.Host.OS, runtime.GOOS)
	}
	if manifest.Host.Arch != runtime.GOARCH {
		t.Errorf("host.arch = %q, want %q", manifest.Host.Arch, runtime.GOARCH)
	}
	if manifest.Host.GoVersion != runtime.Version() {
		t.Errorf("host.goVersion = %q, want %q", manifest.Host.GoVersion, runtime.Version())
	}
}

func TestBuildDiagnoseManifest_ProbeValidation(t *testing.T) {
	manifest := buildDiagnoseManifest("test-agent")

	if len(manifest.Probes) < 1 {
		t.Error("expected at least 1 probe result")
	}

	validStatuses := map[string]bool{"pass": true, "fail": true, "skip": true}
	for i, probe := range manifest.Probes {
		if probe.Name == "" {
			t.Errorf("probe[%d].name should not be empty", i)
		}
		if !validStatuses[probe.Status] {
			t.Errorf("probe[%d].status = %q, want one of: pass, fail, skip", i, probe.Status)
		}
		if probe.Message == "" {
			t.Errorf("probe[%d].message should not be empty", i)
		}
		if probe.DurationMs < 0 {
			t.Errorf("probe[%d].durationMs = %d, should be >= 0", i, probe.DurationMs)
		}
	}
}

func TestProbeResult_StatusValues(t *testing.T) {
	for _, status := range []string{"pass", "fail", "skip"} {
		probe := ProbeResult{Name: "test", Status: status, Message: "test", DurationMs: 0}
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
}

func TestGenerateTraceID_Format(t *testing.T) {
	id := generateTraceID()
	if len(id) != 32 {
		t.Errorf("traceID length = %d, want 32", len(id))
	}
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("traceID contains non-hex character: %c", c)
		}
	}
}
