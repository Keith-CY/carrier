package lifecycle

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"time"

	"carrier/daemon/internal/baseagent"
)

// Evidence represents structured diagnostic evidence collected during a crash or failure.
type Evidence struct {
	Version   string        `json:"version"`
	AgentID   string        `json:"agentId"`
	Timestamp string        `json:"timestamp"`
	ExitCode  *int          `json:"exitCode,omitempty"`
	LogTail   []string      `json:"logTail"`
	HostInfo  HostInfo      `json:"hostInfo"`
	Probes    []ProbeResult `json:"probes"`
	TraceID   string        `json:"traceId"`
}

// SystemInfo contains runtime system metrics (memory, CPU).
type SystemInfo struct {
	MemAllocMB   uint64 `json:"memAllocMB"`
	MemTotalMB   uint64 `json:"memTotalMB"`
	NumGoroutine int    `json:"numGoroutine"`
	NumCPU       int    `json:"numCPU"`
}

// EvidenceCollector gathers diagnostic evidence when a crash or failure occurs.
type EvidenceCollector struct {
	logStore      map[string][]string
	logLimit      int
	probeRunner   func() []ProbeResult
	exitCodeStore map[string]*int
}

// NewEvidenceCollector creates a new evidence collector.
func NewEvidenceCollector(logStore map[string][]string, exitCodeStore map[string]*int, logLimit int) *EvidenceCollector {
	return &EvidenceCollector{
		logStore:      logStore,
		logLimit:      logLimit,
		probeRunner:   runDiagnosticProbes,
		exitCodeStore: exitCodeStore,
	}
}

// Collect gathers all available evidence for the given agent.
func (ec *EvidenceCollector) Collect(agentID string, lastError string) Evidence {
	hostname, _ := os.Hostname()

	// Gather log tail (last N lines)
	logTail := ec.collectLogTail(agentID)

	// Gather exit code if available
	exitCode := ec.collectExitCode(agentID)

	// Run diagnostic probes
	probes := ec.probeRunner()

	evidence := Evidence{
		Version:   "1.0.0",
		AgentID:   agentID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		ExitCode:  exitCode,
		LogTail:   logTail,
		HostInfo: HostInfo{
			OS:        runtime.GOOS,
			Arch:      runtime.GOARCH,
			Hostname:  hostname,
			GoVersion: runtime.Version(),
		},
		Probes:  probes,
		TraceID: generateTraceID(),
	}

	return evidence
}

// collectLogTail retrieves the last N lines from the agent's log buffer.
func (ec *EvidenceCollector) collectLogTail(agentID string) []string {
	logs, ok := ec.logStore[agentID]
	if !ok || len(logs) == 0 {
		return []string{}
	}

	// Return last N lines (up to logLimit)
	start := 0
	if len(logs) > ec.logLimit {
		start = len(logs) - ec.logLimit
	}

	// Create a copy to avoid sharing underlying array
	tail := make([]string, len(logs)-start)
	copy(tail, logs[start:])
	return tail
}

// collectExitCode retrieves the process exit code if available.
func (ec *EvidenceCollector) collectExitCode(agentID string) *int {
	exitCode, ok := ec.exitCodeStore[agentID]
	if !ok {
		return nil
	}
	return exitCode
}

// collectSystemInfo gathers current system resource metrics.
func collectSystemInfo() SystemInfo {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return SystemInfo{
		MemAllocMB:   m.Alloc / 1024 / 1024,
		MemTotalMB:   m.TotalAlloc / 1024 / 1024,
		NumGoroutine: runtime.NumGoroutine(),
		NumCPU:       runtime.NumCPU(),
	}
}

// ToBaseAgentEvidence converts internal Evidence to baseagent.Evidence format.
func (e *Evidence) ToBaseAgentEvidence(lastError string) baseagent.Evidence {
	// Format health probe results as a summary string
	healthProbe := formatProbeResults(e.Probes)

	return baseagent.Evidence{
		AgentID:     e.AgentID,
		LastError:   lastError,
		ExitCode:    e.ExitCode,
		LogTail:     e.LogTail,
		HealthProbe: healthProbe,
	}
}

// formatProbeResults formats probe results into a readable summary string.
func formatProbeResults(probes []ProbeResult) string {
	if len(probes) == 0 {
		return "no probes run"
	}

	summary := fmt.Sprintf("%d probes: ", len(probes))
	passed := 0
	failed := 0
	for _, p := range probes {
		if p.Status == "pass" {
			passed++
		} else if p.Status == "fail" {
			failed++
		}
	}
	summary += fmt.Sprintf("%d passed, %d failed", passed, failed)
	return summary
}

// ToJSON serializes evidence to JSON for storage.
func (e *Evidence) ToJSON() ([]byte, error) {
	return json.MarshalIndent(e, "", "  ")
}

// SaveToFile writes evidence to a JSON file.
func (e *Evidence) SaveToFile(path string) error {
	data, err := e.ToJSON()
	if err != nil {
		return fmt.Errorf("marshal evidence: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write evidence file: %w", err)
	}

	return nil
}
