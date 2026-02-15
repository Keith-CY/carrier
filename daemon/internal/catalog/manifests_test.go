package catalog

import (
	"carrier/daemon/internal/manifest"
	"path/filepath"
	"testing"
)

func TestOpenClawManifestUsesDaemonHealthContract(t *testing.T) {
	m := OpenClawManifest()

	if len(m.Network.Ports) == 0 {
		t.Fatal("expected at least one network port in manifest")
	}
	if got := m.Network.Ports[0].Port; got != defaultDaemonPort {
		t.Fatalf("network port = %d, want %d", got, defaultDaemonPort)
	}
	if got := m.Network.Healthcheck.URL; got != defaultDaemonHealthURL {
		t.Fatalf("healthcheck url = %q, want %q", got, defaultDaemonHealthURL)
	}
}

func TestPicoClawManifestValid(t *testing.T) {
	m := PicoClawManifest()

	if err := m.Validate(); err != nil {
		t.Fatalf("PicoClawManifest().Validate() = %v", err)
	}

	if m.ID != "picoclaw" {
		t.Fatalf("id = %q, want %q", m.ID, "picoclaw")
	}
	if m.Network.Healthcheck.Type != "process" {
		t.Fatalf("healthcheck type = %q, want %q", m.Network.Healthcheck.Type, "process")
	}
	if m.Runtime.Stop.Command != "signal:term" {
		t.Fatalf("stop command = %q, want %q", m.Runtime.Stop.Command, "signal:term")
	}
}

func TestPicoClawBinaryName(t *testing.T) {
	name := picoClawBinaryName()
	if name == "" {
		t.Fatal("picoClawBinaryName() returned empty string")
	}
	// Should contain picoclaw and current GOOS/GOARCH
	if !contains(name, "picoclaw") {
		t.Fatalf("binary name %q does not contain 'picoclaw'", name)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestCatalogJSONHealthcheckMatchesGeneratedManifest(t *testing.T) {
	path := filepath.Join("..", "..", "..", "catalog", "openclaw.manifest.json")
	fileManifest, err := manifest.LoadFile(path)
	if err != nil {
		t.Fatalf("load catalog manifest: %v", err)
	}

	generated := OpenClawManifest()
	if fileManifest.Network.Healthcheck.URL != generated.Network.Healthcheck.URL {
		t.Fatalf("catalog healthcheck url = %q, generated = %q", fileManifest.Network.Healthcheck.URL, generated.Network.Healthcheck.URL)
	}
	if len(fileManifest.Network.Ports) == 0 || len(generated.Network.Ports) == 0 {
		t.Fatal("expected network ports in both manifests")
	}
	if fileManifest.Network.Ports[0].Port != generated.Network.Ports[0].Port {
		t.Fatalf("catalog port = %d, generated = %d", fileManifest.Network.Ports[0].Port, generated.Network.Ports[0].Port)
	}
}
