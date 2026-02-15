package lifecycle

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"runtime"
	"time"
)

// DiagnoseManifest represents the structured metadata for a diagnostic artifact.
type DiagnoseManifest struct {
	Version   string        `json:"version"`
	AgentID   string        `json:"agentId"`
	Timestamp string        `json:"timestamp"`
	Host      HostInfo      `json:"host"`
	Probes    []ProbeResult `json:"probes"`
	TraceID   string        `json:"traceId"`
}

// HostInfo contains information about the host environment.
type HostInfo struct {
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	Hostname  string `json:"hostname"`
	GoVersion string `json:"goVersion"`
}

// ProbeResult represents the result of a diagnostic probe.
type ProbeResult struct {
	Name       string `json:"name"`
	Status     string `json:"status"` // "pass", "fail", or "skip"
	Message    string `json:"message"`
	DurationMs int64  `json:"durationMs"`
}

// buildDiagnoseManifest constructs a DiagnoseManifest with host information and basic probes.
func buildDiagnoseManifest(agentID string) DiagnoseManifest {
	hostname, _ := os.Hostname()

	manifest := DiagnoseManifest{
		Version:   "1.0.0",
		AgentID:   agentID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Host: HostInfo{
			OS:        runtime.GOOS,
			Arch:      runtime.GOARCH,
			Hostname:  hostname,
			GoVersion: runtime.Version(),
		},
		Probes:  runDiagnosticProbes(),
		TraceID: generateTraceID(),
	}

	return manifest
}

// generateTraceID creates a random trace identifier.
func generateTraceID() string {
	b := make([]byte, 16) // 16 bytes = 32 hex chars
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID if random generation fails
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b)
}

// runDiagnosticProbes executes a series of diagnostic checks and returns the results.
func runDiagnosticProbes() []ProbeResult {
	probes := []ProbeResult{}

	// Probe: Check hostname resolution
	start := time.Now()
	hostname, err := os.Hostname()
	duration := time.Since(start).Milliseconds()
	if err != nil {
		probes = append(probes, ProbeResult{
			Name:       "hostname_check",
			Status:     "fail",
			Message:    "failed to resolve hostname: " + err.Error(),
			DurationMs: duration,
		})
	} else {
		probes = append(probes, ProbeResult{
			Name:       "hostname_check",
			Status:     "pass",
			Message:    "hostname resolved: " + hostname,
			DurationMs: duration,
		})
	}

	// Probe: Check working directory access
	start = time.Now()
	wd, err := os.Getwd()
	duration = time.Since(start).Milliseconds()
	if err != nil {
		probes = append(probes, ProbeResult{
			Name:       "workdir_check",
			Status:     "fail",
			Message:    "failed to get working directory: " + err.Error(),
			DurationMs: duration,
		})
	} else {
		probes = append(probes, ProbeResult{
			Name:       "workdir_check",
			Status:     "pass",
			Message:    "working directory: " + wd,
			DurationMs: duration,
		})
	}

	// Probe: Check temp directory access
	start = time.Now()
	tempDir := os.TempDir()
	duration = time.Since(start).Milliseconds()
	if tempDir == "" {
		probes = append(probes, ProbeResult{
			Name:       "tempdir_check",
			Status:     "fail",
			Message:    "temp directory not available",
			DurationMs: duration,
		})
	} else {
		// Try to create a temp file to verify write access
		tmpFile, err := os.CreateTemp(tempDir, "diagnose-probe-*")
		if err != nil {
			probes = append(probes, ProbeResult{
				Name:       "tempdir_check",
				Status:     "fail",
				Message:    "temp directory exists but not writable: " + err.Error(),
				DurationMs: duration,
			})
		} else {
			_ = tmpFile.Close()
			_ = os.Remove(tmpFile.Name())
			probes = append(probes, ProbeResult{
				Name:       "tempdir_check",
				Status:     "pass",
				Message:    "temp directory writable: " + tempDir,
				DurationMs: duration,
			})
		}
	}

	return probes
}
