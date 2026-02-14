package manifest

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFileAcceptsCatalogManifest(t *testing.T) {
	path := filepath.Join("..", "..", "..", "catalog", "openclaw.manifest.json")

	m, err := LoadFile(path)
	if err != nil {
		t.Fatalf("expected catalog manifest to load, got error: %v", err)
	}
	if m.ID != "openclaw" {
		t.Fatalf("expected manifest id openclaw, got %q", m.ID)
	}
}

func TestLoadFileReturnsValidationErrorForMissingRequiredField(t *testing.T) {
	path := filepath.Join("testdata", "invalid_missing_runtime_start.manifest.json")

	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "runtime.start.command is required") {
		t.Fatalf("expected runtime.start.command validation error, got %q", err.Error())
	}
}
