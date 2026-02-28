package lifecycle

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type AgentMetrics struct {
	CPUPercent   float64    `json:"cpuPercent"`
	MemoryRSS    int64      `json:"memoryRSS"`
	Uptime       int64      `json:"uptime"`
	RestartCount int        `json:"restartCount"`
	LastErrorAt  *time.Time `json:"lastErrorAt,omitempty"`
}

func (s *Service) Metrics(agentID string) (AgentMetrics, error) {
	state, err := s.Status(agentID)
	if err != nil {
		return AgentMetrics{}, err
	}
	metrics := AgentMetrics{RestartCount: state.RestartCount}
	if state.LastError != "" {
		ts := state.UpdatedAt.UTC()
		metrics.LastErrorAt = &ts
	}
	if state.StartedAt != nil {
		metrics.Uptime = int64(s.now().Sub(*state.StartedAt).Seconds())
		if metrics.Uptime < 0 {
			metrics.Uptime = 0
		}
	}

	pid := 0
	if pm, ok := s.processManager.(*ProcessManager); ok {
		pid = pm.PID(agentID)
	}
	if pid > 0 {
		collected, collectErr := collectAgentMetrics(pid)
		if collectErr == nil {
			metrics.CPUPercent = collected.CPUPercent
			metrics.MemoryRSS = collected.MemoryRSS
			if collected.Uptime > 0 {
				metrics.Uptime = collected.Uptime
			}
		}
	}
	return metrics, nil
}

func collectAgentMetrics(pid int) (AgentMetrics, error) {
	if pid <= 0 {
		return AgentMetrics{}, fmt.Errorf("invalid pid %d", pid)
	}
	if runtime.GOOS != "linux" {
		return AgentMetrics{}, fmt.Errorf("collectAgentMetrics only supports Linux")
	}

	procUptimeRaw, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return AgentMetrics{}, err
	}
	uptimeFields := strings.Fields(string(procUptimeRaw))
	if len(uptimeFields) == 0 {
		return AgentMetrics{}, fmt.Errorf("invalid /proc/uptime")
	}
	procUptimeSec, err := strconv.ParseFloat(uptimeFields[0], 64)
	if err != nil {
		return AgentMetrics{}, err
	}

	statRaw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return AgentMetrics{}, err
	}
	fields := parseProcStatFields(string(statRaw))
	if len(fields) < 24 {
		return AgentMetrics{}, fmt.Errorf("unexpected /proc/%d/stat format", pid)
	}

	utimeTicks, _ := strconv.ParseFloat(fields[13], 64)
	stimeTicks, _ := strconv.ParseFloat(fields[14], 64)
	startTicks, _ := strconv.ParseFloat(fields[21], 64)
	rssPages, _ := strconv.ParseInt(fields[23], 10, 64)

	const hz = 100.0
	startSec := startTicks / hz
	elapsedSec := procUptimeSec - startSec
	if elapsedSec <= 0 {
		elapsedSec = 1
	}
	cpuSec := (utimeTicks + stimeTicks) / hz
	cpuPercent := (cpuSec / elapsedSec) * 100.0
	if cpuPercent < 0 {
		cpuPercent = 0
	}

	pageSize := int64(os.Getpagesize())
	rssBytes := rssPages * pageSize
	if rssBytes < 0 {
		rssBytes = 0
	}

	return AgentMetrics{
		CPUPercent: cpuPercent,
		MemoryRSS:  rssBytes,
		Uptime:     int64(elapsedSec),
	}, nil
}

func parseProcStatFields(raw string) []string {
	raw = strings.TrimSpace(raw)
	endComm := strings.LastIndex(raw, ")")
	if endComm < 0 || endComm+2 >= len(raw) {
		return strings.Fields(raw)
	}
	prefix := strings.Fields(raw[:endComm+1])
	tail := strings.Fields(raw[endComm+2:])
	if len(prefix) == 0 {
		return tail
	}
	fields := make([]string, 0, 2+len(tail))
	fields = append(fields, prefix[0])
	fields = append(fields, strings.TrimPrefix(prefix[len(prefix)-1], "("))
	fields = append(fields, tail...)
	return fields
}
