package lifecycle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRotateLogFile_NoFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.log")
	if err := rotateLogFile(path, 100); err != nil {
		t.Fatalf("unexpected error for missing file: %v", err)
	}
}

func TestRotateLogFile_UnderThreshold(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "small.log")
	if err := os.WriteFile(path, make([]byte, 50), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := rotateLogFile(path, 100); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Original file should still exist
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file should still exist: %v", err)
	}
	// Backup should not exist
	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Fatal("backup should not exist for small file")
	}
}

func TestRotateLogFile_OverThreshold(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.log")
	data := make([]byte, 200)
	for i := range data {
		data[i] = 'A'
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := rotateLogFile(path, 100); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Original should be gone (renamed)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("original file should have been renamed")
	}
	// Backup should exist with original content
	backup, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatalf("backup should exist: %v", err)
	}
	if len(backup) != 200 {
		t.Fatalf("backup size = %d, want 200", len(backup))
	}
}

func TestRotateLogFile_OverwritesPreviousBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.log")
	// Create an old backup
	if err := os.WriteFile(path+".1", []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Create oversized log
	if err := os.WriteFile(path, make([]byte, 200), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := rotateLogFile(path, 100); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	backup, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatal(err)
	}
	if len(backup) != 200 {
		t.Fatalf("backup should be overwritten, got size %d", len(backup))
	}
}

func TestProcessManager_StartRotatesLog(t *testing.T) {
	dir := t.TempDir()
	pm := NewProcessManager(dir)
	agentID := "rotate-agent"

	// Create an oversized log file
	logPath := filepath.Join(dir, agentID+".log")
	if err := os.WriteFile(logPath, make([]byte, maxLogSize+1), 0o644); err != nil {
		t.Fatal(err)
	}

	pid, err := pm.Start(agentID, "sleep", []string{"60"})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if pid == 0 {
		t.Fatal("expected non-zero PID")
	}
	defer func() { _ = pm.Stop(agentID) }()

	// Backup should exist
	if _, err := os.Stat(logPath + ".1"); err != nil {
		t.Fatalf("backup log should exist after rotation: %v", err)
	}
	// New log file should be small
	fi, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("new log should exist: %v", err)
	}
	if fi.Size() > 1024 {
		t.Fatalf("new log should be small, got %d bytes", fi.Size())
	}
}
