package baseagent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMCPConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	raw := []byte(`{
  "servers": [
    {
      "name": "repo",
      "tools": [
        {
          "name": "repo_search",
          "description": "Search the repository index.",
          "aliases": ["repo_query"]
        }
      ]
    }
  ]
}`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadMCPConfigFile(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.Servers) != 1 || cfg.Servers[0].Name != "repo" {
		t.Fatalf("unexpected config payload: %+v", cfg)
	}
	if len(cfg.Servers[0].Tools) != 1 || cfg.Servers[0].Tools[0].Aliases[0] != "repo_query" {
		t.Fatalf("unexpected config tools: %+v", cfg.Servers[0].Tools)
	}
}
