package logging

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseLogLine(t *testing.T) {
	entry := ParseLogLine("2026-02-28T10:00:00Z [ERROR] install failed")
	if entry.Level != "ERROR" {
		t.Fatalf("Level = %q, want ERROR", entry.Level)
	}
	if entry.Message != "install failed" {
		t.Fatalf("Message = %q, want %q", entry.Message, "install failed")
	}
	if entry.Timestamp.IsZero() {
		t.Fatal("Timestamp should not be zero")
	}
}

func TestSearchLogsFiltersByLevel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CARRIER_PROCESS_LOG_DIR", dir)
	logPath := filepath.Join(dir, "openclaw.log")
	body := "2026-02-28T10:00:00Z [INFO] ready\n2026-02-28T10:00:01Z [ERROR] failed\n"
	if err := os.WriteFile(logPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}

	entries, err := SearchLogs("openclaw", LogQuery{Level: "error", Limit: 10})
	if err != nil {
		t.Fatalf("SearchLogs error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(entries))
	}
	if entries[0].Level != "ERROR" {
		t.Fatalf("entry level = %q, want ERROR", entries[0].Level)
	}
}
