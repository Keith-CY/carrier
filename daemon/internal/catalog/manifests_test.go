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
