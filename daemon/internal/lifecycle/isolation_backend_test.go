package lifecycle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateLimaTemplateIncludesAgentWorkDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	backend := limaIsolationBackend{agentWorkDir: "/tmp/carrier-agent"}
	templatePath, err := backend.generateLimaTemplate("carrier-dev")
	if err != nil {
		t.Fatalf("generateLimaTemplate: %v", err)
	}

	if templatePath != filepath.Join(home, ".carrier", "lima", "carrier-dev.yaml") {
		t.Fatalf("unexpected template path: %q", templatePath)
	}

	data, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "location: \"/tmp/carrier-agent\"") {
		t.Fatalf("expected template to include agent work dir, got:\n%s", content)
	}
	if strings.Contains(content, home) {
		t.Fatalf("template unexpectedly references host home directory %q:\n%s", home, content)
	}
}

func TestGenerateLimaTemplateUsesNoMountsWhenAgentWorkDirEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	backend := limaIsolationBackend{agentWorkDir: ""}
	templatePath, err := backend.generateLimaTemplate("carrier-empty")
	if err != nil {
		t.Fatalf("generateLimaTemplate: %v", err)
	}

	data, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "mounts: []") {
		t.Fatalf("expected empty mounts in template, got:\n%s", content)
	}
	if strings.Contains(content, "location:") {
		t.Fatalf("expected no mount location entries when work dir is empty, got:\n%s", content)
	}
}
