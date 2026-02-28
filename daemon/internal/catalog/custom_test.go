package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegisterCustomManifest(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CARRIER_CUSTOM_CATALOG_DIR", filepath.Join(dir, "custom"))
	src := filepath.Join(dir, "agent.toml")
	if err := os.WriteFile(src, []byte("id = \"my-agent\"\nname = \"My Agent\"\nversion = \"1.2.3\"\n"), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	id, err := RegisterCustomManifest(src)
	if err != nil {
		t.Fatalf("RegisterCustomManifest error: %v", err)
	}
	if id != "my-agent" {
		t.Fatalf("id = %q, want %q", id, "my-agent")
	}
	if _, err := os.Stat(filepath.Join(dir, "custom", "my-agent.toml")); err != nil {
		t.Fatalf("expected copied manifest: %v", err)
	}
}

func TestListIncludesCustomAgents(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CARRIER_CUSTOM_CATALOG_DIR", filepath.Join(dir, "custom"))
	if err := os.MkdirAll(filepath.Join(dir, "custom"), 0o700); err != nil {
		t.Fatalf("mkdir custom dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "custom", "custom-agent.toml"), []byte("id = \"custom-agent\"\nname = \"Custom Agent\"\n"), 0o600); err != nil {
		t.Fatalf("write custom manifest: %v", err)
	}
	found := false
	for _, e := range List() {
		if e.ID == "custom-agent" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected custom-agent to be listed")
	}
}
