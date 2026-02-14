package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadFile currently accepts JSON manifests for deterministic parsing with stdlib.
// YAML support can be added later if we introduce a YAML parser dependency.
func LoadFile(path string) (Manifest, error) {
	if ext := strings.ToLower(filepath.Ext(path)); ext != ".json" {
		return Manifest{}, fmt.Errorf("unsupported manifest file extension %q: only .json is supported in this scaffold", ext)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest: %w", err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest json: %w", err)
	}

	if err := m.Validate(); err != nil {
		return Manifest{}, fmt.Errorf("validate manifest: %w", err)
	}

	return m, nil
}
