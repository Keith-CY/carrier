package lifecycle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCappedFileWriter_RotatesAtMaxBytes(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	w, err := newCappedFileWriter(logPath, 100)
	if err != nil {
		t.Fatalf("newCappedFileWriter: %v", err)
	}
	defer w.Close()

	data := make([]byte, 60)
	for i := range data {
		data[i] = 'A'
	}
	if _, err := w.Write(data); err != nil {
		t.Fatalf("Write: %v", err)
	}

	rotatedPath := logPath + ".1"
	if _, err := os.Stat(rotatedPath); err == nil {
		t.Fatal("rotated file should not exist yet")
	}

	if _, err := w.Write(data); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if _, err := os.Stat(rotatedPath); err != nil {
		t.Fatalf("rotated file should exist: %v", err)
	}

	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("stat current log: %v", err)
	}
	if info.Size() >= 100 {
		t.Errorf("current log should be < 100 bytes after rotation, got %d", info.Size())
	}
}

func TestCappedFileWriter_AppendsToExisting(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	if err := os.WriteFile(logPath, make([]byte, 80), 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := newCappedFileWriter(logPath, 100)
	if err != nil {
		t.Fatalf("newCappedFileWriter: %v", err)
	}
	defer w.Close()

	if _, err := w.Write(make([]byte, 30)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if _, err := os.Stat(logPath + ".1"); err != nil {
		t.Fatalf("rotation should have happened: %v", err)
	}
}
