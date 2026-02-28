package logging

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type LogQuery struct {
	Since  time.Time
	Until  time.Time
	Level  string
	Grep   string
	Limit  int
	Format string
}

type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	AgentID   string    `json:"agentId"`
}

var logLinePattern = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T[^\s]+)\s+\[?([A-Za-z]+)\]?\s+(.*)$`)

func ParseLogLine(line string) LogEntry {
	line = strings.TrimSpace(line)
	if line == "" {
		return LogEntry{}
	}
	matches := logLinePattern.FindStringSubmatch(line)
	if len(matches) != 4 {
		return LogEntry{Level: "INFO", Message: line}
	}
	ts, _ := time.Parse(time.RFC3339, matches[1])
	if ts.IsZero() {
		ts, _ = time.Parse(time.RFC3339Nano, matches[1])
	}
	return LogEntry{
		Timestamp: ts,
		Level:     strings.ToUpper(strings.TrimSpace(matches[2])),
		Message:   strings.TrimSpace(matches[3]),
	}
}

func SearchLogs(agentID string, query LogQuery) ([]LogEntry, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, fmt.Errorf("agentID is required")
	}
	lines, err := readAgentLogLines(agentID)
	if err != nil {
		return nil, err
	}
	level := strings.ToUpper(strings.TrimSpace(query.Level))
	grep := strings.TrimSpace(query.Grep)
	limit := query.Limit
	if limit <= 0 {
		limit = 200
	}
	out := make([]LogEntry, 0, min(limit, len(lines)))
	for _, line := range lines {
		entry := ParseLogLine(line)
		entry.AgentID = agentID
		if !query.Since.IsZero() && !entry.Timestamp.IsZero() && entry.Timestamp.Before(query.Since) {
			continue
		}
		if !query.Until.IsZero() && !entry.Timestamp.IsZero() && entry.Timestamp.After(query.Until) {
			continue
		}
		if level != "" && !strings.EqualFold(strings.TrimSpace(entry.Level), level) {
			continue
		}
		if grep != "" && !strings.Contains(strings.ToLower(entry.Message), strings.ToLower(grep)) {
			continue
		}
		out = append(out, entry)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func readAgentLogLines(agentID string) ([]string, error) {
	path := filepath.Join(processLogDir(), agentID+".log")
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	lines := make([]string, 0, 256)
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for s.Scan() {
		lines = append(lines, s.Text())
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func processLogDir() string {
	if custom := strings.TrimSpace(os.Getenv("CARRIER_PROCESS_LOG_DIR")); custom != "" {
		return custom
	}
	if custom := strings.TrimSpace(os.Getenv("CARRIER_DAEMON_PROCESS_LOG_DIR")); custom != "" {
		return custom
	}
	if custom := strings.TrimSpace(os.Getenv("TMPDIR")); custom != "" {
		return filepath.Join(custom, "carrier-daemon-process-logs")
	}
	return filepath.Join(os.TempDir(), "carrier-daemon-process-logs")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
