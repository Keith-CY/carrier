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

func TestSanitizeInstanceID(t *testing.T) {
	cases := map[string]string{
		"openclaw":         "openclaw",
		"my-agent_01":      "my-agent_01",
		"../../../etc":     "etc",
		"agent\ninjection": "agentinjection",
		`agent"yaml`:       "agentyaml",
		"":                 "",
		"  spaces  ":       "spaces",
	}
	for input, want := range cases {
		got := sanitizeInstanceID(input)
		if got != want {
			t.Errorf("sanitizeInstanceID(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestPerAgentLimaInstance(t *testing.T) {
	// Verify that different agentIDs produce different instance names
	id1 := sanitizeInstanceID("openclaw")
	id2 := sanitizeInstanceID("picoclaw")
	if id1 == id2 {
		t.Fatalf("expected different instance IDs, got %q and %q", id1, id2)
	}
}
