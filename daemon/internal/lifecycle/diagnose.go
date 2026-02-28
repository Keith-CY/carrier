package lifecycle

import (
	"crypto/rand"
	"encoding/hex"
	"io"
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

// Clock abstracts time operations for deterministic testing.
type Clock interface {
	Now() time.Time
}

// SystemClock implements Clock using the real system clock.
type SystemClock struct{}

// Now returns the current UTC time.
func (SystemClock) Now() time.Time { return time.Now().UTC() }

// HostInfoProvider abstracts host environment queries.
type HostInfoProvider interface {
	Hostname() (string, error)
	OS() string
	Arch() string
	GoVersion() string
}

// RealHostInfoProvider implements HostInfoProvider using real system calls.
type RealHostInfoProvider struct{}

func (RealHostInfoProvider) Hostname() (string, error) { return os.Hostname() }
func (RealHostInfoProvider) OS() string                { return runtime.GOOS }
func (RealHostInfoProvider) Arch() string              { return runtime.GOARCH }
func (RealHostInfoProvider) GoVersion() string         { return runtime.Version() }

// ProbeRunner abstracts diagnostic probe execution.
type ProbeRunner interface {
	RunProbes() []ProbeResult
}

// DefaultProbeRunner executes real diagnostic probes against the system.
type DefaultProbeRunner struct {
	Clock Clock
}

// RunProbes executes a series of diagnostic checks and returns the results.
func (r *DefaultProbeRunner) RunProbes() []ProbeResult {
	probes := []ProbeResult{}

	// Probe: Check hostname resolution
	start := r.Clock.Now()
	hostname, err := os.Hostname()
	duration := r.Clock.Now().Sub(start).Milliseconds()
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
	start = r.Clock.Now()
	wd, err := os.Getwd()
	duration = r.Clock.Now().Sub(start).Milliseconds()
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
	start = r.Clock.Now()
	tempDir := os.TempDir()
	duration = r.Clock.Now().Sub(start).Milliseconds()
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

// RandReader abstracts random byte generation.
type RandReader interface {
	Read(b []byte) (int, error)
}

// CryptoRandReader uses crypto/rand for random bytes.
type CryptoRandReader struct{}

func (CryptoRandReader) Read(b []byte) (int, error) { return io.ReadFull(rand.Reader, b) }

// DiagnoseOption configures a diagnose builder.
type DiagnoseOption func(*diagnoseConfig)

type diagnoseConfig struct {
	clock       Clock
	hostInfo    HostInfoProvider
	probeRunner ProbeRunner
	randReader  RandReader
}

// WithClock sets the clock for manifest generation.
func WithClock(c Clock) DiagnoseOption {
	return func(cfg *diagnoseConfig) { cfg.clock = c }
}

// WithHostInfoProvider sets the host info provider.
func WithHostInfoProvider(h HostInfoProvider) DiagnoseOption {
	return func(cfg *diagnoseConfig) { cfg.hostInfo = h }
}

// WithProbeRunner sets the probe runner.
func WithProbeRunner(p ProbeRunner) DiagnoseOption {
	return func(cfg *diagnoseConfig) { cfg.probeRunner = p }
}

// WithRandReader sets the random reader for trace ID generation.
func WithRandReader(r RandReader) DiagnoseOption {
	return func(cfg *diagnoseConfig) { cfg.randReader = r }
}

func defaultDiagnoseConfig() *diagnoseConfig {
	clk := SystemClock{}
	return &diagnoseConfig{
		clock:       clk,
		hostInfo:    RealHostInfoProvider{},
		probeRunner: &DefaultProbeRunner{Clock: clk},
		randReader:  CryptoRandReader{},
	}
}

// buildDiagnoseManifest constructs a DiagnoseManifest with host information and basic probes.
func buildDiagnoseManifest(agentID string, opts ...DiagnoseOption) DiagnoseManifest {
	cfg := defaultDiagnoseConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	hostname, _ := cfg.hostInfo.Hostname()

	manifest := DiagnoseManifest{
		Version:   "1.0.0",
		AgentID:   agentID,
		Timestamp: cfg.clock.Now().Format(time.RFC3339),
		Host: HostInfo{
			OS:        cfg.hostInfo.OS(),
			Arch:      cfg.hostInfo.Arch(),
			Hostname:  hostname,
			GoVersion: cfg.hostInfo.GoVersion(),
		},
		Probes:  cfg.probeRunner.RunProbes(),
		TraceID: generateTraceIDFrom(cfg.randReader),
	}

	return manifest
}

// generateTraceID creates a random trace identifier using crypto/rand.
func generateTraceID() string {
	return generateTraceIDFrom(CryptoRandReader{})
}

// generateTraceIDFrom creates a random trace identifier using the provided reader.
func generateTraceIDFrom(r io.Reader) string {
	b := make([]byte, 16) // 16 bytes = 32 hex chars
	if _, err := r.Read(b); err != nil {
		// Fallback to timestamp-based ID if random generation fails
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b)
}

// runDiagnosticProbes executes a series of diagnostic checks and returns the results.
func runDiagnosticProbes() []ProbeResult {
	runner := &DefaultProbeRunner{Clock: SystemClock{}}
	return runner.RunProbes()
}
