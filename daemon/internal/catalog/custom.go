package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type customManifest struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Description  string   `json:"description"`
	Capabilities []string `json:"capabilities"`
	Status       string   `json:"status"`
}

func customCatalogDir() (string, error) {
	if custom := strings.TrimSpace(os.Getenv("CARRIER_CUSTOM_CATALOG_DIR")); custom != "" {
		return custom, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".carrier", "catalog", "custom"), nil
}

func LoadCustomManifests() ([]Entry, error) {
	dir, err := customCatalogDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		manifest, err := parseCustomManifestFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		if strings.TrimSpace(manifest.ID) == "" {
			continue
		}
		status := StatusActive
		if strings.EqualFold(strings.TrimSpace(manifest.Status), string(StatusCandidate)) {
			status = StatusCandidate
		}
		out = append(out, Entry{
			ID:           strings.TrimSpace(manifest.ID),
			Name:         firstNonEmpty(strings.TrimSpace(manifest.Name), strings.TrimSpace(manifest.ID)),
			Version:      firstNonEmpty(strings.TrimSpace(manifest.Version), "custom"),
			Status:       status,
			Capabilities: manifest.Capabilities,
			Description:  strings.TrimSpace(manifest.Description),
		})
	}
	return out, nil
}

func RegisterCustomManifest(manifestPath string) (string, error) {
	manifestPath = strings.TrimSpace(manifestPath)
	if manifestPath == "" {
		return "", fmt.Errorf("manifest path is required")
	}
	m, err := parseCustomManifestFile(manifestPath)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(m.ID) == "" {
		return "", fmt.Errorf("manifest id is required")
	}
	dir, err := customCatalogDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	ext := filepath.Ext(manifestPath)
	if ext == "" {
		ext = ".json"
	}
	dst := filepath.Join(dir, strings.TrimSpace(m.ID)+ext)
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(dst, raw, 0o600); err != nil {
		return "", err
	}
	return strings.TrimSpace(m.ID), nil
}

func RemoveCustomManifest(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("manifest id is required")
	}
	dir, err := customCatalogDir()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, id+".") || name == id {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
	return nil
}

func parseCustomManifestFile(path string) (customManifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return customManifest{}, err
	}
	var m customManifest
	if json.Unmarshal(raw, &m) == nil {
		return m, nil
	}
	return parseTOMLManifest(raw), nil
}

func parseTOMLManifest(raw []byte) customManifest {
	m := customManifest{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), "\"'")
		switch key {
		case "id":
			m.ID = value
		case "name":
			m.Name = value
		case "version":
			m.Version = value
		case "description":
			m.Description = value
		case "status":
			m.Status = value
		case "capabilities":
			m.Capabilities = parseTOMLArray(parts[1])
		}
	}
	return m
}

func parseTOMLArray(raw string) []string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.Trim(strings.TrimSpace(part), "\"'")
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
