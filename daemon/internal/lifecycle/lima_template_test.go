package lifecycle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGenerateLimaTemplateProducesValidYAML(t *testing.T) {
	origHome := isolationUserHomeDir
	origEnv := isolationEnvLookup
	t.Cleanup(func() {
		isolationUserHomeDir = origHome
		isolationEnvLookup = origEnv
	})

	home := t.TempDir()
	isolationUserHomeDir = func() (string, error) { return home, nil }
	isolationEnvLookup = func(string) string { return "" }

	workspace := filepath.Join(home, ".carrier", "instances", "openclaw", "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	resolvedWorkspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	instance := "carrier-openclaw-a3f2b1c4"
	data, err := generateLimaTemplate(instance, workspace)
	if err != nil {
		t.Fatalf("generateLimaTemplate: %v", err)
	}

	var parsed struct {
		Mounts []struct {
			Location string `yaml:"location"`
			Writable bool   `yaml:"writable"`
		} `yaml:"mounts"`
	}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if len(parsed.Mounts) != 1 {
		t.Fatalf("mount count = %d, want 1", len(parsed.Mounts))
	}
	if parsed.Mounts[0].Location != resolvedWorkspace {
		t.Fatalf("mount location = %q, want %q", parsed.Mounts[0].Location, resolvedWorkspace)
	}
	if !parsed.Mounts[0].Writable {
		t.Fatal("expected mount to be writable")
	}
	if strings.Contains(string(data), ".ssh") || strings.Contains(string(data), "/Users/") {
		t.Fatalf("template should not contain default home mounts:\n%s", string(data))
	}
}

func TestGenerateLimaTemplateExpandsDarwinUbuntuLTSAliasWhenBuiltinTemplateIsAvailable(t *testing.T) {
	origHome := isolationUserHomeDir
	origEnv := isolationEnvLookup
	origGOOS := isolationRuntimeGOOS
	origCandidates := isolationLimaImageTemplateCandidates
	origReadFile := isolationReadFile
	t.Cleanup(func() {
		isolationUserHomeDir = origHome
		isolationEnvLookup = origEnv
		isolationRuntimeGOOS = origGOOS
		isolationLimaImageTemplateCandidates = origCandidates
		isolationReadFile = origReadFile
	})

	home := t.TempDir()
	isolationUserHomeDir = func() (string, error) { return home, nil }
	isolationEnvLookup = func(string) string { return "" }
	isolationRuntimeGOOS = "darwin"

	templatePath := filepath.Join(t.TempDir(), "ubuntu-lts.yaml")
	raw := []byte(`
images:
  - location: "https://example.invalid/ubuntu-arm64.img"
    arch: "aarch64"
    digest: "sha256:abc123"
  - location: "https://example.invalid/ubuntu-amd64.img"
    arch: "x86_64"
`)
	if err := os.WriteFile(templatePath, raw, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	isolationLimaImageTemplateCandidates = func() []string { return []string{templatePath} }
	isolationReadFile = os.ReadFile

	workspace := filepath.Join(home, ".carrier", "instances", "zeroclaw", "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data, err := generateLimaTemplate("carrier-zeroclaw-a3f2b1c4", workspace)
	if err != nil {
		t.Fatalf("generateLimaTemplate: %v", err)
	}

	var parsed struct {
		Images []limaImage `yaml:"images"`
	}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if len(parsed.Images) != 2 {
		t.Fatalf("image count = %d, want 2", len(parsed.Images))
	}
	if parsed.Images[0].Location != "https://example.invalid/ubuntu-arm64.img" {
		t.Fatalf("first image location = %q", parsed.Images[0].Location)
	}
	if parsed.Images[0].Digest != "sha256:abc123" {
		t.Fatalf("first image digest = %q", parsed.Images[0].Digest)
	}
	if strings.Contains(string(data), `location: "ubuntu-lts"`) {
		t.Fatalf("template should not retain ubuntu-lts alias when explicit images are available:\n%s", string(data))
	}
}

func TestYAMLInjectionViaMaliciousWorkspacePath(t *testing.T) {
	origHome := isolationUserHomeDir
	origEnv := isolationEnvLookup
	t.Cleanup(func() {
		isolationUserHomeDir = origHome
		isolationEnvLookup = origEnv
	})

	home := t.TempDir()
	isolationUserHomeDir = func() (string, error) { return home, nil }
	isolationEnvLookup = func(string) string { return "" }

	workspace := filepath.Join(home, ".carrier", "instances", "evil\ninjection: true", "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data, err := generateLimaTemplate("carrier-openclaw-deadbeef", workspace)
	if err != nil {
		t.Fatalf("generateLimaTemplate: %v", err)
	}
	// More robust YAML injection check: unmarshal and verify no unexpected fields were injected
	var parsed map[string]interface{}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if _, ok := parsed["injection"]; ok {
		t.Fatalf("YAML injection was successful; unexpected 'injection' key found in parsed output")
	}
}

func TestValidateLimaInstanceNameRejectsTraversal(t *testing.T) {
	cases := []string{
		"",
		"../../etc/passwd",
		"carrier-openclaw-xyz!",
		"carrier--a3f2",
	}
	for _, name := range cases {
		if err := validateLimaInstanceName(name); err == nil {
			t.Fatalf("validateLimaInstanceName(%q) succeeded unexpectedly", name)
		}
	}
}

func TestValidateWorkspacePathRejectsOutsidePrefix(t *testing.T) {
	origEnv := isolationEnvLookup
	t.Cleanup(func() {
		isolationEnvLookup = origEnv
	})

	base := t.TempDir()
	isolationEnvLookup = func(key string) string {
		if key == instanceStoreEnvKey {
			return filepath.Join(base, "instances.json")
		}
		return ""
	}
	outside := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if _, err := validateWorkspacePath(outside); err == nil {
		t.Fatalf("validateWorkspacePath(%q) should fail for outside prefix", outside)
	}
}

func TestSymlinkEscapeInWorkspaceRejected(t *testing.T) {
	origEnv := isolationEnvLookup
	t.Cleanup(func() {
		isolationEnvLookup = origEnv
	})

	base := t.TempDir()
	isolationEnvLookup = func(key string) string {
		if key == instanceStoreEnvKey {
			return filepath.Join(base, "instances.json")
		}
		return ""
	}
	target := filepath.Join(t.TempDir(), "outside")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatalf("MkdirAll target: %v", err)
	}
	link := filepath.Join(base, "symlinked-workspace")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := validateWorkspacePath(link); err == nil {
		t.Fatal("expected symlink escape to be rejected")
	}
}

func TestTemplatePathConfinement(t *testing.T) {
	origHome := isolationUserHomeDir
	t.Cleanup(func() { isolationUserHomeDir = origHome })
	home := t.TempDir()
	isolationUserHomeDir = func() (string, error) { return home, nil }

	if _, err := templatePath("../../etc/passwd"); err == nil {
		t.Fatal("expected traversal instance name to be rejected")
	}
	path, err := templatePath("carrier-openclaw-abcd1234")
	if err != nil {
		t.Fatalf("templatePath: %v", err)
	}
	root := filepath.Join(home, ".carrier", "lima", "templates")
	if !isPathWithinBase(root, path) {
		t.Fatalf("template path %q escapes root %q", path, root)
	}
}

func TestGenerateLimaInstanceNameUniqueness(t *testing.T) {
	seen := map[string]struct{}{}
	for i := 0; i < 200; i++ {
		name, err := generateLimaInstanceName("OpenClaw")
		if err != nil {
			t.Fatalf("generateLimaInstanceName: %v", err)
		}
		if _, exists := seen[name]; exists {
			t.Fatalf("duplicate generated instance name %q", name)
		}
		seen[name] = struct{}{}
	}
}
